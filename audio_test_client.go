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

// AudioTestClient for testing the audio endpoints
type AudioTestClient struct {
	baseURL string
}

// NewAudioTestClient creates a new audio test client
func NewAudioTestClient(baseURL string) *AudioTestClient {
	return &AudioTestClient{
		baseURL: baseURL,
	}
}

// APIResponse represents the server response format
type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func (atc *AudioTestClient) makeRequest(method, endpoint string, payload interface{}) (*APIResponse, error) {
	var body io.Reader

	if payload != nil {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("error marshaling JSON: %v", err)
		}
		body = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequest(method, atc.baseURL+endpoint, body)
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

func (atc *AudioTestClient) testGetAudioDevices() {
	fmt.Println("🎧 Testing Get Audio Devices...")

	resp, err := atc.makeRequest("GET", "/audio/devices", nil)
	if err != nil {
		fmt.Printf("❌ Get audio devices failed: %v\n", err)
		return
	}

	if resp.Success {
		fmt.Printf("✅ Audio devices retrieved: %s\n", resp.Message)

		// Parse and display devices
		if devices, ok := resp.Data.([]interface{}); ok {
			fmt.Printf("   Found %d audio devices:\n", len(devices))
			for i, device := range devices {
				if deviceMap, ok := device.(map[string]interface{}); ok {
					name := deviceMap["name"]
					isInput := deviceMap["isInput"]
					isOutput := deviceMap["isOutput"]
					isDefault := deviceMap["isDefault"]

					deviceType := ""
					if isInput == true && isOutput == true {
						deviceType = "Input/Output"
					} else if isInput == true {
						deviceType = "Input"
					} else if isOutput == true {
						deviceType = "Output"
					}

					defaultMarker := ""
					if isDefault == true {
						defaultMarker = " (Default)"
					}

					fmt.Printf("     %d. %s - %s%s\n", i, name, deviceType, defaultMarker)
				}
			}
		}
	} else {
		fmt.Printf("❌ Get audio devices failed: %s\n", resp.Error)
	}
}

func (atc *AudioTestClient) testSetInputDevice(deviceIndex int) {
	fmt.Printf("🎤 Testing Set Input Device to index %d...\n", deviceIndex)

	payload := map[string]interface{}{
		"deviceIndex": deviceIndex,
	}

	resp, err := atc.makeRequest("POST", "/audio/set-input-device", payload)
	if err != nil {
		fmt.Printf("❌ Set input device failed: %v\n", err)
		return
	}

	if resp.Success {
		fmt.Printf("✅ Input device set: %s\n", resp.Message)
	} else {
		fmt.Printf("❌ Set input device failed: %s\n", resp.Error)
	}
}

func (atc *AudioTestClient) testSetOutputDevice(deviceIndex int) {
	fmt.Printf("🔊 Testing Set Output Device to index %d...\n", deviceIndex)

	payload := map[string]interface{}{
		"deviceIndex": deviceIndex,
	}

	resp, err := atc.makeRequest("POST", "/audio/set-output-device", payload)
	if err != nil {
		fmt.Printf("❌ Set output device failed: %v\n", err)
		return
	}

	if resp.Success {
		fmt.Printf("✅ Output device set: %s\n", resp.Message)
	} else {
		fmt.Printf("❌ Set output device failed: %s\n", resp.Error)
	}
}

func (atc *AudioTestClient) testStartAudioCapture() {
	fmt.Println("🎤 Testing Start Audio Capture...")

	resp, err := atc.makeRequest("POST", "/audio/start-capture", nil)
	if err != nil {
		fmt.Printf("❌ Start audio capture failed: %v\n", err)
		return
	}

	if resp.Success {
		fmt.Printf("✅ Audio capture started: %s\n", resp.Message)
	} else {
		fmt.Printf("❌ Start audio capture failed: %s\n", resp.Error)
	}
}

func (atc *AudioTestClient) testStopAudioCapture() {
	fmt.Println("🛑 Testing Stop Audio Capture...")

	resp, err := atc.makeRequest("POST", "/audio/stop-capture", nil)
	if err != nil {
		fmt.Printf("❌ Stop audio capture failed: %v\n", err)
		return
	}

	if resp.Success {
		fmt.Printf("✅ Audio capture stopped: %s\n", resp.Message)
	} else {
		fmt.Printf("❌ Stop audio capture failed: %s\n", resp.Error)
	}
}

func (atc *AudioTestClient) testStartAudioPlayback() {
	fmt.Println("🔊 Testing Start Audio Playback...")

	resp, err := atc.makeRequest("POST", "/audio/start-playback", nil)
	if err != nil {
		fmt.Printf("❌ Start audio playback failed: %v\n", err)
		return
	}

	if resp.Success {
		fmt.Printf("✅ Audio playback started: %s\n", resp.Message)
	} else {
		fmt.Printf("❌ Start audio playback failed: %s\n", resp.Error)
	}
}

func (atc *AudioTestClient) testStopAudioPlayback() {
	fmt.Println("🛑 Testing Stop Audio Playback...")

	resp, err := atc.makeRequest("POST", "/audio/stop-playback", nil)
	if err != nil {
		fmt.Printf("❌ Stop audio playback failed: %v\n", err)
		return
	}

	if resp.Success {
		fmt.Printf("✅ Audio playback stopped: %s\n", resp.Message)
	} else {
		fmt.Printf("❌ Stop audio playback failed: %s\n", resp.Error)
	}
}

func (atc *AudioTestClient) testGetAudioStatus() {
	fmt.Println("📊 Testing Get Audio Status...")

	resp, err := atc.makeRequest("GET", "/audio/status", nil)
	if err != nil {
		fmt.Printf("❌ Get audio status failed: %v\n", err)
		return
	}

	if resp.Success {
		fmt.Printf("✅ Audio status retrieved: %s\n", resp.Message)

		// Parse and display status
		if status, ok := resp.Data.(map[string]interface{}); ok {
			fmt.Printf("   Is Capturing: %v\n", status["isCapturing"])
			fmt.Printf("   Is Playing: %v\n", status["isPlaying"])
			fmt.Printf("   Sample Rate: %.0f Hz\n", status["sampleRate"])
			fmt.Printf("   Channels: %.0f\n", status["channels"])
			fmt.Printf("   Frame Size: %.0f\n", status["frameSize"])

			if inputDevice, ok := status["inputDevice"].(map[string]interface{}); ok {
				fmt.Printf("   Input Device: %s\n", inputDevice["name"])
			}

			if outputDevice, ok := status["outputDevice"].(map[string]interface{}); ok {
				fmt.Printf("   Output Device: %s\n", outputDevice["name"])
			}
		}
	} else {
		fmt.Printf("❌ Get audio status failed: %s\n", resp.Error)
	}
}

func (atc *AudioTestClient) runAudioTests() {
	fmt.Println("🎵 Starting Audio System Tests")
	fmt.Println("=============================")

	// Test sequence
	atc.testGetAudioDevices()

	fmt.Println()
	// Try to set reasonable default devices (usually 0 is default)
	atc.testSetInputDevice(0)
	atc.testSetOutputDevice(0)

	fmt.Println()
	atc.testGetAudioStatus()

	fmt.Println()
	atc.testStartAudioCapture()

	// Wait a bit to capture some audio
	fmt.Println("⏳ Capturing audio for 3 seconds...")
	time.Sleep(3 * time.Second)

	atc.testStopAudioCapture()

	fmt.Println()
	atc.testStartAudioPlayback()

	// Wait a bit
	fmt.Println("⏳ Playback ready for 2 seconds...")
	time.Sleep(2 * time.Second)

	atc.testStopAudioPlayback()

	fmt.Println()
	atc.testGetAudioStatus()

	fmt.Println("\n✅ Audio tests completed!")
}

func main() {
	// Default server URL
	serverURL := "http://localhost:8080"

	// You can override this with command line argument
	if len(os.Args) > 1 {
		serverURL = os.Args[1]
	}

	fmt.Printf("Testing audio endpoints at: %s\n\n", serverURL)

	client := NewAudioTestClient(serverURL)
	client.runAudioTests()
}
