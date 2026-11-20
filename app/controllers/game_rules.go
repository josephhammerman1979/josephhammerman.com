package controllers

import (
	"html/template"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

var gameRulesTemplatePath = append([]string{templatePath + "game_rules.gohtml"}, baseTemplatePaths...)

type gameRulesPage struct {
	Game  *games.Def   // nil on the index page
	Games []*games.Def // the catalog, for the index and the nav
}

// GET /games/rules and /games/rules/{gameID} – how to play each dice game,
// in the words of the rules sheet.
func GameRules(w http.ResponseWriter, r *http.Request) {
	page := gameRulesPage{Games: games.Catalog()}
	if id := mux.Vars(r)["gameID"]; id != "" {
		page.Game = games.Lookup(id)
		if page.Game == nil {
			http.NotFound(w, r)
			return
		}
	}
	tmpl, err := template.ParseFS(ffs, gameRulesTemplatePath...)
	if err != nil {
		internalError(err, w)
		return
	}
	if err := tmpl.Execute(w, page); err != nil {
		internalError(err, w)
	}
}
