package retellAI

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

const (
	hostURL = "wss://retell-ai-4ihahnq7.livekit.cloud"
)

// RetellWebClient is the main client for Retell AI calls
type RetellWebClient struct {
	*EventEmitter

	// WebRTC related
	peerConnection *webrtc.PeerConnection
	dataChannel    *webrtc.DataChannel
	wsConn         *websocket.Conn

	// State management
	connected       bool
	isAgentTalking  bool
	connectionState string

	// Audio related
	agentAudioTrack   *webrtc.TrackRemote
	agentMediaStream  *MediaStreamTrack
	analyzerComponent *AnalyzerComponent

	// Custom media stream
	customMediaStream *MediaStreamTrack

	// Synchronization
	mutex sync.RWMutex

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

// NewRetellWebClient creates a new RetellWebClient instance
func NewRetellWebClient() *RetellWebClient {
	ctx, cancel := context.WithCancel(context.Background())

	return &RetellWebClient{
		EventEmitter:    NewEventEmitter(),
		connected:       false,
		isAgentTalking:  false,
		connectionState: ConnectionStateDisconnected,
		ctx:             ctx,
		cancel:          cancel,
	}
}

// StartCall initiates a call with the provided configuration
func (r *RetellWebClient) StartCall(config StartCallConfig) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.connected {
		return errors.New("call already in progress")
	}

	log.Printf("Starting call with config: %+v", config)

	// Create WebRTC peer connection
	if err := r.createPeerConnection(); err != nil {
		r.Emit(EventError, fmt.Sprintf("Error creating peer connection: %v", err))
		return err
	}

	// Setup event handlers
	r.setupWebRTCHandlers()

	// Setup audio handling based on config
	if config.EmitRawAudioSamples {
		r.setupAudioAnalyzer()
	}

	// Connect to WebSocket (simulated LiveKit connection)
	if err := r.connectWebSocket(config.AccessToken); err != nil {
		r.Emit(EventError, fmt.Sprintf("Error connecting to WebSocket: %v", err))
		return err
	}

	// Setup data channel for server communication
	if err := r.setupDataChannel(); err != nil {
		r.Emit(EventError, fmt.Sprintf("Error setting up data channel: %v", err))
		return err
	}

	// Enable microphone
	if err := r.enableMicrophone(true); err != nil {
		r.Emit(EventError, fmt.Sprintf("Error enabling microphone: %v", err))
		return err
	}

	r.connected = true
	r.connectionState = ConnectionStateConnected
	r.Emit(EventCallStarted, nil)

	log.Println("Call started successfully")
	return nil
}

// StartAudioPlayback starts audio playback (equivalent to JS startAudioPlayback)
func (r *RetellWebClient) StartAudioPlayback() error {
	// In a real implementation, this would enable audio output
	log.Println("Audio playback started")
	return nil
}

// StopCall terminates the current call
func (r *RetellWebClient) StopCall() {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if !r.connected {
		return
	}

	log.Println("Stopping call...")

	// Cancel context to stop goroutines
	r.cancel()

	// Cleanup WebRTC connection
	if r.peerConnection != nil {
		r.peerConnection.Close()
		r.peerConnection = nil
	}

	// Cleanup WebSocket connection
	if r.wsConn != nil {
		r.wsConn.Close()
		r.wsConn = nil
	}

	// Cleanup audio components
	if r.analyzerComponent != nil && r.analyzerComponent.Cleanup != nil {
		r.analyzerComponent.Cleanup()
		r.analyzerComponent = nil
	}

	// Reset state
	r.connected = false
	r.isAgentTalking = false
	r.connectionState = ConnectionStateDisconnected
	r.agentAudioTrack = nil
	r.agentMediaStream = nil
	r.customMediaStream = nil

	r.Emit(EventCallEnded, nil)
	log.Println("Call stopped")
}

// Mute turns off the microphone
func (r *RetellWebClient) Mute() error {
	if !r.connected {
		return errors.New("no active call")
	}
	return r.enableMicrophone(false)
}

// Unmute turns on the microphone
func (r *RetellWebClient) Unmute() error {
	if !r.connected {
		return errors.New("no active call")
	}
	return r.enableMicrophone(true)
}

