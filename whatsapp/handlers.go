package whatsapp

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"

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
						//go startAudioSending(transceiver.Sender().Track(), call.ID)
						go initAgentCall(transceiver.Sender().Track())
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

func initAgentCall(trackLocal webrtc.TrackLocal) {
	// Verificar que es una TrackLocalStaticSample para poder enviar audio
	trackLocalSample, ok := trackLocal.(*webrtc.TrackLocalStaticSample)
	if !ok {
		log.Printf("❌ TrackLocal no es del tipo StaticSample, no se puede transmitir audio del agente")
		return
	}

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
		log.Printf("❌ Error conectando al room: %v", err)
		return
	}

	// Inicializar audio
	if err := participant.InitializeAudio(); err != nil {
		log.Printf("❌ Error inicializando audio: %v", err)
		return
	}

	// Configurar el puente de audio del agente hacia el track WebRTC
	go bridgeAgentAudioToWebRTC(participant, trackLocalSample)

	// Configurar manejo de señales para una desconexión limpia
	participant.SetupSignalHandling()

	log.Printf("✅ Conectado exitosamente al room de LiveKit y configurado puente de audio")
}

// bridgeAgentAudioToWebRTC crea un puente entre el audio del agente de Retell y el track WebRTC
func bridgeAgentAudioToWebRTC(participant *retell.RoomParticipant, trackLocal *webrtc.TrackLocalStaticSample) {
	log.Printf("🔗 Iniciando puente de audio del agente hacia WebRTC")

	// Buffer para acumular audio del agente
	audioBuffer := make(chan []byte, 100) // Buffer con capacidad para evitar bloqueos

	// Modificar el participant para que envíe audio a nuestro buffer
	participant.SetAudioBridgeChannel(audioBuffer)

	// Configuración de audio para WebRTC Opus
	const (
		sampleRate      = 48000
		channels        = 1                     // Mono para voz, más estable
		frameDuration   = 20 * time.Millisecond // 20ms por frame Opus
		samplesPerFrame = 960                   // 960 samples para 20ms a 48kHz
	)

	log.Printf("🎵 Configurando puente de audio: SampleRate=%d, Channels=%d, FrameDuration=%v, SamplesPerFrame=%d",
		sampleRate, channels, frameDuration, samplesPerFrame)

	// Crear encoder Opus
	encoder, err := retell.CreateOpusEncoder(sampleRate, channels)
	if err != nil {
		log.Printf("❌ Error creando encoder Opus: %v", err)
		return
	}
	// Los encoders de Opus en Go no necesitan destrucción explícita

	// Buffer para acumular muestras PCM
	pcmBuffer := make([]int16, 0, samplesPerFrame*channels)

	// Procesar audio en bucle
	for audioData := range audioBuffer {
		if len(audioData) == 0 {
			continue
		}

		// Convertir bytes a int16 samples
		numSamples := len(audioData) / 2
		samples := make([]int16, numSamples)
		for i := 0; i < numSamples; i++ {
			// Little endian decoding
			samples[i] = int16(audioData[i*2]) | int16(audioData[i*2+1])<<8
		}

		// Acumular muestras en el buffer
		pcmBuffer = append(pcmBuffer, samples...)

		// Procesar frames completos
		for len(pcmBuffer) >= samplesPerFrame*channels {
			// Extraer un frame completo
			frameData := pcmBuffer[:samplesPerFrame*channels]
			pcmBuffer = pcmBuffer[samplesPerFrame*channels:]

			// Codificar PCM a Opus
			opusBuffer := make([]byte, 4000) // Buffer para datos Opus
			opusLength, err := encoder.Encode(frameData, opusBuffer)
			if err != nil {
				log.Printf("⚠️ Error codificando Opus: %v", err)
				continue
			}

			if opusLength > 0 {
				// Extraer solo los datos útiles del buffer
				opusData := opusBuffer[:opusLength]

				// Enviar datos Opus al track WebRTC
				sample := media.Sample{
					Data:     opusData,
					Duration: frameDuration,
				}

				if err := trackLocal.WriteSample(sample); err != nil {
					log.Printf("⚠️ Error enviando audio al track WebRTC: %v", err)
					continue
				}

				// Log ocasional para verificar transmisión
				if time.Now().UnixNano()%1000000000 < 50000000 { // Log cada ~1 segundo aprox
					log.Printf("🎵 Frame Opus transmitido: %d bytes PCM → %d bytes Opus", len(frameData)*2, len(opusData))
				}
			}
		}
	}

	log.Printf("🔇 Puente de audio del agente finalizado")
}
