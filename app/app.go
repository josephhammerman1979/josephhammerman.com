package app

import (
	"github.com/josephhammerman1979/josephhammerman.com/app/controllers"

	"log"
	"net"
	"net/http"
)

func Run(listenAndServe func(string, http.Handler) error, port string) error {
	log.Println("Listening on port: ", port)
	tm := controllers.NewTopicManager()
	if err := listenAndServe(net.JoinHostPort("", port), controllers.Router(tm)); err != nil {
		return err
	}
	return nil
}
