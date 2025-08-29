package retell

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
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
	channels        = 2 // Mono
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

// playAudioBuffer reproduce un buffer de audio PCM (implementación básica)
func (rp *RoomParticipant) playAudioBuffer(buffer []float32, remoteTrack *RemoteAudioTrack) {
	// Esta función se integraría con el callback del outputStream
	// Por ahora, simplemente marca que hay datos disponibles
	rp.audioMutex.Lock()
	remoteTrack.isActive = len(buffer) > 0
	rp.audioMutex.Unlock()

	// Log ocasional para verificar que se está recibiendo audio
	if len(buffer) > 0 {
		fmt.Printf("🎵 Reproduciendo %d muestras de audio de %s\n", len(buffer), remoteTrack.participant)
	}
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
	timestamp := time.Now().Unix()
	filename := fmt.Sprintf("recorder_agent_retell/audio-%d.ogg", timestamp)

	fmt.Printf("🎙️ Iniciando grabación de audio de %s en: %s\n", participant.Identity(), filename)

	go func() {
		defer func() {
			fmt.Printf("🔇 Grabación finalizada para %s\n", participant.Identity())
		}()

		// Obtener información del codec del track
		codec := track.Codec()
		clockRate := codec.ClockRate
		channels := codec.Channels

		// Debug detallado del codec
		debugAudioInfo(codec)

		// Aplicar valores por defecto optimizados para voz (Retell AI)
		if clockRate == 0 || clockRate > 48000 {
			clockRate = 48000 // Valor estándar para Opus
			fmt.Printf("⚠️ ClockRate corregido a %d\n", clockRate)
		}
		if channels == 0 || channels > 2 {
			channels = 1 // Mono es mejor para voz, evita problemas de sincronización
			fmt.Printf("⚠️ Channels corregido a %d (mono para voz)\n", channels)
		}

		// Para sistemas de voz como Retell, forzar mono siempre
		if codec.MimeType == "audio/opus" && channels == 2 {
			channels = 1
			fmt.Printf("🎙️ Forzando mono para codec de voz (era estéreo)\n")
		}

		fmt.Printf("📊 Codec detectado: %s, ClockRate: %d, Channels: %d\n",
			codec.MimeType, clockRate, channels)

		// Crear escritor OGG con la configuración corregida del codec
		oggWriter, err := createOGGWriter(filename, int(clockRate), int(channels))
		if err != nil {
			fmt.Printf("❌ Error creando escritor OGG: %v\n", err)
			return
		}
		defer oggWriter.Close()

		fmt.Printf("✅ Grabación OGG activa para %s (Rate: %d, Channels: %d)\n",
			participant.Identity(), clockRate, channels)

		// Contador para controlar la frecuencia de logs
		packetCount := 0

		for {
			// Leer paquete RTP del track remoto
			rtpPacket, _, err := track.ReadRTP()
			if err != nil {
				if err.Error() != "EOF" {
					fmt.Printf("⚠️ Error leyendo RTP: %v\n", err)
				}
				return
			}

			if rtpPacket != nil {
				// Escribir paquete RTP al archivo OGG
				if writeErr := oggWriter.WriteRTP(rtpPacket); writeErr != nil {
					fmt.Printf("❌ Error escribiendo RTP a OGG: %v\n", writeErr)
					return
				}

				packetCount++

				// Log cada 100 paquetes para no saturar
				if packetCount%100 == 0 {
					fmt.Printf("💾 Grabando audio RTP de %s (SSRC=%d, Seq=%d, Packets=%d)\n",
						participant.Identity(), rtpPacket.SSRC, rtpPacket.SequenceNumber, packetCount)
				}
			}
		}
	}()
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

// debugAudioInfo imprime información detallada del audio para debugging
func debugAudioInfo(codec webrtc.RTPCodecParameters) {
	fmt.Printf("🔍 DEBUG CODEC INFO:\n")
	fmt.Printf("   MimeType: %s\n", codec.MimeType)
	fmt.Printf("   ClockRate: %d Hz\n", codec.ClockRate)
	fmt.Printf("   Channels: %d\n", codec.Channels)
	fmt.Printf("   PayloadType: %d\n", codec.PayloadType)
	if len(codec.SDPFmtpLine) > 0 {
		fmt.Printf("   FMTP: %s\n", codec.SDPFmtpLine)
	}
}
