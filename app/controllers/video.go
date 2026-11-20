package controllers

import (
	"html/template"
	"net/http"

	"github.com/gorilla/mux"
)

var videoTemplatePath = append([]string{templatePath + "video.gohtml"}, baseTemplatePaths...)

type videoPage struct {
	RoomID string
	// IsDice shows the dice-game panel. Dice rooms are created with
	// ?game=dice, which invite links carry along.
	IsDice bool
}

func Video(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	roomID := vars["roomID"]

	tmpl, err := template.ParseFS(ffs, videoTemplatePath...)
	if err != nil {
		internalError(err, w)
		return
	}

	data := videoPage{RoomID: roomID, IsDice: r.URL.Query().Get("game") == "dice"}

	if err := tmpl.Execute(w, data); err != nil {
		internalError(err, w)
		return
	}
}
