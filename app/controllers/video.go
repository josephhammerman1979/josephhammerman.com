package controllers

import (
	"html/template"
	"net/http"

	"github.com/gorilla/mux"
)

var videoTemplatePath = append([]string{templatePath + "video.gohtml"}, baseTemplatePaths...)

type videoPage struct {
	RoomID string
}

func Video(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	roomID := vars["roomID"]

	tmpl, err := template.ParseFS(ffs, videoTemplatePath...)
	if err != nil {
		internalError(err, w)
		return
	}

	data := videoPage{RoomID: roomID}

	if err := tmpl.Execute(w, data); err != nil {
		internalError(err, w)
		return
	}
}
