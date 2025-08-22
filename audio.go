package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/hajimehoshi/oto"
	"github.com/hraban/opus"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
	"github.com/pion/webrtc/v3/pkg/media/oggreader"
	"github.com/pion/webrtc/v3/pkg/media/oggwriter"
)

// ========================= Funciones de Audio =========================

// RealTimeAudioPlayer maneja la reproducción de audio en tiempo real
type RealTimeAudioPlayer struct {
	context   *oto.Context
	player    *oto.Player
	decoder   *opus.Decoder
	isPlaying bool
	mutex     sync.RWMutex
}

// createRealTimePlayer crea un nuevo reproductor de audio en tiempo real
func createRealTimePlayer() (*RealTimeAudioPlayer, error) {
	// Configurar el contexto de audio: 48kHz, 2 canales, 16-bit samples, buffer 8192
	ctx, err := oto.NewContext(48000, 2, 2, 8192)
	if err != nil {
		return nil, fmt.Errorf("error creando contexto de audio: %w", err)
	}

	// Crear decodificador Opus (48kHz, 2 canales)
	decoder, err := opus.NewDecoder(48000, 2)
	if err != nil {
		return nil, fmt.Errorf("error creando decodificador Opus: %w", err)
	}

	return &RealTimeAudioPlayer{
		context:   ctx,
		decoder:   decoder,
		isPlaying: false,
	}, nil
}

// start inicia la reproducción
func (rtp *RealTimeAudioPlayer) start() error {
	rtp.mutex.Lock()
	defer rtp.mutex.Unlock()

	if rtp.isPlaying {
		return nil // Ya está reproduciendo
	}

	// Crear un nuevo player
	rtp.player = rtp.context.NewPlayer()
	rtp.isPlaying = true

	return nil
}

// playOpusData decodifica y reproduce datos Opus
func (rtp *RealTimeAudioPlayer) playOpusData(opusData []byte) error {
	rtp.mutex.RLock()
	if !rtp.isPlaying || rtp.player == nil {
		rtp.mutex.RUnlock()
		return nil
	}
	player := rtp.player
	rtp.mutex.RUnlock()

	// Decodificar datos Opus a PCM
	pcmData := make([]int16, 5760) // Buffer para 120ms a 48kHz stereo
	n, err := rtp.decoder.Decode(opusData, pcmData)
	if err != nil {
		return fmt.Errorf("error decodificando Opus: %w", err)
	}

	// Convertir int16 a bytes (little endian)
	byteData := make([]byte, n*4) // 2 canales * 2 bytes por sample
	for i := 0; i < n*2; i++ {
		sample := pcmData[i]
		byteData[i*2] = byte(sample & 0xFF)
		byteData[i*2+1] = byte((sample >> 8) & 0xFF)
	}

	// Escribir al player
	_, err = player.Write(byteData)
	return err
}

// stop detiene la reproducción
func (rtp *RealTimeAudioPlayer) stop() {
	rtp.mutex.Lock()
	defer rtp.mutex.Unlock()

	if !rtp.isPlaying {
		return
	}

	rtp.isPlaying = false

	if rtp.player != nil {
		rtp.player.Close()
		rtp.player = nil
	}
}

// playAudioFile reproduce un archivo de audio usando el reproductor del sistema
func playAudioFile(filepath string, callID string) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin": // macOS
		// Usar 'open' que abre con la aplicación predeterminada
		cmd = exec.Command("open", filepath)
	case "linux":
		// Intentar varios reproductores comunes en Linux
		if _, err := exec.LookPath("xdg-open"); err == nil {
			cmd = exec.Command("xdg-open", filepath)
		} else if _, err := exec.LookPath("ffplay"); err == nil {
			cmd = exec.Command("ffplay", "-nodisp", "-autoexit", filepath)
		} else if _, err := exec.LookPath("vlc"); err == nil {
			cmd = exec.Command("vlc", "--intf", "dummy", "--play-and-exit", filepath)
		} else {
			log.Printf(">> No se encontró reproductor de audio compatible en Linux (id=%s)", callID)
			return
		}
	case "windows":
		// En Windows usar el comando start para abrir con la aplicación predeterminada
		cmd = exec.Command("cmd", "/c", "start", "", filepath)
	default:
		log.Printf(">> Sistema operativo no soportado para reproducción: %s (id=%s)", runtime.GOOS, callID)
		return
	}

	log.Printf(">> Ejecutando comando de reproducción: %v (id=%s)", cmd.Args, callID)
	if err := cmd.Run(); err != nil {
		log.Printf(">> Error reproduciendo audio: %v (id=%s)", err, callID)
	} else {
		log.Printf(">> Reproducción de audio iniciada: %s (id=%s)", filepath, callID)
	}
}

