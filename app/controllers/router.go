package controllers

import (
	"net/http"

	"github.com/gorilla/mux"
)

func Router(tm *TopicManager) *mux.Router {
	r := mux.NewRouter()
	r.PathPrefix("/css/").Handler(http.FileServer(http.FS(ffs)))
	r.PathPrefix("/js/").Handler(http.FileServer(http.FS(ffs)))
	// WASM builds are large — serve from the filesystem, not embedded in the binary.
	r.PathPrefix("/wasm/").Handler(http.StripPrefix("/wasm/", http.FileServer(http.Dir("./app/wasm/"))))

	r.HandleFunc("/home/", Home).Methods(http.MethodGet)
	r.HandleFunc("/", Index).Methods(http.MethodGet)

	// New room routes (you'll add handlers/templates later)
	r.HandleFunc("/rooms", RoomsLanding).Methods(http.MethodGet)   // create/join page
	r.HandleFunc("/rooms", CreateRoom).Methods(http.MethodPost)    // generate room code
	r.HandleFunc("/rooms/{roomID}", Video).Methods(http.MethodGet) // video page for a room

	// WebSocket for signaling, scoped to a room
	r.HandleFunc("/rooms/{roomID}/ws", VideoConnections(tm)).Methods(http.MethodGet)

	fileServer := http.FileServer(http.Dir("./app/data/imgdata/"))
	r.Handle("/static/{reqFile}", http.StripPrefix("/static", fileServer))

	return r
}
