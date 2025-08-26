//go:build client

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// TestClient for testing the Retell AI server endpoints
type TestClient struct {
	baseURL   string
	sessionID string
}

// NewTestClient creates a new test client
func NewTestClient(baseURL string) *TestClient {
	return &TestClient{
		baseURL:   baseURL,
		sessionID: fmt.Sprintf("test-session-%d", time.Now().Unix()),
	}
}

// APIResponse represents the server response format
type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func (tc *TestClient) makeRequest(method, endpoint string, payload interface{}) (*APIResponse, error) {
	var body io.Reader

	if payload != nil {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("error marshaling JSON: %v", err)
		}
		body = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequest(method, tc.baseURL+endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("error decoding response: %v", err)
	}

	return &apiResp, nil
}

func (tc *TestClient) testHealthCheck() {
	fmt.Println("🔍 Testing Health Check...")

	resp, err := tc.makeRequest("GET", "/health", nil)
	if err != nil {
		fmt.Printf("❌ Health check failed: %v\n", err)
		return
	}

	if resp.Success {
		fmt.Printf("✅ Health check passed: %s\n", resp.Message)
		if data, ok := resp.Data.(map[string]interface{}); ok {
			fmt.Printf("   Active sessions: %.0f\n", data["activeSessions"])
			fmt.Printf("   Version: %s\n", data["version"])
		}
	} else {
		fmt.Printf("❌ Health check failed: %s\n", resp.Error)
	}
}

func (tc *TestClient) testStartCall() {
	fmt.Println("\n📞 Testing Start Call...")

	payload := map[string]interface{}{
		"sessionId":           tc.sessionID,
		"accessToken":         "test-access-token",
		"sampleRate":          16000,
		"emitRawAudioSamples": true,
	}

	resp, err := tc.makeRequest("POST", "/start-call", payload)
	if err != nil {
		fmt.Printf("❌ Start call failed: %v\n", err)
		return
	}

	if resp.Success {
		fmt.Printf("✅ Call started: %s\n", resp.Message)
		fmt.Printf("   Session ID: %s\n", tc.sessionID)
	} else {
		fmt.Printf("❌ Start call failed: %s\n", resp.Error)
	}
}

func (tc *TestClient) testCallStatus() {
	fmt.Println("\n📊 Testing Call Status...")

	resp, err := tc.makeRequest("GET", fmt.Sprintf("/call-status?sessionId=%s", tc.sessionID), nil)
	if err != nil {
		fmt.Printf("❌ Call status failed: %v\n", err)
		return
	}

	if resp.Success {
		fmt.Printf("✅ Call status retrieved: %s\n", resp.Message)
		if data, ok := resp.Data.(map[string]interface{}); ok {
			fmt.Printf("   Is Active: %v\n", data["isActive"])
			fmt.Printf("   Is Connected: %v\n", data["isConnected"])
			fmt.Printf("   Is Agent Talking: %v\n", data["isAgentTalking"])
		}
	} else {
		fmt.Printf("❌ Call status failed: %s\n", resp.Error)
	}
}

func (tc *TestClient) testMute() {
	fmt.Println("\n🔇 Testing Mute...")

	payload := map[string]interface{}{
		"sessionId": tc.sessionID,
	}

	resp, err := tc.makeRequest("POST", "/mute", payload)
	if err != nil {
		fmt.Printf("❌ Mute failed: %v\n", err)
		return
	}

	if resp.Success {
		fmt.Printf("✅ Mute successful: %s\n", resp.Message)
	} else {
		fmt.Printf("❌ Mute failed: %s\n", resp.Error)
	}
}

func (tc *TestClient) testUnmute() {
	fmt.Println("\n🔊 Testing Unmute...")

	payload := map[string]interface{}{
		"sessionId": tc.sessionID,
	}

	resp, err := tc.makeRequest("POST", "/unmute", payload)
	if err != nil {
		fmt.Printf("❌ Unmute failed: %v\n", err)
		return
	}

	if resp.Success {
		fmt.Printf("✅ Unmute successful: %s\n", resp.Message)
	} else {
		fmt.Printf("❌ Unmute failed: %s\n", resp.Error)
	}
}

func (tc *TestClient) testCustomStream() {
	fmt.Println("\n📤 Testing Custom Stream...")

	payload := map[string]interface{}{
		"sessionId": tc.sessionID,
		"streamId":  "test-custom-stream-123",
		"kind":      "audio",
	}

	resp, err := tc.makeRequest("POST", "/send-custom-stream", payload)
	if err != nil {
		fmt.Printf("❌ Custom stream failed: %v\n", err)
		return
	}

	if resp.Success {
		fmt.Printf("✅ Custom stream sent: %s\n", resp.Message)
	} else {
		fmt.Printf("❌ Custom stream failed: %s\n", resp.Error)
	}
}

func (tc *TestClient) testResumeMicrophone() {
	fmt.Println("\n🎤 Testing Resume Microphone...")

	payload := map[string]interface{}{
		"sessionId": tc.sessionID,
	}

	resp, err := tc.makeRequest("POST", "/resume-microphone", payload)
	if err != nil {
		fmt.Printf("❌ Resume microphone failed: %v\n", err)
		return
	}

	if resp.Success {
		fmt.Printf("✅ Microphone resumed: %s\n", resp.Message)
	} else {
		fmt.Printf("❌ Resume microphone failed: %s\n", resp.Error)
	}
}

func (tc *TestClient) testTrackStatus() {
	fmt.Println("\n🎵 Testing Track Status...")

	resp, err := tc.makeRequest("GET", fmt.Sprintf("/track-status?sessionId=%s", tc.sessionID), nil)
	if err != nil {
		fmt.Printf("❌ Track status failed: %v\n", err)
		return
	}

	if resp.Success {
		fmt.Printf("✅ Track status retrieved: %s\n", resp.Message)
		if data, ok := resp.Data.(map[string]interface{}); ok {
			fmt.Printf("   Microphone Enabled: %v\n", data["microphoneEnabled"])
			fmt.Printf("   Total Tracks: %.0f\n", data["totalTracks"])
		}
	} else {
		fmt.Printf("❌ Track status failed: %s\n", resp.Error)
	}
}

func (tc *TestClient) testStopCall() {
	fmt.Println("\n🛑 Testing Stop Call...")

	payload := map[string]interface{}{
		"sessionId": tc.sessionID,
	}

	resp, err := tc.makeRequest("POST", "/stop-call", payload)
	if err != nil {
		fmt.Printf("❌ Stop call failed: %v\n", err)
		return
	}

	if resp.Success {
		fmt.Printf("✅ Call stopped: %s\n", resp.Message)
	} else {
		fmt.Printf("❌ Stop call failed: %s\n", resp.Error)
	}
}

func (tc *TestClient) runAllTests() {
	fmt.Println("🚀 Starting Retell AI Server API Tests")
	fmt.Println("=====================================")

	// Test sequence
	tc.testHealthCheck()
	tc.testStartCall()

	// Wait a bit for call to establish
	time.Sleep(1 * time.Second)

	tc.testCallStatus()
	tc.testMute()
	tc.testUnmute()
	tc.testCustomStream()
	tc.testResumeMicrophone()
	tc.testTrackStatus()
	tc.testStopCall()

	fmt.Println("\n✅ All tests completed!")
}

func main() {
	// Default server URL
	serverURL := "http://localhost:8080"

	// You can override this with command line argument
	if len(os.Args) > 1 {
		serverURL = os.Args[1]
	}

	fmt.Printf("Testing server at: %s\n\n", serverURL)

	client := NewTestClient(serverURL)
	client.runAllTests()
}
