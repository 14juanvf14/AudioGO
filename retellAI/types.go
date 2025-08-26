package retellAI

import (
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
)

// StartCallConfig represents the configuration for starting a call
type StartCallConfig struct {
	AccessToken         string `json:"accessToken"`
	SampleRate          int    `json:"sampleRate,omitempty"`
	CaptureDeviceID     string `json:"captureDeviceId,omitempty"`
	PlaybackDeviceID    string `json:"playbackDeviceId,omitempty"`
	EmitRawAudioSamples bool   `json:"emitRawAudioSamples,omitempty"`
}

// TrackStatus represents the current status of tracks
type TrackStatus struct {
	MicrophoneEnabled bool        `json:"microphoneEnabled"`
	PublishedTracks   []TrackInfo `json:"publishedTracks"`
	TotalTracks       int         `json:"totalTracks"`
}

// TrackInfo represents information about a track
type TrackInfo struct {
	SID    string `json:"sid"`
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Source string `json:"source"`
}

// AnalyzerComponent represents an audio analyzer similar to the JS version
type AnalyzerComponent struct {
	Volume  float64 `json:"volume"`
	Cleanup func()  `json:"-"`
}

// RetellEvent represents events received from the server
type RetellEvent struct {
	EventType string      `json:"event_type"`
	Data      interface{} `json:"data,omitempty"`
}

// Connection states
const (
	ConnectionStateDisconnected = "disconnected"
	ConnectionStateConnecting   = "connecting"
	ConnectionStateConnected    = "connected"
	ConnectionStateFailed       = "failed"
)

// Event types
const (
	EventCallStarted              = "call_started"
	EventCallEnded                = "call_ended"
	EventCallReady                = "call_ready"
	EventError                    = "error"
	EventUpdate                   = "update"
	EventMetadata                 = "metadata"
	EventAgentStartTalking        = "agent_start_talking"
	EventAgentStopTalking         = "agent_stop_talking"
	EventNodeTransition           = "node_transition"
	EventCustomMediaStreamSent    = "custom_media_stream_sent"
	EventMicrophoneResumed        = "microphone_resumed"
	EventAgentMediaStreamReady    = "agent_media_stream_ready"
	EventAgentMediaStreamActive   = "agent_media_stream_active"
	EventAgentMediaStreamInactive = "agent_media_stream_inactive"
	EventAudio                    = "audio"
)

// MediaStreamTrack represents a media stream track wrapper
type MediaStreamTrack struct {
	Track   *webrtc.TrackLocalStaticSample
	Kind    string
	ID      string
	Enabled bool
	mutex   sync.RWMutex
}

// NewMediaStreamTrack creates a new MediaStreamTrack
func NewMediaStreamTrack(kind, id string) *MediaStreamTrack {
	return &MediaStreamTrack{
		Kind:    kind,
		ID:      id,
		Enabled: true,
	}
}

// IsEnabled returns whether the track is enabled
func (m *MediaStreamTrack) IsEnabled() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.Enabled
}

// SetEnabled sets the enabled state of the track
func (m *MediaStreamTrack) SetEnabled(enabled bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.Enabled = enabled
}

// AudioSample represents an audio sample
type AudioSample struct {
	Data      []float32 `json:"data"`
	Timestamp time.Time `json:"timestamp"`
}
