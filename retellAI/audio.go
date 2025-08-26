package retellAI

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
)

// AudioProcessor handles audio processing and streaming
type AudioProcessor struct {
	sampleRate   int
	channels     int
	bitDepth     int
	bufferSize   int
	isProcessing bool
	mutex        sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
}

// NewAudioProcessor creates a new audio processor
func NewAudioProcessor(sampleRate, channels, bitDepth, bufferSize int) *AudioProcessor {
	ctx, cancel := context.WithCancel(context.Background())

	return &AudioProcessor{
		sampleRate:   sampleRate,
		channels:     channels,
		bitDepth:     bitDepth,
		bufferSize:   bufferSize,
		isProcessing: false,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start begins audio processing
func (ap *AudioProcessor) Start() error {
	ap.mutex.Lock()
	defer ap.mutex.Unlock()

	if ap.isProcessing {
		return fmt.Errorf("audio processor already running")
	}

	ap.isProcessing = true
	log.Printf("Audio processor started - Sample Rate: %d, Channels: %d, Bit Depth: %d",
		ap.sampleRate, ap.channels, ap.bitDepth)

	return nil
}

// Stop stops audio processing
func (ap *AudioProcessor) Stop() {
	ap.mutex.Lock()
	defer ap.mutex.Unlock()

	if !ap.isProcessing {
		return
	}

	ap.cancel()
	ap.isProcessing = false
	log.Println("Audio processor stopped")
}

// ProcessAudioSamples processes raw audio samples
func (ap *AudioProcessor) ProcessAudioSamples(samples []float32) []float32 {
	if !ap.isProcessing {
		return samples
	}

	// Apply basic audio processing (normalize, filter, etc.)
	processed := make([]float32, len(samples))
	copy(processed, samples)

	// Apply normalization
	maxVal := float32(0.0)
	for _, sample := range processed {
		if abs := float32(math.Abs(float64(sample))); abs > maxVal {
			maxVal = abs
		}
	}

	if maxVal > 0 {
		normalizer := float32(0.8) / maxVal
		for i := range processed {
			processed[i] *= normalizer
		}
	}

	return processed
}

// CalculateVolume calculates the RMS volume of audio samples
func (ap *AudioProcessor) CalculateVolume(samples []float32) float64 {
	if len(samples) == 0 {
		return 0.0
	}

	sum := float64(0)
	for _, sample := range samples {
		sum += float64(sample) * float64(sample)
	}

	rms := math.Sqrt(sum / float64(len(samples)))

	// Convert to dB scale
	if rms > 0 {
		return 20 * math.Log10(rms)
	}

	return -100.0 // Silent
}

// AudioStreamManager manages audio streaming for WebRTC
type AudioStreamManager struct {
	track       *webrtc.TrackLocalStaticSample
	processor   *AudioProcessor
	isStreaming bool
	mutex       sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewAudioStreamManager creates a new audio stream manager
func NewAudioStreamManager() *AudioStreamManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &AudioStreamManager{
		isStreaming: false,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// CreateAudioTrack creates a new local audio track for streaming
func (asm *AudioStreamManager) CreateAudioTrack(codec webrtc.RTPCodecCapability) (*webrtc.TrackLocalStaticSample, error) {
	var err error
	asm.track, err = webrtc.NewTrackLocalStaticSample(codec, "audio", "retell-audio")
	if err != nil {
		return nil, fmt.Errorf("failed to create audio track: %v", err)
	}

	log.Printf("Audio track created with codec: %s", codec.MimeType)
	return asm.track, nil
}

// StartStreaming starts streaming audio samples to the track
func (asm *AudioStreamManager) StartStreaming(audioSamples <-chan []float32) error {
	asm.mutex.Lock()
	defer asm.mutex.Unlock()

	if asm.isStreaming {
		return fmt.Errorf("audio streaming already started")
	}

	if asm.track == nil {
		return fmt.Errorf("no audio track available")
	}

	asm.isStreaming = true

	// Start streaming goroutine
	go asm.streamAudioSamples(audioSamples)

	log.Println("Audio streaming started")
	return nil
}

// StopStreaming stops audio streaming
func (asm *AudioStreamManager) StopStreaming() {
	asm.mutex.Lock()
	defer asm.mutex.Unlock()

	if !asm.isStreaming {
		return
	}

	asm.cancel()
	asm.isStreaming = false
	log.Println("Audio streaming stopped")
}

// streamAudioSamples streams audio samples to the WebRTC track
func (asm *AudioStreamManager) streamAudioSamples(audioSamples <-chan []float32) {
	ticker := time.NewTicker(20 * time.Millisecond) // 50 FPS
	defer ticker.Stop()

	for {
		select {
		case <-asm.ctx.Done():
			return
		case <-ticker.C:
			select {
			case samples := <-audioSamples:
				if err := asm.writeSamplesToTrack(samples); err != nil {
					log.Printf("Error writing samples to track: %v", err)
					return
				}
			default:
				// No samples available, continue
			}
		}
	}
}

// writeSamplesToTrack converts float32 samples to RTP packets and writes to track
func (asm *AudioStreamManager) writeSamplesToTrack(samples []float32) error {
	if asm.track == nil {
		return fmt.Errorf("no track available")
	}

	// Convert float32 samples to PCM 16-bit
	pcmData := make([]byte, len(samples)*2)
	for i, sample := range samples {
		// Clamp sample to [-1, 1] range
		if sample > 1.0 {
			sample = 1.0
		} else if sample < -1.0 {
			sample = -1.0
		}

		// Convert to 16-bit PCM
		pcmValue := int16(sample * 32767)
		pcmData[i*2] = byte(pcmValue)
		pcmData[i*2+1] = byte(pcmValue >> 8)
	}

	// Create media sample
	sample := media.Sample{
		Data:     pcmData,
		Duration: time.Millisecond * 20, // 20ms per packet
	}

	// Write sample to track
	return asm.track.WriteSample(sample)
}

// IsStreaming returns whether audio is currently streaming
func (asm *AudioStreamManager) IsStreaming() bool {
	asm.mutex.RLock()
	defer asm.mutex.RUnlock()
	return asm.isStreaming
}

// GetTrack returns the current audio track
func (asm *AudioStreamManager) GetTrack() *webrtc.TrackLocalStaticSample {
	asm.mutex.RLock()
	defer asm.mutex.RUnlock()
	return asm.track
}

// AudioRecorder records audio from a WebRTC track
type AudioRecorder struct {
	isRecording bool
	samples     [][]float32
	mutex       sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewAudioRecorder creates a new audio recorder
func NewAudioRecorder() *AudioRecorder {
	ctx, cancel := context.WithCancel(context.Background())

	return &AudioRecorder{
		isRecording: false,
		samples:     make([][]float32, 0),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// StartRecording starts recording audio from a track
func (ar *AudioRecorder) StartRecording(track *webrtc.TrackRemote) error {
	ar.mutex.Lock()
	defer ar.mutex.Unlock()

	if ar.isRecording {
		return fmt.Errorf("recording already in progress")
	}

	ar.isRecording = true
	ar.samples = make([][]float32, 0)

	// Start recording goroutine
	go ar.recordFromTrack(track)

	log.Println("Audio recording started")
	return nil
}

// StopRecording stops recording and returns recorded samples
func (ar *AudioRecorder) StopRecording() [][]float32 {
	ar.mutex.Lock()
	defer ar.mutex.Unlock()

	if !ar.isRecording {
		return nil
	}

	ar.cancel()
	ar.isRecording = false

	recordedSamples := make([][]float32, len(ar.samples))
	copy(recordedSamples, ar.samples)

	log.Printf("Audio recording stopped - %d sample chunks recorded", len(recordedSamples))
	return recordedSamples
}

// recordFromTrack records audio samples from a WebRTC track
func (ar *AudioRecorder) recordFromTrack(track *webrtc.TrackRemote) {
	for {
		select {
		case <-ar.ctx.Done():
			return
		default:
			rtpPacket, _, err := track.ReadRTP()
			if err != nil {
				log.Printf("Error reading RTP packet: %v", err)
				return
			}

			// Convert RTP payload to float32 samples
			samples := ar.convertRTPToSamples(rtpPacket.Payload)
			if len(samples) > 0 {
				ar.mutex.Lock()
				ar.samples = append(ar.samples, samples)
				ar.mutex.Unlock()
			}
		}
	}
}

// convertRTPToSamples converts RTP payload to float32 audio samples
func (ar *AudioRecorder) convertRTPToSamples(payload []byte) []float32 {
	// Simple conversion from PCM 16-bit to float32
	if len(payload)%2 != 0 {
		return nil
	}

	samples := make([]float32, len(payload)/2)
	for i := 0; i < len(samples); i++ {
		// Convert little-endian 16-bit PCM to float32
		pcmValue := int16(payload[i*2]) | (int16(payload[i*2+1]) << 8)
		samples[i] = float32(pcmValue) / 32767.0
	}

	return samples
}

// IsRecording returns whether recording is in progress
func (ar *AudioRecorder) IsRecording() bool {
	ar.mutex.RLock()
	defer ar.mutex.RUnlock()
	return ar.isRecording
}

// GetSampleCount returns the number of recorded sample chunks
func (ar *AudioRecorder) GetSampleCount() int {
	ar.mutex.RLock()
	defer ar.mutex.RUnlock()
	return len(ar.samples)
}
