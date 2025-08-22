package main

import (
	"log"
	"math/rand"
	"net/http"
	"time"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	mux := http.NewServeMux()
	mux.HandleFunc("/sdp", handleSDP)       // crea/negocia una llamada
	mux.HandleFunc("/hangup", handleHangup) // cuelga por id
	mux.HandleFunc("/status", handleStatus) // lista llamadas activas

	log.Printf("Servidor escuchando en %s (POST /sdp, GET /hangup?id=..., GET /status)", ServerPort)
	if err := http.ListenAndServe(ServerPort, mux); err != nil {
		log.Fatal(err)
	}
}
