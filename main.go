package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
	"github.com/pion/webrtc/v3/pkg/media/oggreader"
	"github.com/pion/webrtc/v3/pkg/media/oggwriter"
)

// ========================= Configuración WebRTC =========================

// Cambia/añade TURN si lo necesitas para NATs estrictos
var rtcConfig = webrtc.Configuration{
	ICEServers: []webrtc.ICEServer{
		{
			URLs: []string{
				"stun:stun.l.google.com:19302",
				"stun:stun.l.google.com:19305",
			},
		},
		// {
		// 	URLs:       []string{"turn:tu-turn.example.com:3478"},
		// 	Username:   "user",
		// 	Credential: "pass",
		// },
	},
}

// Autocolgado por inactividad RTP (0 = deshabilitado)
const IdleHangupSeconds = 0

// ========================= Registro de llamadas =========================

type Call struct {
	ID   string
	PC   *webrtc.PeerConnection
	Done chan struct{}
}

var calls sync.Map // map[string]*Call

func newCallID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), rand.Intn(100000))
}

func storeCall(c *Call) { calls.Store(c.ID, c) }

func loadCall(id string) (*Call, bool) {
	if v, ok := calls.Load(id); ok {
		return v.(*Call), true
	}
	return nil, false
}

func deleteCall(id string) { calls.Delete(id) }

// ========================= Handlers HTTP =========================

