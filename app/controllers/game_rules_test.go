package controllers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

func getRulesPage(t *testing.T, path string) (int, string) {
	t.Helper()
	r := mux.NewRouter()
	r.HandleFunc("/games/rules", GameRules)
	r.HandleFunc("/games/rules/{gameID}", GameRules)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	body, _ := io.ReadAll(rec.Body)
	return rec.Code, string(body)
}

func TestGameRulesPageShowsSheetText(t *testing.T) {
	code, body := getRulesPage(t, "/games/rules/bigpig")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	for _, want := range []string{"Big Pig", "Number of dice: 2", "Double ones are worth 25."} {
		if !strings.Contains(body, want) {
			t.Errorf("rules page missing %q", want)
		}
	}
}

func TestGameRulesIndexListsGames(t *testing.T) {
	code, body := getRulesPage(t, "/games/rules")
	if code != http.StatusOK || !strings.Contains(body, `href="/games/rules/pig"`) {
		t.Fatalf("status %d, index missing pig link", code)
	}
}

func TestGameRulesUnknownGame404s(t *testing.T) {
	if code, _ := getRulesPage(t, "/games/rules/nosuchgame"); code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", code)
	}
}