// SendCustomMediaStream sends a custom media stream instead of microphone
func (r *RetellWebClient) SendCustomMediaStream(mediaStream *MediaStreamTrack) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if !r.connected || r.peerConnection == nil {
		return errors.New("no active call")
	}

	log.Println("Starting custom MediaStream transmission...")

	// Disable microphone first
	if err := r.enableMicrophone(false); err != nil {
		return fmt.Errorf("error disabling microphone: %v", err)
	}

	// Store custom media stream
	r.customMediaStream = mediaStream

	// In a real implementation, you would add the custom track to peer connection
	// For now, we'll simulate this
	log.Printf("Custom MediaStream sent successfully. Track ID: %s", mediaStream.ID)

	r.Emit(EventCustomMediaStreamSent, mediaStream)
	return nil
}

// ResumeMicrophone resumes microphone and stops custom media stream
func (r *RetellWebClient) ResumeMicrophone() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if !r.connected || r.peerConnection == nil {
		return errors.New("no active call")
	}

	log.Println("Resuming microphone...")

	// Remove custom media stream
	r.customMediaStream = nil

	// Re-enable microphone
	if err := r.enableMicrophone(true); err != nil {
		return fmt.Errorf("error enabling microphone: %v", err)
	}

	log.Println("Microphone resumed successfully")
	r.Emit(EventMicrophoneResumed, nil)
	return nil
}

// GetTrackStatus returns current track status information
func (r *RetellWebClient) GetTrackStatus() TrackStatus {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	status := TrackStatus{
		MicrophoneEnabled: false, // This would be determined by actual microphone state
		PublishedTracks:   []TrackInfo{},
		TotalTracks:       0,
	}

	// In a real implementation, you would enumerate actual tracks
	if r.customMediaStream != nil {
		status.PublishedTracks = append(status.PublishedTracks, TrackInfo{
			SID:    r.customMediaStream.ID,
			Kind:   r.customMediaStream.Kind,
			Name:   "custom_audio",
			Source: "custom",
		})
		status.TotalTracks++
	}

	return status
}

// GetAgentMediaStream returns the agent's media stream
func (r *RetellWebClient) GetAgentMediaStream() *MediaStreamTrack {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.agentMediaStream
}

// IsConnected returns whether the client is connected
func (r *RetellWebClient) IsConnected() bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.connected
}

// IsAgentTalking returns whether the agent is currently talking
func (r *RetellWebClient) IsAgentTalking() bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.isAgentTalking
}

// createPeerConnection creates a new WebRTC peer connection
func (r *RetellWebClient) createPeerConnection() error {
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	var err error
	r.peerConnection, err = webrtc.NewPeerConnection(config)
	if err != nil {
		return fmt.Errorf("failed to create peer connection: %v", err)
	}

	return nil
}

// setupWebRTCHandlers sets up WebRTC event handlers
func (r *RetellWebClient) setupWebRTCHandlers() {
	if r.peerConnection == nil {
		return
	}

	// Handle ICE connection state changes
	r.peerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("ICE Connection State changed: %s", state.String())

		switch state {
		case webrtc.ICEConnectionStateConnected:
			r.connectionState = ConnectionStateConnected
		case webrtc.ICEConnectionStateDisconnected:
			r.connectionState = ConnectionStateDisconnected
			r.StopCall()
		case webrtc.ICEConnectionStateFailed:
			r.connectionState = ConnectionStateFailed
			r.Emit(EventError, "ICE connection failed")
			r.StopCall()
		}
	})

	// Handle incoming tracks (agent audio)
	r.peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("Received track: %s, kind: %s", track.ID(), track.Kind().String())

		if track.Kind() == webrtc.RTPCodecTypeAudio {
			r.handleAgentAudioTrack(track)
		}
	})
}

// handleAgentAudioTrack handles the agent's audio track
func (r *RetellWebClient) handleAgentAudioTrack(track *webrtc.TrackRemote) {
	r.mutex.Lock()
	r.agentAudioTrack = track
	r.agentMediaStream = NewMediaStreamTrack("audio", track.ID())
	r.mutex.Unlock()

	log.Println("Agent audio track received")
	r.Emit(EventCallReady, nil)
	r.Emit(EventAgentMediaStreamReady, r.agentMediaStream)

	// Start reading RTP packets
	go r.readAudioPackets(track)
}

