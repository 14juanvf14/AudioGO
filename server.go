package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"webrtc-audio-server/retellAI"

	"github.com/gorilla/websocket"
)

// Server manages HTTP endpoints and WebSocket connections for Retell AI calls
type Server struct {
	clients      map[string]*ClientSession
	audioManager *SystemAudioManager
	mutex        sync.RWMutex
	port         string
}

// ClientSession represents an active client session with Retell AI
type ClientSession struct {
	ID           string
	RetellClient *retellAI.RetellWebClient
	WSConn       *websocket.Conn
	IsActive     bool
	mutex        sync.RWMutex
}

// APIResponse represents standard API response format
type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// StartCallRequest represents the request to start a call
type StartCallRequest struct {
	SessionID           string `json:"sessionId"`
	AccessToken         string `json:"accessToken"`
	SampleRate          int    `json:"sampleRate,omitempty"`
	CaptureDeviceID     string `json:"captureDeviceId,omitempty"`
	PlaybackDeviceID    string `json:"playbackDeviceId,omitempty"`
	EmitRawAudioSamples bool   `json:"emitRawAudioSamples,omitempty"`
}

// BasicRequest represents a basic request with session ID
type BasicRequest struct {
	SessionID string `json:"sessionId"`
}

// CustomStreamRequest represents request to send custom media stream
type CustomStreamRequest struct {
	SessionID string `json:"sessionId"`
	StreamID  string `json:"streamId"`
	Kind      string `json:"kind"`
}

