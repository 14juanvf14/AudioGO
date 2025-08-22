package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media/oggwriter"
)

// Cambia los STUN si lo necesitas
var rtcConfig = webrtc.Configuration{
	ICEServers: []webrtc.ICEServer{
		{URLs: []string{
			"stun:stun.l.google.com:19302",
			"stun:stun.l.google.com:19305",
		}},
	},
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/sdp", handleSDP) // POST body: "<offerEncoded>;<candidatesEncoded>"

	addr := ":8080"
	log.Printf("Servidor escuchando en %s (POST /sdp)", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func handleSDP(w http.ResponseWriter, r *http.Request) {
	log.Println(">> Nueva solicitud SDP recibida")

	if r.Method != http.MethodPost {
		http.Error(w, "use POST", http.StatusMethodNotAllowed)
		return
	}

	// 1) Leer TODO el body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "error leyendo cuerpo", http.StatusBadRequest)
		return
	}
	log.Printf(">> Payload recibido (len=%d)", len(body))

	// 2) Separar "<offerEncoded>;<candidatesEncoded>"
	payload := strings.TrimSpace(string(body))
	parts := strings.Split(payload, ";")
	if len(parts) != 2 {
		http.Error(w, "formato esperado: <offerEncoded>;<candidatesEncoded>", http.StatusBadRequest)
		return
	}

	// 3) Decodificar oferta y candidatos remotos
	var remoteOffer webrtc.SessionDescription
	signalDecode(parts[0], &remoteOffer)
	log.Printf(">> RemoteOffer.type=%s, len(SDP)=%d", remoteOffer.Type, len(remoteOffer.SDP))

	var remoteCandidates []webrtc.ICECandidateInit
	signalDecode(parts[1], &remoteCandidates)
	log.Printf(">> RemoteCandidates recibidos=%d", len(remoteCandidates))

	// 4) MediaEngine (Opus, etc.)
	var m webrtc.MediaEngine
	if err := m.RegisterDefaultCodecs(); err != nil {
		http.Error(w, "no se pudo registrar codecs", http.StatusInternalServerError)
		return
	}

	// 5) SettingEngine: forzar DTLS role activo al responder (clave con a=setup:actpass)
	se := webrtc.SettingEngine{}

	// opcional: solo UDP/IPv4 si tienes muchas interfaces
	se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})

	// 👇 maneja el error; esto fuerza que tu *answer* sea DTLS "client" (setup:active)
	if err := se.SetAnsweringDTLSRole(webrtc.DTLSRoleClient); err != nil {
		log.Printf("SetAnsweringDTLSRole error: %v", err)
	}

	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(&m),
		webrtc.WithSettingEngine(se),
	)

	// 6) Crear PeerConnection
	peer, err := api.NewPeerConnection(rtcConfig)
	if err != nil {
		http.Error(w, "error creando PeerConnection", http.StatusInternalServerError)
		return
	}
	log.Println(">> PeerConnection creado")

	// 7) Logs detallados de estados/negociación
	peer.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) {
		log.Printf(">> ICE state: %s", s.String())
	})

	peer.OnSignalingStateChange(func(s webrtc.SignalingState) {
		log.Printf(">> Signaling state: %s", s.String())
	})
	peer.OnNegotiationNeeded(func() {
		log.Printf(">> Negotiation needed")
	})
	peer.OnICEGatheringStateChange(func(s webrtc.ICEGathererState) {
		log.Printf(">> ICE gathering state: %s", s.String())
	})

	peer.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		log.Printf(">> PC state: %s", s.String())
		if s == webrtc.PeerConnectionStateFailed ||
			s == webrtc.PeerConnectionStateClosed ||
			s == webrtc.PeerConnectionStateDisconnected {
			_ = peer.Close()
		}
	})

	// 8) Anunciar que QUEREMOS RECIBIR audio (recvonly) antes de crear la answer
	if _, err := peer.AddTransceiverFromKind(
		webrtc.RTPCodecTypeAudio,
		webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly},
	); err != nil {
		log.Printf("AddTransceiverFromKind error: %v", err)
	}

	// 9) Recolectar candidatos locales (para devolver al cliente)
	localCandidates := []webrtc.ICECandidateInit{}
	peer.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c != nil {
			log.Printf(">> Nuevo ICE Candidate local: %s", c.String())
			localCandidates = append(localCandidates, c.ToJSON())
		} else {
			log.Println(">> Recolección de ICE finalizada")
		}
	})

	// 10) OnTrack: guardar audio entrante en OGG con ruta absoluta
	peer.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if track.Kind() != webrtc.RTPCodecTypeAudio {
			log.Printf(">> Track entrante ignorado (no audio): %s", track.Kind().String())
			return
		}
		cwd, _ := os.Getwd()
		filename := fmt.Sprintf("audio-%d.ogg", time.Now().Unix())
		abs := filepath.Join(cwd, filename)
		log.Printf(">> Audio entrante detectado, guardando en: %s (codec=%s)", abs, track.Codec().MimeType)

		ogg, err := oggwriter.New(abs, 48000, 2)
		if err != nil {
			log.Printf("error creando ogg: %v", err)
			return
		}
		defer ogg.Close()

		for {
			pkt, _, err := track.ReadRTP()
			if err != nil {
				log.Printf(">> Fin de track: %v", err)
				return
			}
			log.Printf(">> RTP recibido: SSRC=%d Seq=%d TS=%d", pkt.SSRC, pkt.SequenceNumber, pkt.Timestamp)
			if writeErr := ogg.WriteRTP(pkt); writeErr != nil {
				log.Printf("error escribiendo ogg: %v", writeErr)
				return
			}
		}
	})

	// 11) Aplicar la oferta remota y los candidatos remotos
	if err := peer.SetRemoteDescription(remoteOffer); err != nil {
		http.Error(w, "SetRemoteDescription falló: "+err.Error(), http.StatusBadRequest)
		return
	}
	log.Println(">> RemoteDescription establecida")

	for _, c := range remoteCandidates {
		if err := peer.AddICECandidate(c); err != nil {
			http.Error(w, "AddICECandidate falló: "+err.Error(), http.StatusBadRequest)
			return
		}
		log.Printf(">> ICE Candidate remoto añadido: %+v", c)
	}

	// 12) Crear y aplicar la answer local
	answer, err := peer.CreateAnswer(nil)
	if err != nil {
		http.Error(w, "CreateAnswer falló: "+err.Error(), http.StatusInternalServerError)
		return
	}
	log.Println(">> Answer creada")

	gatherComplete := webrtc.GatheringCompletePromise(peer)
	if err := peer.SetLocalDescription(answer); err != nil {
		http.Error(w, "SetLocalDescription falló: "+err.Error(), http.StatusInternalServerError)
		return
	}
	log.Println(">> LocalDescription establecida, esperando gathering...")
	<-gatherComplete
	log.Println(">> Gathering completado")

	// (Útil para verificar que quedó a=recvonly y a=setup:active)
	log.Printf(">> Local SDP generado:\n%s", peer.LocalDescription().SDP)

	// 13) Responder al cliente con "<answerEncoded>;<candidatesEncoded>"
	localSDP := peer.LocalDescription()
	out := signalEncode(*localSDP) + ";" + signalEncode(localCandidates)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(out))
	log.Println(">> Answer enviada al cliente")
}
