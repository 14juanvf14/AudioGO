package main

import (
	"log"
	"math/rand"
	"net/http"
	"time"
	"webrtc-audio-server/whatsapp"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	mux := http.NewServeMux()
	mux.HandleFunc("/sdp", whatsapp.HandleSDP)       // crea/negocia una llamada
	mux.HandleFunc("/hangup", whatsapp.HandleHangup) // cuelga por id
	mux.HandleFunc("/status", whatsapp.HandleStatus) // lista llamadas activas

	log.Printf("Servidor escuchando en %s (POST /sdp, GET /hangup?id=..., GET /status)", whatsapp.ServerPort)
	if err := http.ListenAndServe(whatsapp.ServerPort, mux); err != nil {
		log.Fatal(err)
	}
}
