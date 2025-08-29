package whatsapp

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/pion/webrtc/v3"

	"fmt"
	"math/rand"
	"time"

	"webrtc-audio-server/retell"

)

// ========================= Handlers HTTP =========================

func HandleSDP(w http.ResponseWriter, r *http.Request) {
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

func HandleHangup(w http.ResponseWriter, r *http.Request) {
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

func HandleStatus(w http.ResponseWriter, r *http.Request) {
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
		// OPTIMIZACIÓN: Empezar audio tan pronto como ICE esté conectado
		if s == webrtc.ICEConnectionStateConnected {
			call.AudioMutex.Lock()
			if !call.AudioStarted {
				call.AudioStarted = true
				call.AudioMutex.Unlock()

				log.Printf(">> ICE conectado, iniciando envío de audio anticipado (id=%s)", call.ID)
				// Buscar el transceiver de audio y activar el envío
				for _, transceiver := range peer.GetTransceivers() {
					if transceiver.Kind() == webrtc.RTPCodecTypeAudio && transceiver.Sender().Track() != nil {
						go startAudioSending(transceiver.Sender().Track(), call.ID)
						break
					}
				}
			} else {
				call.AudioMutex.Unlock()
				log.Printf(">> Audio ya iniciado previamente (id=%s)", call.ID)
			}
		}
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

func initAgentCall() {
		// Inicializar generador de números aleatorios
		rand.Seed(time.Now().UnixNano())

		// Cargar configuración desde variables de entorno o usar valores por defecto
		config := retell.LoadConfig()
	
		// Crear una nueva instancia del participante
		participant := &retell.RoomParticipant{
			Config:       config,
			SampleRate:   retell.SampleRate,
			RemoteTracks: make(map[string]*retell.RemoteAudioTrack),
		}
	
		// Conectar al room
		if err := participant.Connect(); err != nil {
			log.Fatalf("Error conectando al room: %v", err)
		}
	
		// Inicializar audio (simulado para demostración)
		if err := participant.InitializeAudio(); err != nil {
			log.Fatalf("Error inicializando audio: %v", err)
		}
	
		// Configurar manejo de señales para una desconexión limpia
		participant.SetupSignalHandling()
	
		fmt.Printf("Conectado exitosamente al room \n")
		fmt.Println("\n=== FUNCIONALIDADES DE AUDIO ===")
		fmt.Println("✅ Track de micrófono: Publicado y listo")
		fmt.Println("✅ Recepción de audio: Configurada para otros participantes")
		fmt.Println("✅ Gestión de tracks: Automática (conexión/desconexión)")
		fmt.Println("✅ Envío de audio: Nuevo método desde carpeta static al room LiveKit")
		fmt.Println("✅ Grabación de audio: Audio entrante guardado en carpeta recorder")
		fmt.Println("\n📋 Estado actual: MODO LIVEKIT COMPLETO")
		fmt.Println("   • Track de audio está activo en el room")
		fmt.Println("   • Enviando audio desde static/ usando nuevo método")
		fmt.Println("   • Grabando audio entrante en recorder/")
		fmt.Println("   • Sistema simplificado usando solo LiveKit")
		fmt.Println("\nPresiona Ctrl+C para desconectar...")
}
