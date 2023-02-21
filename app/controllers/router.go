package controllers

import (
	"github.com/gorilla/mux"
	"net/http"
)

func Router() *mux.Router {
	r := mux.NewRouter()
	r.PathPrefix("/css/").Handler(http.FileServer(http.FS(ffs)))
	r.PathPrefix("/js/").Handler(http.FileServer(http.FS(ffs)))
	r.HandleFunc("/home/", Home).Methods(http.MethodGet)
	r.HandleFunc("/", Index).Methods(http.MethodGet)
        r.HandleFunc("/video", Video).Methods(http.MethodGet)
	//r.HandleFunc("/video/connections", videoConnections).Methods(http.MethodGet)

	// Create a file server which serves files out of the "./ui/static" directory.
	// Note that the path given to the http.Dir function is relative to the project
	// directory root.
	fileServer := http.FileServer(http.Dir("./app/data/imgdata/"))

	// Use the mux.Handle() function to register the file server as the handler for
	// all URL paths that start with "/static/". For matching paths, we strip the
	// "/static" prefix before the request reaches the file server.
	r.Handle("/static/{reqFile}", http.StripPrefix("/static", fileServer))

	return r
}
