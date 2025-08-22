package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
	"github.com/pion/webrtc/v3/pkg/media/oggreader"
	"github.com/pion/webrtc/v3/pkg/media/oggwriter"
)

// ========================= Funciones de Audio =========================

// setupAudioReceiver configura el receptor de audio para guardar en OGG
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

	// NOTA: El audio ahora se inicia desde setupPeerCallbacks cuando ICE está conectado
	// Esto mejora la latencia en ~1-2 segundos comparado con esperar PeerConnectionStateConnected

	return nil
}

// startAudioSending inicia el envío de audio usando la track ya configurada
func startAudioSending(track webrtc.TrackLocal, callID string) {
	if OutOGGPath == "" {
		log.Printf(">> OUTGOING: No hay archivo OGG configurado (id=%s)", callID)
		return
	}

	// Verificar que es una TrackLocalStaticSample
	trackLocal, ok := track.(*webrtc.TrackLocalStaticSample)
	if !ok {
		log.Printf(">> OUTGOING: Track no es StaticSample (id=%s)", callID)
		return
	}

	log.Printf(">> OUTGOING: iniciando envío anticipado de OGG='%s' (id=%s)", OutOGGPath, callID)
	sendOGGAudio(trackLocal, callID)
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
