package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/pion/webrtc/v3"
)

// ========================= Handlers HTTP =========================

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

	// 4) Crear llamada con PeerConnection
	call, err := createCall()
	if err != nil {
		http.Error(w, "error creando llamada: "+err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf(">> Call creada: id=%s", call.ID)

	peer := call.PC

	// 5) Configurar callbacks de estado
	setupPeerCallbacks(peer, call)

	// 6) Transceiver de audio
	dir := webrtc.RTPTransceiverDirectionRecvonly
	if OutOGGPath != "" {
		dir = webrtc.RTPTransceiverDirectionSendrecv
	}
	audioTrans, err := peer.AddTransceiverFromKind(
		webrtc.RTPCodecTypeAudio,
		webrtc.RTPTransceiverInit{Direction: dir},
	)
	if err != nil {
		log.Printf("AddTransceiverFromKind error: %v (id=%s)", err, call.ID)
	}

	// 7) Recolectar candidatos locales (para devolver al cliente)
	localCandidates := []webrtc.ICECandidateInit{}
	peer.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c != nil {
			log.Printf(">> Nuevo ICE Candidate local: %s (id=%s)", c.String(), call.ID)
			localCandidates = append(localCandidates, c.ToJSON())
		} else {
			log.Printf(">> Recolección de ICE finalizada (id=%s)", call.ID)
		}
	})

	// 8) Configurar audio
	setupAudioReceiver(peer, call.ID)
	if err := setupAudioSender(peer, audioTrans, call.ID); err != nil {
		log.Printf("Error configurando audio sender: %v (id=%s)", err, call.ID)
	}

	// 9) Aplicar la oferta remota y los candidatos remotos
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
		log.Printf(">> ICE Candidate remoto añadido: %+v (id=%s)", c, call.ID)
	}

	// 10) Crear y aplicar la answer local
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

	// (Útil para verificar que quedó a=sendrecv (si emites) y a=setup:active)
	log.Printf(">> Local SDP generado:\n%s", peer.LocalDescription().SDP)

	// 11) Responder al cliente con "<answerEncoded>;<candidatesEncoded>"
	localSDP := peer.LocalDescription()
	out := signalEncode(*localSDP) + ";" + signalEncode(localCandidates)

	// Devolver el callID por header (para /hangup)
	w.Header().Set("X-Call-ID", call.ID)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(out))
	log.Printf(">> Answer enviada al cliente (id=%s)", call.ID)
}

func handleHangup(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "falta query param id", http.StatusBadRequest)
		return
	}
	call, ok := loadCall(id)
	if !ok {
		http.Error(w, "call id no encontrado", http.StatusNotFound)
		return
	}
	log.Printf(">> Hangup solicitado para id=%s", id)
	closeCall(call)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
	log.Printf(">> Hangup completado para id=%s", id)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	ids := getAllCalls()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"active_calls": ids,
		"count":        len(ids),
	})
}

// setupPeerCallbacks configura los callbacks de estado del PeerConnection
func setupPeerCallbacks(peer *webrtc.PeerConnection, call *Call) {
	peer.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) {
		log.Printf(">> ICE state: %s (id=%s)", s.String(), call.ID)
	})

	peer.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		log.Printf(">> PC state: %s (id=%s)", s.String(), call.ID)
		if s == webrtc.PeerConnectionStateFailed ||
			s == webrtc.PeerConnectionStateClosed {
			closeCall(call)
			log.Printf(">> Call cerrada y eliminada: id=%s", call.ID)
		}
	})

	peer.OnSignalingStateChange(func(s webrtc.SignalingState) {
		log.Printf(">> Signaling state: %s (id=%s)", s.String(), call.ID)
	})

	peer.OnNegotiationNeeded(func() {
		log.Printf(">> Negotiation needed (id=%s)", call.ID)
	})

	peer.OnICEGatheringStateChange(func(s webrtc.ICEGathererState) {
		log.Printf(">> ICE gathering state: %s (id=%s)", s.String(), call.ID)
	})
}
