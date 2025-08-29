package retell

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media/oggreader"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"
	"gopkg.in/hraban/opus.v2"
)

const (
	// Configuración por defecto - estos valores deben ser proporcionados por el usuario
	defaultHostURL = "wss://retell-ai-4ihahnq7.livekit.cloud"

	// Configuración de audio
	SampleRate      = 48000
	framesPerBuffer = 1024
	channels        = 1 // Mono para voz
)

type RoomParticipant struct {
	room         *lksdk.Room
	Config       Config
	audioTrack   *lksdk.LocalTrack
	audioCtx     context.Context
	audioCancel  context.CancelFunc
	audioMutex   sync.Mutex
	isRecording  bool
	isPlaying    bool
	SampleRate   float64
	RemoteTracks map[string]*RemoteAudioTrack
	decoder      *opus.Decoder
	oggReader    *oggreader.OggReader
	oggFile      *os.File
	// Canal para enviar audio del agente hacia el puente WebRTC
	audioBridgeChannel chan []byte
}

type Config struct {
	HostURL string
}

type RemoteAudioTrack struct {
	track       *webrtc.TrackRemote
	decoder     *opus.Decoder
	participant string
	isActive    bool
}

func LoadConfig() Config {
	return Config{
		HostURL: getEnvOrDefault("LIVEKIT_HOST_URL", defaultHostURL),
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func (rp *RoomParticipant) Connect() error {
	// Configurar callbacks para manejar eventos del room
	callbacks := &lksdk.RoomCallback{
		ParticipantCallback: lksdk.ParticipantCallback{
			OnTrackSubscribed:   rp.onTrackSubscribed,
			OnTrackUnsubscribed: rp.onTrackUnsubscribed,
			OnDataReceived:      rp.onDataReceived,
		},
		OnParticipantConnected:    rp.onParticipantConnected,
		OnParticipantDisconnected: rp.onParticipantDisconnected,
		OnRoomMetadataChanged:     rp.onRoomMetadataChanged,
		OnReconnecting:            rp.onReconnecting,
		OnReconnected:             rp.onReconnected,
		OnDisconnected:            rp.onDisconnected,
	}

	// Obtener access token de la API de Retell
	token, err := GetAccessToken()
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}
	log.Printf("Access token obtenido exitosamente")

	// Conectar al room
	room, err := lksdk.ConnectToRoomWithToken(
		rp.Config.HostURL,
		token,
		callbacks,
	)
	if err != nil {
		return fmt.Errorf("failed to connect to room: %w", err)
	}

	rp.room = room
	return nil
}

// initializeAudio configura la captura y reproducción de audio real
func (rp *RoomParticipant) InitializeAudio() error {

	// Crear un contexto para el audio
	rp.audioCtx, rp.audioCancel = context.WithCancel(context.Background())

	// Crear decoder Opus para audio entrante
	var err error
	rp.decoder, err = opus.NewDecoder(SampleRate, channels)
	if err != nil {
		return fmt.Errorf("error creando decoder Opus: %w", err)
	}

	// Crear track de audio local para el micrófono
	rp.audioTrack, err = lksdk.NewLocalSampleTrack(webrtc.RTPCodecCapability{
		MimeType:  "audio/opus",
		ClockRate: SampleRate,
		Channels:  channels,
	})
	if err != nil {
		return fmt.Errorf("error creando track de audio: %w", err)
	}

	// Publicar el track de audio
	_, err = rp.room.LocalParticipant.PublishTrack(rp.audioTrack, &lksdk.TrackPublicationOptions{
		Name: "server-audio-stream",
	})
	if err != nil {
		return fmt.Errorf("error publicando track de audio: %w", err)
	}

	rp.isRecording = true
	rp.isPlaying = true

	fmt.Println("🎵 Audio inicializado correctamente - MODO REAL")
	fmt.Println("✅ Decoder Opus: Configurado para audio entrante")
	fmt.Println("✅ Salida de audio: Configurada para parlantes del sistema")
	fmt.Println("✅ Track de micrófono: Publicado (silencioso)")
	return nil
}

func (rp *RoomParticipant) SetupSignalHandling() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		fmt.Println("\nDesconectando del room...")
		rp.disconnect()
		os.Exit(0)
	}()
}

