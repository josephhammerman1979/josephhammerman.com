package controllers

import (
	"html/template"
	"net/http"
	"time"

	"github.com/josephhammerman1979/josephhammerman.com/app/data"
)

var homeTemplatePath = append([]string{templatePath + "index.gohtml"}, baseTemplatePaths...)

type homePage struct {
	imageInfo
	ImageCaption  string
	ImageLocation string
	Year          string
}

func makeHomePage(image *data.Image) homePage {
	return homePage{
		imageInfo: imageInfo{
			ImageURL:    imageBaseURL + image.ID,
			ImageWidth:  image.Width,
			ImageHeight: image.Height,
		},
		ImageCaption:  image.Caption,
		ImageLocation: image.Location,
		Year:          time.Now().Format("2006"),
	}
}

func Home(w http.ResponseWriter, r *http.Request) {

	var image *data.Image

	image, err := image.GetImage(homeImage)

	if err != nil {
		internalError(err, w)
		return
	}
	tmpl, err := template.ParseFS(ffs, homeTemplatePath...)
	if err != nil {
		internalError(err, w)
		return
	}

	imagePage := makeHomePage(image)
	err = tmpl.Execute(w, imagePage)
        if err != nil {
                internalError(err, w)
                return
        }
}
