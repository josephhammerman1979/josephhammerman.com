package controllers

import (
	"fmt"
	"net/http"
	"regexp"
)

func Index(w http.ResponseWriter, r *http.Request) {
	httpProtocolMatched, err := regexp.Match(`localhost`, []byte(r.Host))

	if err != nil {
                internalError(err, w)
                return
        }

	if httpProtocolMatched {
		http.Redirect(w, r, fmt.Sprintf("http://%s/home/", r.Host), http.StatusFound)
	} else {
		http.Redirect(w, r, fmt.Sprintf("https://%s/home/", r.Host), http.StatusFound)
	}
}