// readAudioPackets reads audio packets from the track
func (r *RetellWebClient) readAudioPackets(track *webrtc.TrackRemote) {
	for {
		select {
		case <-r.ctx.Done():
			return
		default:
			_, _, err := track.ReadRTP()
			if err != nil {
				log.Printf("Error reading RTP packet: %v", err)
				return
			}
			// Process audio packet here if needed
		}
	}
}

// setupAudioAnalyzer sets up audio analysis for raw audio samples
func (r *RetellWebClient) setupAudioAnalyzer() {
	r.analyzerComponent = &AnalyzerComponent{
		Volume: 0.0,
		Cleanup: func() {
			// Cleanup analyzer resources
		},
	}

	// Start audio analysis goroutine
	go r.analyzeAudio()
}

// analyzeAudio continuously analyzes audio and emits samples
func (r *RetellWebClient) analyzeAudio() {
	ticker := time.NewTicker(16 * time.Millisecond) // ~60fps
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			if r.analyzerComponent != nil {
				// Generate mock audio samples for demonstration
				samples := make([]float32, 1024)
				for i := range samples {
					samples[i] = float32(i) * 0.001 // Mock data
				}

				audioSample := AudioSample{
					Data:      samples,
					Timestamp: time.Now(),
				}

				r.Emit(EventAudio, audioSample)
			}
		}
	}
}

// connectWebSocket establishes WebSocket connection for data communication
func (r *RetellWebClient) connectWebSocket(accessToken string) error {
	// In a real implementation, this would connect to the actual LiveKit WebSocket
	// For now, we'll simulate this connection
	log.Printf("Connecting to WebSocket with access token: %s...", accessToken[:10]+"...")

	// Simulate connection delay
	time.Sleep(100 * time.Millisecond)

	log.Println("WebSocket connected (simulated)")
	return nil
}

// setupDataChannel creates a data channel for server communication
func (r *RetellWebClient) setupDataChannel() error {
	if r.peerConnection == nil {
		return errors.New("peer connection not initialized")
	}

	var err error
	r.dataChannel, err = r.peerConnection.CreateDataChannel("retell-data", nil)
	if err != nil {
		return fmt.Errorf("failed to create data channel: %v", err)
	}

	// Handle data channel events
	r.dataChannel.OnOpen(func() {
		log.Println("Data channel opened")
	})

	r.dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		r.handleDataMessage(msg.Data)
	})

	return nil
}

// handleDataMessage processes messages received via data channel
func (r *RetellWebClient) handleDataMessage(data []byte) {
	var event RetellEvent
	if err := json.Unmarshal(data, &event); err != nil {
		log.Printf("Error parsing data message: %v", err)
		return
	}

	switch event.EventType {
	case "update":
		r.Emit(EventUpdate, event)
	case "metadata":
		r.Emit(EventMetadata, event)
	case "agent_start_talking":
		r.mutex.Lock()
		r.isAgentTalking = true
		r.mutex.Unlock()
		r.Emit(EventAgentStartTalking, nil)
		if r.agentMediaStream != nil {
			r.Emit(EventAgentMediaStreamActive, r.agentMediaStream)
		}
	case "agent_stop_talking":
		r.mutex.Lock()
		r.isAgentTalking = false
		r.mutex.Unlock()
		r.Emit(EventAgentStopTalking, nil)
		if r.agentMediaStream != nil {
			r.Emit(EventAgentMediaStreamInactive, r.agentMediaStream)
		}
	case "node_transition":
		r.Emit(EventNodeTransition, event)
	default:
		log.Printf("Unknown event type: %s", event.EventType)
	}
}

// enableMicrophone enables or disables the microphone
func (r *RetellWebClient) enableMicrophone(enabled bool) error {
	// In a real implementation, this would control the actual microphone track
	log.Printf("Microphone enabled: %t", enabled)
	return nil
}
