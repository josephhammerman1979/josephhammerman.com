package app

import (
	"code.squarespace.net/db/squaremeet/app/controllers"

	"log"
	"net"
	"net/http"
)

func Run(listenAndServe func(string, http.Handler) error, port string) error {
	log.Println("Listening on port: ", port)
	if err := listenAndServe(net.JoinHostPort("", port), controllers.Router()); err != nil {
		return err
	}
	return nil
}