// AudioDeviceRequest represents request to set audio device
type AudioDeviceRequest struct {
	DeviceIndex int `json:"deviceIndex"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for development
	},
}

// NewServer creates a new server instance
func NewServer(port string) *Server {
	audioManager := NewSystemAudioManager()
	if err := audioManager.Initialize(); err != nil {
		log.Printf("⚠️ Failed to initialize audio manager: %v", err)
		log.Println("🔄 Server will continue without real audio capabilities")
		audioManager = nil // Set to nil to indicate audio is not available
	}
	
	return &Server{
		clients:      make(map[string]*ClientSession),
		audioManager: audioManager,
		port:         port,
	}
}

// Start starts the HTTP server with all endpoints
func (s *Server) Start() error {
	// Setup HTTP routes
	http.HandleFunc("/health", s.healthHandler)
	http.HandleFunc("/start-call", s.startCallHandler)
	http.HandleFunc("/stop-call", s.stopCallHandler)
	http.HandleFunc("/mute", s.muteHandler)
	http.HandleFunc("/unmute", s.unmuteHandler)
	http.HandleFunc("/send-custom-stream", s.sendCustomStreamHandler)
	http.HandleFunc("/resume-microphone", s.resumeMicrophoneHandler)
	http.HandleFunc("/call-status", s.callStatusHandler)
	http.HandleFunc("/track-status", s.trackStatusHandler)
	http.HandleFunc("/ws", s.websocketHandler)

	// Audio device management endpoints
	http.HandleFunc("/audio/devices", s.audioDevicesHandler)
	http.HandleFunc("/audio/set-input-device", s.setInputDeviceHandler)
	http.HandleFunc("/audio/set-output-device", s.setOutputDeviceHandler)
	http.HandleFunc("/audio/start-capture", s.startAudioCaptureHandler)
	http.HandleFunc("/audio/stop-capture", s.stopAudioCaptureHandler)
	http.HandleFunc("/audio/start-playback", s.startAudioPlaybackHandler)
	http.HandleFunc("/audio/stop-playback", s.stopAudioPlaybackHandler)
	http.HandleFunc("/audio/status", s.audioStatusHandler)

	// Start server
	log.Printf("🚀 Retell AI Server starting on port %s", s.port)
	log.Printf("📋 Available endpoints:")
	log.Printf("   GET  /health - Health check")
	log.Printf("   POST /start-call - Start a new call")
	log.Printf("   POST /stop-call - Stop an active call")
	log.Printf("   POST /mute - Mute microphone")
	log.Printf("   POST /unmute - Unmute microphone")
	log.Printf("   POST /send-custom-stream - Send custom media stream")
	log.Printf("   POST /resume-microphone - Resume microphone")
	log.Printf("   GET  /call-status?sessionId=X - Get call status")
	log.Printf("   GET  /track-status?sessionId=X - Get track status")
	log.Printf("   WS   /ws?sessionId=X - WebSocket for real-time events")
	log.Printf("   🎤 Audio Device Management:")
	log.Printf("   GET  /audio/devices - List audio devices")
	log.Printf("   POST /audio/set-input-device - Set input device")
	log.Printf("   POST /audio/set-output-device - Set output device")
	log.Printf("   POST /audio/start-capture - Start audio capture")
	log.Printf("   POST /audio/stop-capture - Stop audio capture")
	log.Printf("   POST /audio/start-playback - Start audio playback")
	log.Printf("   POST /audio/stop-playback - Stop audio playback")
	log.Printf("   GET  /audio/status - Get audio system status")

	return http.ListenAndServe(":"+s.port, nil)
}

// Cleanup cleans up server resources
func (s *Server) Cleanup() {
	log.Println("🧹 Cleaning up server resources...")

	// Stop all active sessions
	s.mutex.Lock()
	for sessionID := range s.clients {
		if session := s.clients[sessionID]; session != nil {
			session.RetellClient.StopCall()
		}
	}
	s.clients = make(map[string]*ClientSession)
	s.mutex.Unlock()

	// Terminate audio manager
	if s.audioManager != nil {
		s.audioManager.Terminate()
	}

	log.Println("✅ Server cleanup completed")
}

// healthHandler provides a health check endpoint
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendErrorResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := APIResponse{
		Success: true,
		Message: "Retell AI Server is running",
		Data: map[string]interface{}{
			"activeSessions": len(s.clients),
			"version":        "1.0.0",
		},
	}

	s.sendJSONResponse(w, response, http.StatusOK)
}

// startCallHandler handles POST /start-call
func (s *Server) startCallHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendErrorResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req StartCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendErrorResponse(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.SessionID == "" {
		s.sendErrorResponse(w, "sessionId is required", http.StatusBadRequest)
		return
	}
	if req.AccessToken == "" {
		s.sendErrorResponse(w, "accessToken is required", http.StatusBadRequest)
		return
	}

	// Check if session already exists
	s.mutex.RLock()
	if existingSession, exists := s.clients[req.SessionID]; exists && existingSession.IsActive {
		s.mutex.RUnlock()
		s.sendErrorResponse(w, "Session already active", http.StatusConflict)
		return
	}
	s.mutex.RUnlock()

	// Create new Retell client
	retellClient := retellAI.NewRetellWebClient()

	// Create client session
	session := &ClientSession{
		ID:           req.SessionID,
		RetellClient: retellClient,
		IsActive:     false,
	}

	// Setup event forwarding (will be used when WebSocket is connected)
	s.setupEventForwarding(session)

	// Store session
	s.mutex.Lock()
	s.clients[req.SessionID] = session
	s.mutex.Unlock()

	// Create call config
	config := retellAI.StartCallConfig{
		AccessToken:         req.AccessToken,
		SampleRate:          req.SampleRate,
		CaptureDeviceID:     req.CaptureDeviceID,
		PlaybackDeviceID:    req.PlaybackDeviceID,
		EmitRawAudioSamples: req.EmitRawAudioSamples,
	}

	// Set default sample rate if not provided
	if config.SampleRate == 0 {
		config.SampleRate = 16000
	}

	// Start the call
	if err := retellClient.StartCall(config); err != nil {
		// Remove session on error
		s.mutex.Lock()
		delete(s.clients, req.SessionID)
		s.mutex.Unlock()

		s.sendErrorResponse(w, "Failed to start call: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Setup audio integration
	s.setupAudioIntegration(session)

	// Mark session as active
	session.mutex.Lock()
	session.IsActive = true
	session.mutex.Unlock()

	response := APIResponse{
		Success: true,
		Message: "Call started successfully",
		Data: map[string]interface{}{
			"sessionId": req.SessionID,
			"config":    config,
		},
	}

	log.Printf("✅ Call started for session: %s", req.SessionID)
	s.sendJSONResponse(w, response, http.StatusOK)
}

// stopCallHandler handles POST /stop-call
func (s *Server) stopCallHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendErrorResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req BasicRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendErrorResponse(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	session := s.getSession(req.SessionID)
	if session == nil {
		s.sendErrorResponse(w, "Session not found", http.StatusNotFound)
		return
	}

	// Stop the call
	session.RetellClient.StopCall()

	// Mark session as inactive and close WebSocket if exists
	session.mutex.Lock()
	session.IsActive = false
	if session.WSConn != nil {
		session.WSConn.Close()
		session.WSConn = nil
	}
	session.mutex.Unlock()

	// Remove session
	s.mutex.Lock()
	delete(s.clients, req.SessionID)
	s.mutex.Unlock()

	response := APIResponse{
		Success: true,
		Message: "Call stopped successfully",
		Data: map[string]string{
			"sessionId": req.SessionID,
		},
	}

	log.Printf("🛑 Call stopped for session: %s", req.SessionID)
	s.sendJSONResponse(w, response, http.StatusOK)
}

// muteHandler handles POST /mute
func (s *Server) muteHandler(w http.ResponseWriter, r *http.Request) {
	s.handleAudioControl(w, r, "mute", func(client *retellAI.RetellWebClient) error {
		return client.Mute()
	})
}

// unmuteHandler handles POST /unmute
func (s *Server) unmuteHandler(w http.ResponseWriter, r *http.Request) {
	s.handleAudioControl(w, r, "unmute", func(client *retellAI.RetellWebClient) error {
		return client.Unmute()
	})
}

// sendCustomStreamHandler handles POST /send-custom-stream
func (s *Server) sendCustomStreamHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendErrorResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CustomStreamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendErrorResponse(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	session := s.getActiveSession(req.SessionID)
	if session == nil {
		s.sendErrorResponse(w, "Active session not found", http.StatusNotFound)
		return
	}

	// Create custom media stream
	kind := req.Kind
	if kind == "" {
		kind = "audio"
	}

	customStream := retellAI.NewMediaStreamTrack(kind, req.StreamID)

	// Send custom stream
	if err := session.RetellClient.SendCustomMediaStream(customStream); err != nil {
		s.sendErrorResponse(w, "Failed to send custom stream: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := APIResponse{
		Success: true,
		Message: "Custom media stream sent successfully",
		Data: map[string]string{
			"sessionId": req.SessionID,
			"streamId":  req.StreamID,
			"kind":      kind,
		},
	}

	s.sendJSONResponse(w, response, http.StatusOK)
}

// resumeMicrophoneHandler handles POST /resume-microphone
func (s *Server) resumeMicrophoneHandler(w http.ResponseWriter, r *http.Request) {
	s.handleAudioControl(w, r, "resume microphone", func(client *retellAI.RetellWebClient) error {
		return client.ResumeMicrophone()
	})
}

// callStatusHandler handles GET /call-status
func (s *Server) callStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendErrorResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		s.sendErrorResponse(w, "sessionId parameter is required", http.StatusBadRequest)
		return
	}

	session := s.getSession(sessionID)
	if session == nil {
		s.sendErrorResponse(w, "Session not found", http.StatusNotFound)
		return
	}

	status := map[string]interface{}{
		"sessionId":      sessionID,
		"isActive":       session.IsActive,
		"isConnected":    session.RetellClient.IsConnected(),
		"isAgentTalking": session.RetellClient.IsAgentTalking(),
		"hasWebSocket":   session.WSConn != nil,
	}

	response := APIResponse{
		Success: true,
		Message: "Call status retrieved",
		Data:    status,
	}

	s.sendJSONResponse(w, response, http.StatusOK)
}

// trackStatusHandler handles GET /track-status
func (s *Server) trackStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendErrorResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		s.sendErrorResponse(w, "sessionId parameter is required", http.StatusBadRequest)
		return
	}

	session := s.getActiveSession(sessionID)
	if session == nil {
		s.sendErrorResponse(w, "Active session not found", http.StatusNotFound)
		return
	}

	trackStatus := session.RetellClient.GetTrackStatus()

	response := APIResponse{
		Success: true,
		Message: "Track status retrieved",
		Data:    trackStatus,
	}

	s.sendJSONResponse(w, response, http.StatusOK)
}

// websocketHandler handles WebSocket connections for real-time events
func (s *Server) websocketHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		http.Error(w, "sessionId parameter is required", http.StatusBadRequest)
		return
	}

	session := s.getSession(sessionID)
	if session == nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("❌ WebSocket upgrade failed: %v", err)
		return
	}

	// Store WebSocket connection
	session.mutex.Lock()
	session.WSConn = conn
	session.mutex.Unlock()

	log.Printf("🔌 WebSocket connected for session: %s", sessionID)

	// Send initial status
	initialStatus := map[string]interface{}{
		"type":      "connection_established",
		"sessionId": sessionID,
		"status": map[string]interface{}{
			"isConnected":    session.RetellClient.IsConnected(),
			"isAgentTalking": session.RetellClient.IsAgentTalking(),
		},
	}

	if err := conn.WriteJSON(initialStatus); err != nil {
		log.Printf("❌ Failed to send initial status: %v", err)
	}

	// Handle WebSocket messages (ping/pong, etc.)
	go s.handleWebSocketMessages(session, conn)
}

// Helper methods

// checkAudioAvailable checks if audio system is available
func (s *Server) checkAudioAvailable(w http.ResponseWriter) bool {
	if s.audioManager == nil {
		s.sendErrorResponse(w, "Audio system not available - PortAudio not initialized", http.StatusServiceUnavailable)
		return false
	}
	return true
}

func (s *Server) handleAudioControl(w http.ResponseWriter, r *http.Request, action string, controlFunc func(*retellAI.RetellWebClient) error) {
	if r.Method != http.MethodPost {
		s.sendErrorResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req BasicRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendErrorResponse(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	session := s.getActiveSession(req.SessionID)
	if session == nil {
		s.sendErrorResponse(w, "Active session not found", http.StatusNotFound)
		return
	}

	if err := controlFunc(session.RetellClient); err != nil {
		s.sendErrorResponse(w, fmt.Sprintf("Failed to %s: %v", action, err), http.StatusInternalServerError)
		return
	}

	response := APIResponse{
		Success: true,
		Message: fmt.Sprintf("%s successful", action),
		Data: map[string]string{
			"sessionId": req.SessionID,
			"action":    action,
		},
	}

	s.sendJSONResponse(w, response, http.StatusOK)
}

func (s *Server) getSession(sessionID string) *ClientSession {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.clients[sessionID]
}

func (s *Server) getActiveSession(sessionID string) *ClientSession {
	session := s.getSession(sessionID)
	if session == nil || !session.IsActive {
		return nil
	}
	return session
}

func (s *Server) sendJSONResponse(w http.ResponseWriter, response APIResponse, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

func (s *Server) sendErrorResponse(w http.ResponseWriter, message string, statusCode int) {
	response := APIResponse{
		Success: false,
		Error:   message,
	}
	s.sendJSONResponse(w, response, statusCode)
}

func (s *Server) setupEventForwarding(session *ClientSession) {
	// Forward all Retell AI events to WebSocket if connected
	events := []string{
		retellAI.EventCallStarted,
		retellAI.EventCallReady,
		retellAI.EventCallEnded,
		retellAI.EventAgentStartTalking,
		retellAI.EventAgentStopTalking,
		retellAI.EventAgentMediaStreamReady,
		retellAI.EventAgentMediaStreamActive,
		retellAI.EventAgentMediaStreamInactive,
		retellAI.EventCustomMediaStreamSent,
		retellAI.EventMicrophoneResumed,
		retellAI.EventUpdate,
		retellAI.EventMetadata,
		retellAI.EventNodeTransition,
		retellAI.EventError,
	}

	for _, eventType := range events {
		session.RetellClient.On(eventType, func(data interface{}) {
			s.forwardEventToWebSocket(session, eventType, data)
		})
	}

	// Special handling for audio events (can be high frequency)
	session.RetellClient.On(retellAI.EventAudio, func(data interface{}) {
		// Only forward audio events if specifically requested
		// You might want to throttle these or make them optional
		s.forwardEventToWebSocket(session, retellAI.EventAudio, data)
	})
}

func (s *Server) forwardEventToWebSocket(session *ClientSession, eventType string, data interface{}) {
	session.mutex.RLock()
	conn := session.WSConn
	session.mutex.RUnlock()

	if conn == nil {
		return // No WebSocket connection
	}

	message := map[string]interface{}{
		"type":      "retell_event",
		"eventType": eventType,
		"data":      data,
		"timestamp": fmt.Sprintf("%d", 0), // You can add proper timestamp here
	}

	if err := conn.WriteJSON(message); err != nil {
		log.Printf("❌ Failed to send WebSocket message: %v", err)
		// Close connection on error
		session.mutex.Lock()
		session.WSConn = nil
		session.mutex.Unlock()
		conn.Close()
	}
}

func (s *Server) handleWebSocketMessages(session *ClientSession, conn *websocket.Conn) {
	defer func() {
		session.mutex.Lock()
		session.WSConn = nil
		session.mutex.Unlock()
		conn.Close()
		log.Printf("🔌 WebSocket disconnected for session: %s", session.ID)
	}()

	for {
		// Read message from WebSocket
		var msg map[string]interface{}
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("❌ WebSocket error: %v", err)
			}
			break
		}

		// Handle ping/pong or other client messages
		if msgType, ok := msg["type"].(string); ok && msgType == "ping" {
			pongResponse := map[string]interface{}{
				"type":      "pong",
				"timestamp": fmt.Sprintf("%d", 0),
			}
			if err := conn.WriteJSON(pongResponse); err != nil {
				log.Printf("❌ Failed to send pong: %v", err)
				break
			}
		}
	}
}

// Audio device management handlers

// audioDevicesHandler handles GET /audio/devices
func (s *Server) audioDevicesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendErrorResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.checkAudioAvailable(w) {
		return
	}

	devices, err := s.audioManager.GetAudioDevices()
	if err != nil {
		s.sendErrorResponse(w, "Failed to get audio devices: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := APIResponse{
		Success: true,
		Message: "Audio devices retrieved successfully",
		Data:    devices,
	}

	s.sendJSONResponse(w, response, http.StatusOK)
}

// setInputDeviceHandler handles POST /audio/set-input-device
func (s *Server) setInputDeviceHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendErrorResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.checkAudioAvailable(w) {
		return
	}

	var req AudioDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendErrorResponse(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.audioManager.SetInputDevice(req.DeviceIndex); err != nil {
		s.sendErrorResponse(w, "Failed to set input device: "+err.Error(), http.StatusBadRequest)
		return
	}

	response := APIResponse{
		Success: true,
		Message: "Input device set successfully",
		Data: map[string]int{
			"deviceIndex": req.DeviceIndex,
		},
	}

	s.sendJSONResponse(w, response, http.StatusOK)
}

// setOutputDeviceHandler handles POST /audio/set-output-device
func (s *Server) setOutputDeviceHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendErrorResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AudioDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendErrorResponse(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.audioManager.SetOutputDevice(req.DeviceIndex); err != nil {
		s.sendErrorResponse(w, "Failed to set output device: "+err.Error(), http.StatusBadRequest)
		return
	}

	response := APIResponse{
		Success: true,
		Message: "Output device set successfully",
		Data: map[string]int{
			"deviceIndex": req.DeviceIndex,
		},
	}

	s.sendJSONResponse(w, response, http.StatusOK)
}

// startAudioCaptureHandler handles POST /audio/start-capture
func (s *Server) startAudioCaptureHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendErrorResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := s.audioManager.StartCapture(); err != nil {
		s.sendErrorResponse(w, "Failed to start audio capture: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := APIResponse{
		Success: true,
		Message: "Audio capture started successfully",
	}

	s.sendJSONResponse(w, response, http.StatusOK)
}

// stopAudioCaptureHandler handles POST /audio/stop-capture
func (s *Server) stopAudioCaptureHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendErrorResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.audioManager.StopCapture()

	response := APIResponse{
		Success: true,
		Message: "Audio capture stopped successfully",
	}

	s.sendJSONResponse(w, response, http.StatusOK)
}

// startAudioPlaybackHandler handles POST /audio/start-playback
func (s *Server) startAudioPlaybackHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendErrorResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := s.audioManager.StartPlayback(); err != nil {
		s.sendErrorResponse(w, "Failed to start audio playback: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := APIResponse{
		Success: true,
		Message: "Audio playback started successfully",
	}

	s.sendJSONResponse(w, response, http.StatusOK)
}

// stopAudioPlaybackHandler handles POST /audio/stop-playback
func (s *Server) stopAudioPlaybackHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendErrorResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.audioManager.StopPlayback()

	response := APIResponse{
		Success: true,
		Message: "Audio playback stopped successfully",
	}

	s.sendJSONResponse(w, response, http.StatusOK)
}

// audioStatusHandler handles GET /audio/status
func (s *Server) audioStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.sendErrorResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := s.audioManager.GetStatus()

	response := APIResponse{
		Success: true,
		Message: "Audio status retrieved successfully",
		Data:    status,
	}

	s.sendJSONResponse(w, response, http.StatusOK)
}

// setupAudioIntegration sets up audio integration between system audio and Retell client
func (s *Server) setupAudioIntegration(session *ClientSession) {
	// Setup callback to forward microphone audio to Retell
	s.audioManager.SetInputCallback(func(audioData []float32) {
		// Forward audio data to Retell client
		// This would be implemented in the Retell client to accept real audio
		// For now, we'll emit it as an event
		if session.WSConn != nil {
			audioEvent := map[string]interface{}{
				"type":      "microphone_audio",
				"data":      audioData[:10], // Send only first 10 samples for demo
				"timestamp": time.Now().Unix(),
				"sessionId": session.ID,
			}
			session.WSConn.WriteJSON(audioEvent)
		}
	})

	// Setup callback to receive agent audio and send to speakers
	session.RetellClient.On(retellAI.EventAgentMediaStreamActive, func(data interface{}) {
		// When agent starts talking, we would receive audio data here
		// and send it to the speakers via s.audioManager.QueueOutputAudio()
		log.Printf("🗣️ Agent audio stream active for session %s", session.ID)
	})

	log.Printf("🔗 Audio integration setup completed for session: %s", session.ID)
}