func main() {
	rand.Seed(time.Now().UnixNano())

	mux := http.NewServeMux()
	mux.HandleFunc("/sdp", handleSDP)       // crea/negocia una llamada
	mux.HandleFunc("/hangup", handleHangup) // cuelga por id
	mux.HandleFunc("/status", handleStatus) // lista llamadas activas

	addr := ":8080"
	log.Printf("Servidor escuchando en %s (POST /sdp, GET /hangup?id=..., GET /status)", addr)
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

	// ========= CONFIG LOCAL "QUEMADA" (emisón de OGG) =========
	const outOGGPath = "/home/desarrollo2/GolandProjects/webrtc-audio-server/audio-1755881306.ogg" // <-- CAMBIA ESTO
	const outTimeoutSec = 25                                                                       // 0 = sin timeout; >0 segundos para cortar el envío
	const closeOnTimeout = false                                                                   // true: cierra la llamada al expirar el timeout
	// =========================================================

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

	// 5) SettingEngine: responder como DTLS CLIENT (setup:active) y solo UDP4 opcional
	se := webrtc.SettingEngine{}
	se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
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

	// ---- Crear y registrar la "Call" ----
	callID := newCallID()
	call := &Call{ID: callID, PC: peer, Done: make(chan struct{})}
	storeCall(call)
	log.Printf(">> Call creada: id=%s", callID)

	// 7) Logs detallados de estados/negociación
	peer.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) {
		log.Printf(">> ICE state: %s (id=%s)", s.String(), callID)
	})
	peer.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		log.Printf(">> PC state: %s (id=%s)", s.String(), callID)
		if s == webrtc.PeerConnectionStateFailed ||
			s == webrtc.PeerConnectionStateClosed {
			_ = peer.Close()
			select {
			case <-call.Done:
			default:
				close(call.Done)
			}
			deleteCall(callID)
			log.Printf(">> Call cerrada y eliminada: id=%s", callID)
		}
	})
	peer.OnSignalingStateChange(func(s webrtc.SignalingState) {
		log.Printf(">> Signaling state: %s (id=%s)", s.String(), callID)
	})
	peer.OnNegotiationNeeded(func() {
		log.Printf(">> Negotiation needed (id=%s)", callID)
	})
	peer.OnICEGatheringStateChange(func(s webrtc.ICEGathererState) {
		log.Printf(">> ICE gathering state: %s (id=%s)", s.String(), callID)
	})

	// 8) Transceiver de audio:
	//    - si vamos a ENVIAR OGG: SENDRECV
	//    - si no enviamos: RECVONLY
	dir := webrtc.RTPTransceiverDirectionRecvonly
	if outOGGPath != "" {
		dir = webrtc.RTPTransceiverDirectionSendrecv
	}
	audioTrans, err := peer.AddTransceiverFromKind(
		webrtc.RTPCodecTypeAudio,
		webrtc.RTPTransceiverInit{Direction: dir},
	)
	if err != nil {
		log.Printf("AddTransceiverFromKind error: %v (id=%s)", err, callID)
	}

	// 9) Recolectar candidatos locales (para devolver al cliente)
	localCandidates := []webrtc.ICECandidateInit{}
	peer.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c != nil {
			log.Printf(">> Nuevo ICE Candidate local: %s (id=%s)", c.String(), callID)
			localCandidates = append(localCandidates, c.ToJSON())
		} else {
			log.Printf(">> Recolección de ICE finalizada (id=%s)", callID)
		}
	})

	// 10) OnTrack: guardar audio entrante en OGG con ruta absoluta
	peer.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if track.Kind() != webrtc.RTPCodecTypeAudio {
			log.Printf(">> Track entrante ignorado (no audio): %s (id=%s)", track.Kind().String(), callID)
			return
		}
		cwd, _ := os.Getwd()
		filename := fmt.Sprintf("audio-%d.ogg", time.Now().Unix())
		abs := filepath.Join(cwd, filename)
		log.Printf(">> Audio entrante detectado, guardando en: %s (codec=%s) (id=%s)", abs, track.Codec().MimeType, callID)

		ogg, err := oggwriter.New(abs, 48000, 2)
		if err != nil {
			log.Printf("error creando ogg: %v (id=%s)", err, callID)
			return
		}
		defer ogg.Close()

		// Colgar por inactividad, si está habilitado
		var timer *time.Timer
		if IdleHangupSeconds > 0 {
			timer = time.NewTimer(time.Duration(IdleHangupSeconds) * time.Second)
			defer timer.Stop()
			go func() {
				<-timer.C
				log.Printf(">> No hay RTP por %ds. Colgando (id=%s)", IdleHangupSeconds, callID)
				_ = peer.Close()
			}()
		}

		for {
			pkt, _, err := track.ReadRTP()
			if err != nil {
				log.Printf(">> Fin de track: %v (id=%s)", err, callID)
				return
			}
			if timer != nil {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(time.Duration(IdleHangupSeconds) * time.Second)
			}

			log.Printf(">> RTP recibido: SSRC=%d Seq=%d TS=%d (id=%s)", pkt.SSRC, pkt.SequenceNumber, pkt.Timestamp, callID)
			if writeErr := ogg.WriteRTP(pkt); writeErr != nil {
				log.Printf("error escribiendo ogg: %v (id=%s)", writeErr, callID)
				return
			}
		}
	})

	// 11) **EMISIÓN DE OGG** (arranca cuando PC=connected)
	if outOGGPath != "" && audioTrans != nil {
		log.Printf(">> OUTGOING: preparado para enviar OGG='%s' timeout=%ds (id=%s)", outOGGPath, outTimeoutSec, callID)

		// Creamos pista local "sample" Opus y la conectamos al sender del transceiver
		trackLocal, err := webrtc.NewTrackLocalStaticSample(
			webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeOpus,
				ClockRate: 48000,
				Channels:  2,
			},
			"server-audio", "pion",
		)
		if err != nil {
			log.Printf("NewTrackLocalStaticSample error: %v (id=%s)", err, callID)
		} else if err := audioTrans.Sender().ReplaceTrack(trackLocal); err != nil {
			log.Printf("ReplaceTrack error: %v (id=%s)", err, callID)
		} else {
			// drenar RTCP para evitar bloqueo del sender
			go func(ss *webrtc.RTPSender) {
				buf := make([]byte, 1500)
				for {
					if _, _, err := ss.Read(buf); err != nil {
						return
					}
				}
			}(audioTrans.Sender())

			// IMPORTANTE: empieza a enviar SOLO cuando la PC está conectada
			peer.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
				log.Printf(">> PC state: %s (id=%s)", s.String(), callID)

				if s == webrtc.PeerConnectionStateConnected {
					log.Printf(">> OUTGOING: conexión lista, comenzando envío OGG (id=%s)", callID)

					go func() {
						f, err := os.Open(outOGGPath)
						if err != nil {
							log.Printf("OGG open error: %v (id=%s)", err, callID)
							return
						}
						defer f.Close()

						r, _, err := oggreader.NewWith(f)
						if err != nil {
							log.Printf("oggreader.NewWith error: %v (id=%s)", err, callID)
							return
						}

						// timeout opcional
						var timeout <-chan time.Time
						if outTimeoutSec > 0 {
							t := time.NewTimer(time.Duration(outTimeoutSec) * time.Second)
							defer t.Stop()
							timeout = t.C
						}

						frame := 20 * time.Millisecond // pacing típico Opus

						for {
							select {
							case <-timeout:
								log.Printf(">> OUTGOING: timeout alcanzado (%ds) (id=%s)", outTimeoutSec, callID)
								if closeOnTimeout {
									_ = peer.Close()
								}
								return
							default:
							}

							// Lee siguiente página OGG (payload Opus)
							pageData, _, err := r.ParseNextPage()
							if err == io.EOF {
								log.Printf(">> OUTGOING: EOF OGG %s (id=%s)", outOGGPath, callID)
								return
							}
							if err != nil {
								log.Printf("ParseNextPage error: %v (id=%s)", err, callID)
								return
							}

							// Empuja sample hacia el remoto
							if werr := trackLocal.WriteSample(media.Sample{
								Data:     pageData,
								Duration: frame,
							}); werr != nil {
								log.Printf("WriteSample error: %v (id=%s)", werr, callID)
								return
							}

							time.Sleep(frame) // pacing simple
						}
					}()
				}

				if s == webrtc.PeerConnectionStateFailed || s == webrtc.PeerConnectionStateClosed {
					_ = peer.Close()
					select {
					case <-call.Done:
					default:
						close(call.Done)
					}
					deleteCall(callID)
					log.Printf(">> Call cerrada y eliminada: id=%s", callID)
				}
			})
		}
	}

	// 12) Aplicar la oferta remota y los candidatos remotos
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
		log.Printf(">> ICE Candidate remoto añadido: %+v (id=%s)", c, callID)
	}

	// 13) Crear y aplicar la answer local
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

	// 14) Responder al cliente con "<answerEncoded>;<candidatesEncoded>"
	localSDP := peer.LocalDescription()
	out := signalEncode(*localSDP) + ";" + signalEncode(localCandidates)

	// Devolver el callID por header (para /hangup)
	w.Header().Set("X-Call-ID", callID)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(out))
	log.Printf(">> Answer enviada al cliente (id=%s)", callID)
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
	_ = call.PC.Close()
	select {
	case <-call.Done:
	default:
		close(call.Done)
	}
	deleteCall(id)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
	log.Printf(">> Hangup completado para id=%s", id)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	var ids []string
	calls.Range(func(k, _ any) bool {
		ids = append(ids, k.(string))
		return true
	})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"active_calls": ids,
		"count":        len(ids),
	})
}

