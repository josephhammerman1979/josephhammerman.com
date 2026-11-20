package games

// RulesLinks maps game IDs to their tab in the shared rules document.  Games
// without an entry link only to their rules page on this site.
//
// To add one, open the game's tab in the document, copy its link, and add a
// line like:
//
//	"pig": "https://docs.google.com/document/d/<doc-id>/edit?tab=t.<tab-id>",
var RulesLinks = map[string]string{}