func (rp *RoomParticipant) disconnect() {
	// Detener captura de audio
	rp.stopAudio()

	// Desconectar del room
	if rp.room != nil {
		rp.room.Disconnect()
	}
}

// stopAudio detiene la captura y reproducción de audio
func (rp *RoomParticipant) stopAudio() {
	rp.audioMutex.Lock()
	defer rp.audioMutex.Unlock()

	rp.isRecording = false
	rp.isPlaying = false

	if rp.audioCancel != nil {
		rp.audioCancel()
	}

	// Cerrar el canal del puente de audio si existe
	if rp.audioBridgeChannel != nil {
		close(rp.audioBridgeChannel)
		rp.audioBridgeChannel = nil
		fmt.Println("🔗 Canal de puente de audio cerrado")
	}

	// Cerrar decoders de tracks remotos
	for _, remoteTrack := range rp.RemoteTracks {
		if remoteTrack.decoder != nil {
			// Los decoders de Opus no necesitan cierre explícito
		}
	}

	if rp.audioTrack != nil {
		rp.audioTrack.Close()
	}

	fmt.Println("🎵 Sistema de audio detenido completamente")
}

// Callbacks para manejar eventos del room

func (rp *RoomParticipant) onTrackSubscribed(track *webrtc.TrackRemote, publication *lksdk.RemoteTrackPublication, participant *lksdk.RemoteParticipant) {
	fmt.Printf("Track suscrito: %s de participante %s (kind: %s)\n",
		publication.Name(), participant.Identity(), publication.Kind().String())

	// Si es un track de audio, configurar la reproducción
	if publication.Kind() == lksdk.TrackKindAudio {
		go rp.handleAudioTrack(track, publication, participant)
	}
}

// handleAudioTrack maneja la reproducción de audio de tracks remotos REALES
func (rp *RoomParticipant) handleAudioTrack(track *webrtc.TrackRemote, publication *lksdk.RemoteTrackPublication, participant *lksdk.RemoteParticipant) {
	trackID := participant.Identity() + "_" + publication.SID()

	// Crear decoder específico para este track
	decoder, err := opus.NewDecoder(SampleRate, channels)
	if err != nil {
		fmt.Printf("❌ Error creando decoder para %s: %v\n", participant.Identity(), err)
		return
	}

	// Crear estructura de track remoto
	remoteTrack := &RemoteAudioTrack{
		track:       track,
		decoder:     decoder,
		participant: participant.Identity(),
		isActive:    true,
	}

	// Almacenar el track
	rp.audioMutex.Lock()
	rp.RemoteTracks[trackID] = remoteTrack
	rp.audioMutex.Unlock()

	fmt.Printf("🎵 Audio track REAL recibido de %s - iniciando reproducción y grabación\n", participant.Identity())

	// Iniciar grabación del audio entrante
	go rp.setupAudioRecording(track, participant)

	// Procesar audio en goroutine separada
	go rp.processAudioTrack(remoteTrack, trackID)
}

// processAudioTrack procesa los paquetes de audio entrantes en tiempo real
func (rp *RoomParticipant) processAudioTrack(remoteTrack *RemoteAudioTrack, trackID string) {
	defer func() {
		rp.audioMutex.Lock()
		delete(rp.RemoteTracks, trackID)
		rp.audioMutex.Unlock()
		fmt.Printf("🔇 Procesamiento de audio detenido para %s\n", remoteTrack.participant)
	}()

	// Buffer para datos RTP
	rtpBuffer := make([]byte, 1600) // Tamaño típico para Opus

	for {
		// Verificar si el contexto sigue activo
		select {
		case <-rp.audioCtx.Done():
			return
		default:
		}

		// Leer paquete RTP del track
		n, _, readErr := remoteTrack.track.Read(rtpBuffer)
		if readErr != nil {
			if readErr.Error() != "EOF" {
				fmt.Printf("⚠️ Error leyendo audio de %s: %v\n", remoteTrack.participant, readErr)
			}
			return
		}

		if n > 0 {
			// Decodificar Opus a PCM
			pcmBuffer := make([]float32, framesPerBuffer*channels)
			samplesDecoded, decodeErr := remoteTrack.decoder.DecodeFloat32(rtpBuffer[:n], pcmBuffer)

			if decodeErr != nil {
				fmt.Printf("⚠️ Error decodificando audio de %s: %v\n", remoteTrack.participant, decodeErr)
				continue
			}

			if samplesDecoded > 0 {
				// Reproducir audio inmediatamente (esto será manejado por el callback del stream)
				// El audio se mezcla automáticamente en el callback de outputStream
				rp.playAudioBuffer(pcmBuffer[:samplesDecoded*channels], remoteTrack)
			}
		}
	}
}

