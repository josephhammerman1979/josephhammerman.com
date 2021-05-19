package main

import (
	"code.squarespace.net/db/squaremeet/app"
	"net/http"
)

func main() {
	if err := app.Run(http.ListenAndServe, "8000"); err != nil {
		panic(err)
	}
}