// setupAudioReceiver configura el receptor de audio para guardar en OGG y reproducir en tiempo real
func setupAudioReceiver(peer *webrtc.PeerConnection, callID string) {
	peer.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if track.Kind() != webrtc.RTPCodecTypeAudio {
			log.Printf(">> Track entrante ignorado (no audio): %s (id=%s)", track.Kind().String(), callID)
			return
		}

		cwd, _ := os.Getwd()
		filename := fmt.Sprintf("recorder/audio-%d.ogg", time.Now().Unix())
		abs := filepath.Join(cwd, filename)
		log.Printf(">> Audio entrante detectado, guardando en: %s (codec=%s) (id=%s)", abs, track.Codec().MimeType, callID)

		// Crear directorio recorder si no existe
		if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
			log.Printf("error creando directorio: %v (id=%s)", err, callID)
			return
		}

		// Configurar grabación en OGG
		ogg, err := oggwriter.New(abs, 48000, 2)
		if err != nil {
			log.Printf("error creando ogg: %v (id=%s)", err, callID)
			return
		}
		defer func() {
			ogg.Close()
			log.Printf(">> Grabación finalizada: %s (id=%s)", abs, callID)
		}()

		// Configurar reproductor en tiempo real
		rtPlayer, err := createRealTimePlayer()
		if err != nil {
			log.Printf("error creando reproductor en tiempo real: %v (id=%s)", err, callID)
			// Continuar sin reproductor si falla
		} else {
			if err := rtPlayer.start(); err != nil {
				log.Printf("error iniciando reproductor: %v (id=%s)", err, callID)
			} else {
				log.Printf(">> Reproductor en tiempo real iniciado (id=%s)", callID)
				defer func() {
					rtPlayer.stop()
					log.Printf(">> Reproductor en tiempo real detenido (id=%s)", callID)
				}()
			}
		}

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

			log.Printf(">> RTP recibido: SSRC=%d Seq=%d TS=%d Payload=%d bytes (id=%s)",
				pkt.SSRC, pkt.SequenceNumber, pkt.Timestamp, len(pkt.Payload), callID)

			// Guardar en OGG
			if writeErr := ogg.WriteRTP(pkt); writeErr != nil {
				log.Printf("error escribiendo ogg: %v (id=%s)", writeErr, callID)
				return
			}

			// Reproducir en tiempo real si el reproductor está disponible
			if rtPlayer != nil && rtPlayer.isPlaying {
				// El payload contiene datos Opus, reproducir directamente
				if playErr := rtPlayer.playOpusData(pkt.Payload); playErr != nil {
					log.Printf("error reproduciendo audio en tiempo real: %v (id=%s)", playErr, callID)
					// No retornar, continuar grabando aunque falle la reproducción
				}
			}
		}
	})
}

// setupAudioSender configura el emisor de audio para enviar OGG
func setupAudioSender(peer *webrtc.PeerConnection, audioTrans *webrtc.RTPTransceiver, callID string) error {
	if OutOGGPath == "" || audioTrans == nil {
		return nil
	}

	log.Printf(">> OUTGOING: preparado para enviar OGG='%s' timeout=%ds (id=%s)", OutOGGPath, OutTimeoutSec, callID)

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
		return fmt.Errorf("NewTrackLocalStaticSample error: %w", err)
	}

	if err := audioTrans.Sender().ReplaceTrack(trackLocal); err != nil {
		return fmt.Errorf("ReplaceTrack error: %w", err)
	}

	// Drenar RTCP para evitar bloqueo del sender
	go drainRTCP(audioTrans.Sender())

	// IMPORTANTE: empieza a enviar SOLO cuando la PC está conectada
	peer.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		if s == webrtc.PeerConnectionStateConnected {
			log.Printf(">> OUTGOING: conexión lista, comenzando envío OGG (id=%s)", callID)
			go sendOGGAudio(trackLocal, callID)
		}
	})

	return nil
}

// sendOGGAudio envía audio desde un archivo OGG
func sendOGGAudio(trackLocal *webrtc.TrackLocalStaticSample, callID string) {
	f, err := os.Open(OutOGGPath)
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

	// Timeout opcional
	var timeout <-chan time.Time
	if OutTimeoutSec > 0 {
		t := time.NewTimer(time.Duration(OutTimeoutSec) * time.Second)
		defer t.Stop()
		timeout = t.C
	}

	frame := time.Duration(AudioFrameTime) * time.Millisecond // pacing típico Opus

	for {
		select {
		case <-timeout:
			log.Printf(">> OUTGOING: timeout alcanzado (%ds) (id=%s)", OutTimeoutSec, callID)
			return
		default:
		}

		// Lee siguiente página OGG (payload Opus)
		pageData, _, err := r.ParseNextPage()
		if err == io.EOF {
			log.Printf(">> OUTGOING: EOF OGG %s (id=%s)", OutOGGPath, callID)
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
}

// drainRTCP lee RTCP para que el sender no se bloquee
func drainRTCP(ss *webrtc.RTPSender) {
	buf := make([]byte, 1500)
	for {
		if _, _, err := ss.Read(buf); err != nil {
			return // sender cerrado
		}
	}
}

// attachOGGToTransceiver adjunta una pista local Opus (Sample) al transceiver de audio y
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
		frame := time.Duration(AudioFrameTime) * time.Millisecond

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
