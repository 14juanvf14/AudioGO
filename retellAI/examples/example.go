package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"

	"retellAI"
)

func main() {
	// Create a new Retell client
	client := retellAI.NewRetellWebClient()

	// Setup event listeners (equivalent to JavaScript event listeners)
	setupEventListeners(client)

	// Configure call settings
	config := retellAI.StartCallConfig{
		AccessToken:         "your-access-token-here",
		SampleRate:          16000,
		CaptureDeviceID:     "",   // Default device
		PlaybackDeviceID:    "",   // Default device
		EmitRawAudioSamples: true, // Enable raw audio sample emission
	}

	// Start the call
	fmt.Println("Starting Retell AI call...")
	if err := client.StartCall(config); err != nil {
		log.Fatalf("Failed to start call: %v", err)
	}

	// Simulate call interactions
	go simulateCallInteractions(client)

	// Wait for interrupt signal to gracefully shutdown
	waitForShutdown(client)
}

// setupEventListeners configures all event listeners for the client
func setupEventListeners(client *retellAI.RetellWebClient) {
	// Call lifecycle events
	client.On(retellAI.EventCallStarted, func(data interface{}) {
		fmt.Println("📞 Call started!")
	})

	client.On(retellAI.EventCallReady, func(data interface{}) {
		fmt.Println("✅ Call ready - agent audio track available")
	})

	client.On(retellAI.EventCallEnded, func(data interface{}) {
		fmt.Println("📴 Call ended")
	})

	// Agent speaking events
	client.On(retellAI.EventAgentStartTalking, func(data interface{}) {
		fmt.Println("🗣️  Agent started talking")
	})

	client.On(retellAI.EventAgentStopTalking, func(data interface{}) {
		fmt.Println("🤐 Agent stopped talking")
	})

	// Media stream events
	client.On(retellAI.EventAgentMediaStreamReady, func(data interface{}) {
		if mediaStream, ok := data.(*retellAI.MediaStreamTrack); ok {
			fmt.Printf("🎵 Agent media stream ready - Track ID: %s\n", mediaStream.ID)
		}
	})

	client.On(retellAI.EventAgentMediaStreamActive, func(data interface{}) {
		fmt.Println("🔊 Agent media stream active")
	})

	client.On(retellAI.EventAgentMediaStreamInactive, func(data interface{}) {
		fmt.Println("🔇 Agent media stream inactive")
	})

	// Custom media stream events
	client.On(retellAI.EventCustomMediaStreamSent, func(data interface{}) {
		if mediaStream, ok := data.(*retellAI.MediaStreamTrack); ok {
			fmt.Printf("📤 Custom media stream sent - Track ID: %s\n", mediaStream.ID)
		}
	})

	client.On(retellAI.EventMicrophoneResumed, func(data interface{}) {
		fmt.Println("🎤 Microphone resumed")
	})

	// Audio samples (for visualization/animation)
	client.On(retellAI.EventAudio, func(data interface{}) {
		if audioSample, ok := data.(retellAI.AudioSample); ok {
			// Calculate volume for visualization
			volume := calculateVolume(audioSample.Data)
			if volume > -50 { // Only log if volume is above threshold
				fmt.Printf("🎵 Audio sample received - Volume: %.1f dB\n", volume)
			}
		}
	})

	// Server events
	client.On(retellAI.EventUpdate, func(data interface{}) {
		fmt.Printf("📊 Update received: %+v\n", data)
	})

	client.On(retellAI.EventMetadata, func(data interface{}) {
		fmt.Printf("📋 Metadata received: %+v\n", data)
	})

	client.On(retellAI.EventNodeTransition, func(data interface{}) {
		fmt.Printf("🔄 Node transition: %+v\n", data)
	})

	// Error handling
	client.On(retellAI.EventError, func(data interface{}) {
		if errMsg, ok := data.(string); ok {
			fmt.Printf("❌ Error: %s\n", errMsg)
		}
	})
}

// simulateCallInteractions simulates various call interactions
func simulateCallInteractions(client *retellAI.RetellWebClient) {
	// Wait a bit for call to establish
	time.Sleep(2 * time.Second)

	// Demonstrate muting/unmuting
	fmt.Println("\n--- Testing Microphone Controls ---")

	fmt.Println("🔇 Muting microphone...")
	if err := client.Mute(); err != nil {
		fmt.Printf("Error muting: %v\n", err)
	}

	time.Sleep(2 * time.Second)

	fmt.Println("🔊 Unmuting microphone...")
	if err := client.Unmute(); err != nil {
		fmt.Printf("Error unmuting: %v\n", err)
	}

	// Demonstrate custom media stream
	time.Sleep(2 * time.Second)
	fmt.Println("\n--- Testing Custom Media Stream ---")

	// Create a mock custom media stream
	customStream := retellAI.NewMediaStreamTrack("audio", "custom-stream-123")

	fmt.Println("📤 Sending custom media stream...")
	if err := client.SendCustomMediaStream(customStream); err != nil {
		fmt.Printf("Error sending custom stream: %v\n", err)
	}

	// Wait and then resume microphone
	time.Sleep(3 * time.Second)

	fmt.Println("🎤 Resuming microphone...")
	if err := client.ResumeMicrophone(); err != nil {
		fmt.Printf("Error resuming microphone: %v\n", err)
	}

	// Show track status
	time.Sleep(1 * time.Second)
	fmt.Println("\n--- Track Status ---")
	status := client.GetTrackStatus()
	fmt.Printf("Microphone Enabled: %t\n", status.MicrophoneEnabled)
	fmt.Printf("Total Tracks: %d\n", status.TotalTracks)
	for i, track := range status.PublishedTracks {
		fmt.Printf("Track %d: %s (%s - %s)\n", i+1, track.Name, track.Kind, track.Source)
	}

	// Show connection status
	fmt.Printf("\nConnection Status: Connected=%t, Agent Talking=%t\n",
		client.IsConnected(), client.IsAgentTalking())

	if agentStream := client.GetAgentMediaStream(); agentStream != nil {
		fmt.Printf("Agent Stream: ID=%s, Kind=%s, Enabled=%t\n",
			agentStream.ID, agentStream.Kind, agentStream.IsEnabled())
	}
}

// calculateVolume calculates the RMS volume of audio samples
func calculateVolume(samples []float32) float64 {
	if len(samples) == 0 {
		return -100.0
	}

	sum := float64(0)
	for _, sample := range samples {
		sum += float64(sample) * float64(sample)
	}

	rms := math.Sqrt(sum / float64(len(samples)))
	if rms > 0 {
		return 20 * log10(rms)
	}

	return -100.0
}

// log10 calculates base-10 logarithm using math package
func log10(x float64) float64 {
	if x <= 0 {
		return -100.0
	}
	return math.Log10(x)
}

// waitForShutdown waits for interrupt signal and gracefully shuts down
func waitForShutdown(client *retellAI.RetellWebClient) {
	// Create channel to listen for interrupt signals
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Wait for signal
	fmt.Println("\n--- Call Active ---")
	fmt.Println("Press Ctrl+C to end the call...")
	<-c

	// Graceful shutdown
	fmt.Println("\n🛑 Shutting down...")
	client.StopCall()

	// Give some time for cleanup
	time.Sleep(500 * time.Millisecond)
	fmt.Println("👋 Goodbye!")
}
