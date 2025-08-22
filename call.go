package main

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/pion/webrtc/v3"
)

// ========================= Registro de llamadas =========================

type Call struct {
	ID           string
	PC           *webrtc.PeerConnection
	Done         chan struct{}
	AudioStarted bool // Prevenir múltiples goroutines de audio
	AudioMutex   sync.Mutex
}

var calls sync.Map // map[string]*Call

func newCallID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), rand.Intn(100000))
}

func storeCall(c *Call) {
	calls.Store(c.ID, c)
}

func loadCall(id string) (*Call, bool) {
	if v, ok := calls.Load(id); ok {
		return v.(*Call), true
	}
	return nil, false
}

func deleteCall(id string) {
	calls.Delete(id)
}

// getAllCalls retorna todos los IDs de llamadas activas
func getAllCalls() []string {
	var ids []string
	calls.Range(func(k, _ any) bool {
		ids = append(ids, k.(string))
		return true
	})
	return ids
}

// createCall crea una nueva llamada con PeerConnection y callbacks
func createCall() (*Call, error) {
	// MediaEngine (Opus, etc.)
	var m webrtc.MediaEngine
	if err := m.RegisterDefaultCodecs(); err != nil {
		return nil, fmt.Errorf("no se pudo registrar codecs: %w", err)
	}

	// SettingEngine: responder como DTLS CLIENT (setup:active) y solo UDP4 opcional
	se := webrtc.SettingEngine{}
	se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	if err := se.SetAnsweringDTLSRole(webrtc.DTLSRoleClient); err != nil {
		return nil, fmt.Errorf("SetAnsweringDTLSRole error: %w", err)
	}

	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(&m),
		webrtc.WithSettingEngine(se),
	)

	// Crear PeerConnection
	peer, err := api.NewPeerConnection(rtcConfig)
	if err != nil {
		return nil, fmt.Errorf("error creando PeerConnection: %w", err)
	}

	// Crear y registrar la "Call"
	callID := newCallID()
	call := &Call{ID: callID, PC: peer, Done: make(chan struct{})}
	storeCall(call)

	return call, nil
}

// closeCall cierra una llamada y la elimina del registro
func closeCall(call *Call) {
	_ = call.PC.Close()
	select {
	case <-call.Done:
	default:
		close(call.Done)
	}
	deleteCall(call.ID)
}
