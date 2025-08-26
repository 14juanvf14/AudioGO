package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gordonklaus/portaudio"
	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
)

const (
	sampleRate = 16000
	channels   = 1
	frameSize  = 1024
)

// AudioDevice represents an audio device
type AudioDevice struct {
	Index      int    `json:"index"`
	Name       string `json:"name"`
	HostAPI    string `json:"hostApi"`
	MaxInputs  int    `json:"maxInputs"`
	MaxOutputs int    `json:"maxOutputs"`
	IsDefault  bool   `json:"isDefault"`
	IsInput    bool   `json:"isInput"`
	IsOutput   bool   `json:"isOutput"`
}

// SystemAudioManager manages real audio input/output
type SystemAudioManager struct {
	// Input (microphone)
	inputStream   *portaudio.Stream
	inputDevice   *AudioDevice
	inputBuffer   []float32
	inputCallback func([]float32)

	// Output (speakers)
	outputStream *portaudio.Stream
	outputDevice *AudioDevice
	outputBuffer chan []float32

	// WebRTC integration
	audioTrack *webrtc.TrackLocalStaticSample

	// State management
	isCapturing bool
	isPlaying   bool
	mutex       sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewSystemAudioManager creates a new system audio manager
func NewSystemAudioManager() *SystemAudioManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &SystemAudioManager{
		inputBuffer:  make([]float32, frameSize),
		outputBuffer: make(chan []float32, 100), // Buffer for 100 frames
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Initialize initializes PortAudio
func (sam *SystemAudioManager) Initialize() error {
	log.Println("🎤 Initializing PortAudio...")
	
	if err := portaudio.Initialize(); err != nil {
		log.Printf("⚠️ Failed to initialize PortAudio: %v", err)
		log.Println("📝 Note: PortAudio may require microphone permissions on macOS")
		log.Println("   Go to System Preferences > Privacy & Security > Microphone")
		log.Println("   and enable access for your terminal application.")
		return fmt.Errorf("failed to initialize PortAudio: %v", err)
	}
	
	log.Println("✅ PortAudio initialized successfully")
	return nil
}

// Terminate terminates PortAudio
func (sam *SystemAudioManager) Terminate() {
	sam.StopCapture()
	sam.StopPlayback()

	if err := portaudio.Terminate(); err != nil {
		log.Printf("❌ Error terminating PortAudio: %v", err)
	} else {
		log.Println("✅ PortAudio terminated")
	}
}

// GetAudioDevices returns all available audio devices
func (sam *SystemAudioManager) GetAudioDevices() ([]AudioDevice, error) {
	devices, err := portaudio.Devices()
	if err != nil {
		return nil, fmt.Errorf("failed to get audio devices: %v", err)
	}

	defaultInput, err := portaudio.DefaultInputDevice()
	if err != nil {
		log.Printf("⚠️ No default input device: %v", err)
	}

	defaultOutput, err := portaudio.DefaultOutputDevice()
	if err != nil {
		log.Printf("⚠️ No default output device: %v", err)
	}

	var audioDevices []AudioDevice
	for i, device := range devices {
		audioDevice := AudioDevice{
			Index:      i,
			Name:       device.Name,
			HostAPI:    device.HostApi.Name,
			MaxInputs:  device.MaxInputChannels,
			MaxOutputs: device.MaxOutputChannels,
			IsInput:    device.MaxInputChannels > 0,
			IsOutput:   device.MaxOutputChannels > 0,
		}

		// Check if it's the default device
		if defaultInput != nil && device == defaultInput {
			audioDevice.IsDefault = true
		}
		if defaultOutput != nil && device == defaultOutput {
			audioDevice.IsDefault = true
		}

		audioDevices = append(audioDevices, audioDevice)
	}

	return audioDevices, nil
}

// SetInputDevice sets the input device for audio capture
func (sam *SystemAudioManager) SetInputDevice(deviceIndex int) error {
	devices, err := portaudio.Devices()
	if err != nil {
		return fmt.Errorf("failed to get devices: %v", err)
	}

	if deviceIndex < 0 || deviceIndex >= len(devices) {
		return fmt.Errorf("invalid device index: %d", deviceIndex)
	}

	device := devices[deviceIndex]
	if device.MaxInputChannels == 0 {
		return fmt.Errorf("device %s is not an input device", device.Name)
	}

	sam.mutex.Lock()
	sam.inputDevice = &AudioDevice{
		Index:      deviceIndex,
		Name:       device.Name,
		HostAPI:    device.HostApi.Name,
		MaxInputs:  device.MaxInputChannels,
		MaxOutputs: device.MaxOutputChannels,
		IsInput:    true,
		IsOutput:   device.MaxOutputChannels > 0,
	}
	sam.mutex.Unlock()

	log.Printf("🎤 Input device set to: %s", device.Name)
	return nil
}

// SetOutputDevice sets the output device for audio playback
func (sam *SystemAudioManager) SetOutputDevice(deviceIndex int) error {
	devices, err := portaudio.Devices()
	if err != nil {
		return fmt.Errorf("failed to get devices: %v", err)
	}

	if deviceIndex < 0 || deviceIndex >= len(devices) {
		return fmt.Errorf("invalid device index: %d", deviceIndex)
	}

	device := devices[deviceIndex]
	if device.MaxOutputChannels == 0 {
		return fmt.Errorf("device %s is not an output device", device.Name)
	}

	sam.mutex.Lock()
	sam.outputDevice = &AudioDevice{
		Index:      deviceIndex,
		Name:       device.Name,
		HostAPI:    device.HostApi.Name,
		MaxInputs:  device.MaxInputChannels,
		MaxOutputs: device.MaxOutputChannels,
		IsInput:    device.MaxInputChannels > 0,
		IsOutput:   true,
	}
	sam.mutex.Unlock()

	log.Printf("🔊 Output device set to: %s", device.Name)
	return nil
}

// StartCapture starts capturing audio from the input device
func (sam *SystemAudioManager) StartCapture() error {
	sam.mutex.Lock()
	defer sam.mutex.Unlock()

	if sam.isCapturing {
		return fmt.Errorf("audio capture already running")
	}

	// Use default input device if none specified
	if sam.inputDevice == nil {
		defaultDevice, err := portaudio.DefaultInputDevice()
		if err != nil {
			return fmt.Errorf("no input device available: %v", err)
		}

		if err := sam.SetInputDevice(0); err != nil { // Find the default device index
			devices, _ := portaudio.Devices()
			for i, device := range devices {
				if device == defaultDevice {
					sam.SetInputDevice(i)
					break
				}
			}
		}
	}

	devices, err := portaudio.Devices()
	if err != nil {
		return fmt.Errorf("failed to get devices: %v", err)
	}

	inputDevice := devices[sam.inputDevice.Index]

	// Configure input stream parameters
	inputParams := portaudio.StreamParameters{
		Input: portaudio.StreamDeviceParameters{
			Device:   inputDevice,
			Channels: channels,
			Latency:  inputDevice.DefaultLowInputLatency,
		},
		SampleRate:      sampleRate,
		FramesPerBuffer: frameSize,
	}

	// Create input stream
	sam.inputStream, err = portaudio.OpenStream(inputParams, sam.audioInputCallback)
	if err != nil {
		return fmt.Errorf("failed to open input stream: %v", err)
	}

	// Start the stream
	if err := sam.inputStream.Start(); err != nil {
		sam.inputStream.Close()
		return fmt.Errorf("failed to start input stream: %v", err)
	}

	sam.isCapturing = true
	log.Printf("🎤 Audio capture started on device: %s", sam.inputDevice.Name)
	return nil
}

// StopCapture stops audio capture
func (sam *SystemAudioManager) StopCapture() {
	sam.mutex.Lock()
	defer sam.mutex.Unlock()

	if !sam.isCapturing {
		return
	}

	if sam.inputStream != nil {
		sam.inputStream.Stop()
		sam.inputStream.Close()
		sam.inputStream = nil
	}

	sam.isCapturing = false
	log.Println("🎤 Audio capture stopped")
}

// StartPlayback starts audio playback to the output device
func (sam *SystemAudioManager) StartPlayback() error {
	sam.mutex.Lock()
	defer sam.mutex.Unlock()

	if sam.isPlaying {
		return fmt.Errorf("audio playback already running")
	}

	// Use default output device if none specified
	if sam.outputDevice == nil {
		defaultDevice, err := portaudio.DefaultOutputDevice()
		if err != nil {
			return fmt.Errorf("no output device available: %v", err)
		}

		devices, _ := portaudio.Devices()
		for i, device := range devices {
			if device == defaultDevice {
				sam.SetOutputDevice(i)
				break
			}
		}
	}

	devices, err := portaudio.Devices()
	if err != nil {
		return fmt.Errorf("failed to get devices: %v", err)
	}

	outputDevice := devices[sam.outputDevice.Index]

	// Configure output stream parameters
	outputParams := portaudio.StreamParameters{
		Output: portaudio.StreamDeviceParameters{
			Device:   outputDevice,
			Channels: channels,
			Latency:  outputDevice.DefaultLowOutputLatency,
		},
		SampleRate:      sampleRate,
		FramesPerBuffer: frameSize,
	}

	// Create output stream
	sam.outputStream, err = portaudio.OpenStream(outputParams, sam.audioOutputCallback)
	if err != nil {
		return fmt.Errorf("failed to open output stream: %v", err)
	}

	// Start the stream
	if err := sam.outputStream.Start(); err != nil {
		sam.outputStream.Close()
		return fmt.Errorf("failed to start output stream: %v", err)
	}

	sam.isPlaying = true
	log.Printf("🔊 Audio playback started on device: %s", sam.outputDevice.Name)
	return nil
}

// StopPlayback stops audio playback
func (sam *SystemAudioManager) StopPlayback() {
	sam.mutex.Lock()
	defer sam.mutex.Unlock()

	if !sam.isPlaying {
		return
	}

	if sam.outputStream != nil {
		sam.outputStream.Stop()
		sam.outputStream.Close()
		sam.outputStream = nil
	}

	sam.isPlaying = false
	log.Println("🔊 Audio playback stopped")
}

// audioInputCallback is called when audio input is available
func (sam *SystemAudioManager) audioInputCallback(inputBuffer []float32) {
	// Copy the input buffer
	audioData := make([]float32, len(inputBuffer))
	copy(audioData, inputBuffer)

	// Call the registered callback if available
	if sam.inputCallback != nil {
		sam.inputCallback(audioData)
	}

	// Send audio to WebRTC track if available
	if sam.audioTrack != nil {
		sam.sendAudioToWebRTC(audioData)
	}
}

// audioOutputCallback is called when audio output is needed
func (sam *SystemAudioManager) audioOutputCallback(outputBuffer []float32) {
	// Try to get audio data from the buffer
	select {
	case audioData := <-sam.outputBuffer:
		// Copy audio data to output buffer
		copy(outputBuffer, audioData)

		// If audioData is shorter than outputBuffer, fill the rest with silence
		if len(audioData) < len(outputBuffer) {
			for i := len(audioData); i < len(outputBuffer); i++ {
				outputBuffer[i] = 0
			}
		}
	default:
		// No audio data available, output silence
		for i := range outputBuffer {
			outputBuffer[i] = 0
		}
	}
}

// sendAudioToWebRTC sends audio data to WebRTC track
func (sam *SystemAudioManager) sendAudioToWebRTC(audioData []float32) {
	if sam.audioTrack == nil {
		return
	}

	// Convert float32 samples to PCM 16-bit
	pcmData := make([]byte, len(audioData)*2)
	for i, sample := range audioData {
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
		Duration: time.Duration(len(audioData)) * time.Second / sampleRate,
	}

	// Write sample to track
	if err := sam.audioTrack.WriteSample(sample); err != nil {
		log.Printf("⚠️ Failed to write audio sample to WebRTC: %v", err)
	}
}

// SetWebRTCTrack sets the WebRTC track for audio streaming
func (sam *SystemAudioManager) SetWebRTCTrack(track *webrtc.TrackLocalStaticSample) {
	sam.mutex.Lock()
	defer sam.mutex.Unlock()
	sam.audioTrack = track
	log.Println("🌐 WebRTC audio track set")
}

// SetInputCallback sets a callback function for input audio data
func (sam *SystemAudioManager) SetInputCallback(callback func([]float32)) {
	sam.mutex.Lock()
	defer sam.mutex.Unlock()
	sam.inputCallback = callback
}

// QueueOutputAudio queues audio data for output playback
func (sam *SystemAudioManager) QueueOutputAudio(audioData []float32) {
	if !sam.isPlaying {
		return
	}

	select {
	case sam.outputBuffer <- audioData:
		// Audio queued successfully
	default:
		// Buffer is full, skip this frame
		log.Printf("⚠️ Audio output buffer full, skipping frame")
	}
}

// GetStatus returns the current status of the audio manager
func (sam *SystemAudioManager) GetStatus() map[string]interface{} {
	sam.mutex.RLock()
	defer sam.mutex.RUnlock()

	status := map[string]interface{}{
		"isCapturing": sam.isCapturing,
		"isPlaying":   sam.isPlaying,
		"sampleRate":  sampleRate,
		"channels":    channels,
		"frameSize":   frameSize,
	}

	if sam.inputDevice != nil {
		status["inputDevice"] = sam.inputDevice
	}

	if sam.outputDevice != nil {
		status["outputDevice"] = sam.outputDevice
	}

	return status
}

// IsCapturing returns whether audio capture is active
func (sam *SystemAudioManager) IsCapturing() bool {
	sam.mutex.RLock()
	defer sam.mutex.RUnlock()
	return sam.isCapturing
}

// IsPlaying returns whether audio playback is active
func (sam *SystemAudioManager) IsPlaying() bool {
	sam.mutex.RLock()
	defer sam.mutex.RUnlock()
	return sam.isPlaying
}