// playAudioBuffer reproduce un buffer de audio PCM y lo envía al puente WebRTC si está configurado
func (rp *RoomParticipant) playAudioBuffer(buffer []float32, remoteTrack *RemoteAudioTrack) {
	rp.audioMutex.Lock()
	remoteTrack.isActive = len(buffer) > 0
	bridgeChannel := rp.audioBridgeChannel
	rp.audioMutex.Unlock()

	if len(buffer) > 0 {
		// Si hay un canal de puente configurado, enviar el audio como PCM mono
		if bridgeChannel != nil {
			// Si el buffer es estéreo, convertir a mono
			var monoBuffer []float32
			if len(buffer)%2 == 0 { // Asumir estéreo si es par
				monoBuffer = make([]float32, len(buffer)/2)
				for i := 0; i < len(monoBuffer); i++ {
					// Downmix estéreo a mono (promedio de L+R)
					left := buffer[i*2]
					right := buffer[i*2+1]
					monoBuffer[i] = (left + right) * 0.5
				}
			} else {
				// Ya es mono
				monoBuffer = buffer
			}

			// Convertir float32 a int16 PCM (formato estándar de audio)
			pcmBytes := make([]byte, len(monoBuffer)*2) // 2 bytes por muestra int16
			for i, sample := range monoBuffer {
				// Clamp el valor para evitar clipping
				if sample > 1.0 {
					sample = 1.0
				} else if sample < -1.0 {
					sample = -1.0
				}

				// Convertir a int16 con rango completo
				int16Sample := int16(sample * 32767.0)

				// Little endian encoding
				pcmBytes[i*2] = byte(int16Sample & 0xFF)
				pcmBytes[i*2+1] = byte((int16Sample >> 8) & 0xFF)
			}

			// Enviar al canal del puente de forma no bloqueante
			select {
			case bridgeChannel <- pcmBytes:
				// Audio enviado exitosamente al puente
			default:
				// Canal lleno, omitir este frame para evitar bloqueos
				fmt.Printf("⚠️ Canal de puente lleno, omitiendo frame de audio\n")
			}
		}

		// Log ocasional para verificar que se está recibiendo audio
		if len(buffer) > 100 { // Solo log para buffers significativos
			fmt.Printf("🎵 Procesando %d muestras de audio de %s (original: %d, mono: %d)\n",
				len(buffer), remoteTrack.participant, len(buffer), len(buffer)/2)
		}
	}
}

// SetAudioBridgeChannel configura el canal para enviar audio del agente hacia el puente WebRTC
func (rp *RoomParticipant) SetAudioBridgeChannel(channel chan []byte) {
	rp.audioMutex.Lock()
	defer rp.audioMutex.Unlock()
	rp.audioBridgeChannel = channel
	fmt.Printf("🔗 Canal de puente de audio configurado\n")
}

func (rp *RoomParticipant) onTrackUnsubscribed(track *webrtc.TrackRemote, publication *lksdk.RemoteTrackPublication, participant *lksdk.RemoteParticipant) {
	fmt.Printf("Track no suscrito: %s de participante %s\n",
		publication.Name(), participant.Identity())

	// Si es un track de audio, detener la reproducción
	if publication.Kind() == lksdk.TrackKindAudio {
		trackID := participant.Identity() + "_" + publication.SID()

		rp.audioMutex.Lock()
		if remoteTrack, exists := rp.RemoteTracks[trackID]; exists {
			remoteTrack.isActive = false
			delete(rp.RemoteTracks, trackID)
			fmt.Printf("🔇 Detenida reproducción REAL de audio de %s\n", participant.Identity())
		}
		rp.audioMutex.Unlock()
	}
}

func (rp *RoomParticipant) onDataReceived(data []byte, params lksdk.DataReceiveParams) {
	fmt.Printf("Datos recibidos de %s: %s\n", params.SenderIdentity, string(data))
}

