// app/controllers/rooms.go
package controllers

import (
	"crypto/rand"
	"encoding/base64"
	"html/template"
	"io"
	"net/http"
)

var roomsTemplatePath = append([]string{templatePath + "rooms.gohtml"}, baseTemplatePaths...)

type roomsPage struct {
	RoomID string
	Error  string
}

// GET /rooms – landing page with create/join form.
func RoomsLanding(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFS(ffs, roomsTemplatePath...)
	if err != nil {
		internalError(err, w)
		return
	}

	data := roomsPage{}
	if err := tmpl.Execute(w, data); err != nil {
		internalError(err, w)
		return
	}
}

// POST /rooms – create a new room and redirect.
func CreateRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	roomID, err := generateRoomID(12)
	if err != nil {
		internalError(err, w)
		return
	}

	http.Redirect(w, r, "/rooms/"+roomID, http.StatusSeeOther)
}

func generateRoomID(n int) (string, error) {
	// ~6 bits per char, 12 chars ≈ 72 bits entropy.
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	id := base64.RawURLEncoding.EncodeToString(b)
	// Truncate to n chars to keep it short.
	if len(id) > n {
		id = id[:n]
	}
	return id, nil
}
