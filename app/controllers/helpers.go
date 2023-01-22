package controllers

import (
	"embed"
	"encoding/base64"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
)

var (
	ffs = &flexFS{}

	//go:embed templates
	templatesEmbedFS embed.FS
	//go:embed css
	cssEmbedFS embed.FS
	//go:embed js
	jsEmbedFS embed.FS

	homeImage    = "homeImg.png"
	imageBaseURL = "/static/"

	templatePath      = "templates/"
	baseTemplatePaths = []string{
		templatePath + "image.gohtml",
		templatePath + "base.gohtml",
	}
)

type flexFS struct{}

func (f *flexFS) Open(name string) (fs.File, error) {
	if os.Getenv("USE_LOCAL_FS") != "" {
		return os.Open("./app/controllers/" + name)
	}
	if strings.HasPrefix(name, "js/") {
		return jsEmbedFS.Open(name)
	}
	if strings.HasPrefix(name, "css/") {
		return cssEmbedFS.Open(name)
	}
	if strings.HasPrefix(name, "templates/") {
		return templatesEmbedFS.Open(name)
	}
	return nil, errors.New("could not find file")
}

func makeImagePath(imageID string) string {
	return imageBaseURL + encodeImageID(imageID)
}

func encodeImageID(imageID string) string {
	return base64.URLEncoding.EncodeToString([]byte(imageID))
}

func internalError(err error, w http.ResponseWriter) {
	log.Println(err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

type imageInfo struct {
	ImageURL    string
	ImageWidth  int
	ImageHeight int
}

// func decodeImageID(encodedID string) (string, error) {
//	imageID, err := base64.URLEncoding.DecodeString(encodedID)
//	if err != nil {
//		return "", err
//	}
//	return string(imageID), nil
// }