// Enviar audio a un servidor de voz

// lee RTCP para que el sender no se bloquee
func drainRTCP(ss *webrtc.RTPSender) {
	buf := make([]byte, 1500)
	for {
		if _, _, err := ss.Read(buf); err != nil {
			return // sender cerrado
		}
	}
}

// Adjunta una pista local Opus (Sample) al transceiver de audio y
// empuja el contenido de un .ogg (Opus) durante "duration".
// Si duration <= 0, envía hasta EOF. Si closeOnTimeout = true, cierra el Peer al vencer.
func attachOGGToTransceiver(peer *webrtc.PeerConnection, trans *webrtc.RTPTransceiver,
	oggPath string, duration time.Duration, closeOnTimeout bool) (chan struct{}, error) {

	// Pista local para enviar SAMPLES Opus (48k, 2ch)
	trackLocal, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeOpus,
			ClockRate: 48000,
			Channels:  2,
		},
		"server-audio", "pion",
	)
	if err != nil {
		return nil, fmt.Errorf("NewTrackLocalStaticSample: %w", err)
	}

	// Usamos el sender del transceiver ya creado (evita crear una 2da m=audio)
	if err := trans.Sender().ReplaceTrack(trackLocal); err != nil {
		return nil, fmt.Errorf("ReplaceTrack: %w", err)
	}

	// lee RTCP en background
	go drainRTCP(trans.Sender())

	done := make(chan struct{})

	go func() {
		defer close(done)

		f, err := os.Open(oggPath)
		if err != nil {
			log.Printf("attachOGGToTransceiver: no puedo abrir OGG: %v", err)
			return
		}
		defer f.Close()

		r, _, err := oggreader.NewWith(f)
		if err != nil {
			log.Printf("attachOGGToTransceiver: oggreader.NewWith: %v", err)
			return
		}

		// timeout opcional
		var timeout <-chan time.Time
		if duration > 0 {
			t := time.NewTimer(duration)
			defer t.Stop()
			timeout = t.C
		}

		// pacing típico de Opus (~20ms por frame)
		frame := 20 * time.Millisecond

		for {
			select {
			case <-timeout:
				log.Printf("attachOGGToTransceiver: timeout alcanzado (%v), deteniendo envío", duration)
				if closeOnTimeout {
					_ = peer.Close()
				}
				return
			default:
			}

			// lee siguiente página OGG (payload Opus)
			pageData, _, err := r.ParseNextPage()
			if err == io.EOF {
				log.Printf("attachOGGToTransceiver: EOF %s", oggPath)
				return
			}
			if err != nil {
				log.Printf("attachOGGToTransceiver: ParseNextPage error: %v", err)
				return
			}

			// empuja sample a la pista local
			if werr := trackLocal.WriteSample(media.Sample{
				Data:     pageData,
				Duration: frame,
			}); werr != nil {
				log.Printf("attachOGGToTransceiver: WriteSample error: %v", werr)
				return
			}

			time.Sleep(frame) // pacing simple
		}
	}()

	return done, nil
}