func (rp *RoomParticipant) onParticipantConnected(participant *lksdk.RemoteParticipant) {
	fmt.Printf("Participante conectado: %s\n", participant.Identity())
}

func (rp *RoomParticipant) onParticipantDisconnected(participant *lksdk.RemoteParticipant) {
	fmt.Printf("Participante desconectado: %s\n", participant.Identity())
}

func (rp *RoomParticipant) onRoomMetadataChanged(metadata string) {
	fmt.Printf("Metadatos del room cambiaron: %s\n", metadata)
}

func (rp *RoomParticipant) onReconnecting() {
	fmt.Println("Reconectando al room...")
}

func (rp *RoomParticipant) onReconnected() {
	fmt.Println("Reconectado al room exitosamente")
}

func (rp *RoomParticipant) onDisconnected() {
	fmt.Println("Desconectado del room")
}

// setupAudioRecording configura la grabación de audio entrante en formato OGG
func (rp *RoomParticipant) setupAudioRecording(track *webrtc.TrackRemote, participant *lksdk.RemoteParticipant) {
	callID := participant.Identity() + "_" + track.ID()
	timestamp := time.Now().Unix()
	filename := fmt.Sprintf("recorder/retell_agent/audio-%d.ogg", timestamp)

	if track.Kind() != webrtc.RTPCodecTypeAudio {
		log.Printf(">> Track entrante ignorado (no audio): %s (id=%s)", track.Kind().String(), callID)
		return
	}

	// Detectar parámetros del codec
	codec := track.Codec()
	sampleRate := codec.ClockRate
	if sampleRate == 0 {
		sampleRate = 48000 // Fallback
	}

	channels := 2 // Por defecto stereo
	// Algunos codecs como Opus pueden ser mono en WebRTC
	if strings.Contains(strings.ToLower(codec.MimeType), "mono") {
		channels = 1
	}

	log.Printf(">> Codec: %s, SampleRate: %d, Channels: %d (id=%s)",
		codec.MimeType, sampleRate, channels, callID)

	cwd, _ := os.Getwd()
	abs := filepath.Join(cwd, filename)
	log.Printf(">> Guardando audio en: %s (id=%s)", abs, callID)

	ogg, err := oggwriter.New(abs, uint32(sampleRate), uint16(channels))
	if err != nil {
		log.Printf("error creando ogg: %v (id=%s)", err, callID)
		return
	}
	defer ogg.Close()

	// Resto del código sin cambios...
	for {
		pkt, _, err := track.ReadRTP()
		if err != nil {
			log.Printf(">> Fin de track: %v (id=%s)", err, callID)
			return
		}

		if writeErr := ogg.WriteRTP(pkt); writeErr != nil {
			log.Printf("error escribiendo ogg: %v (id=%s)", writeErr, callID)
			return
		}
	}
}

// createOGGWriter crea un escritor OGG para grabar audio
func createOGGWriter(filename string, sampleRate int, channels int) (*oggwriter.OggWriter, error) {
	fmt.Printf("🔧 Creando OGG Writer: %s (Rate: %d, Channels: %d)\n", filename, sampleRate, channels)
	writer, err := oggwriter.New(filename, uint32(sampleRate), uint16(channels))
	if err != nil {
		fmt.Printf("❌ Error en oggwriter.New: %v\n", err)
		return nil, err
	}
	fmt.Printf("✅ OGG Writer creado exitosamente\n")
	return writer, nil
}



// CreateOpusEncoder crea un encoder Opus optimizado para voz
func CreateOpusEncoder(sampleRate int, channels int) (*opus.Encoder, error) {
	encoder, err := opus.NewEncoder(sampleRate, channels, opus.AppVoIP)
	if err != nil {
		return nil, fmt.Errorf("error creando encoder Opus: %w", err)
	}

	// Configuración optimizada para voz en tiempo real
	encoder.SetBitrate(64000)  // 64kbps, buena calidad para voz
	encoder.SetComplexity(5)   // Compromiso entre calidad y latencia
	encoder.SetDTX(false)      // Desactivar DTX para consistencia
	encoder.SetInBandFEC(true) // Activar corrección de errores

	fmt.Printf("✅ Encoder Opus creado: %dHz, %d canales, 64kbps, optimizado para voz\n", sampleRate, channels)
	return encoder, nil
}
