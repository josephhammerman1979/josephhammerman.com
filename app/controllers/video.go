package controllers

import (
        "html/template"
        "net/http"

)

var videoTemplatePath = append([]string{templatePath + "video.gohtml"}, baseTemplatePaths...)

func Video(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFS(ffs, videoTemplatePath...)
        if err != nil {
                internalError(err, w)
                return
        }
	err = tmpl.Execute(w, tmpl)
        if err != nil {
                internalError(err, w)
                return
        }
}
