package main

import "github.com/pion/webrtc/v3"

// ========================= Configuración WebRTC =========================

// Cambia/añade TURN si lo necesitas para NATs estrictos
var rtcConfig = webrtc.Configuration{
	ICEServers: []webrtc.ICEServer{
		{
			URLs: []string{
				"stun:stun.l.google.com:19302",
				"stun:stun.l.google.com:19305",
			},
		},
		// {
		// 	URLs:       []string{"turn:tu-turn.example.com:3478"},
		// 	Username:   "user",
		// 	Credential: "pass",
		// },
	},
}

// Autocolgado por inactividad RTP (0 = deshabilitado)
const IdleHangupSeconds = 0

// Configuración de audio saliente
const (
	OutOGGPath     = "/Users/haroldcamargo/Desktop/aldeamo/retell_cursor/AudioGO/audio-1755886077.ogg"
	OutTimeoutSec  = 25    // 0 = sin timeout; >0 segundos para cortar el envío
	CloseOnTimeout = false // true: cierra la llamada al expirar el timeout
	ServerPort     = ":8080"
	AudioFrameTime = 20 // milisegundos por frame de audio
)
