package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"
)

func routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.FileServerFS(staticFS))

	mux.HandleFunc("GET /{$}", home)
	mux.HandleFunc("POST /decks", createDeckHandler)
	mux.HandleFunc("DELETE /decks/{id}", deleteDeckHandler)
	mux.HandleFunc("POST /import", importHandler)

	mux.HandleFunc("GET /deck/{id}", deckPageHandler)
	mux.HandleFunc("GET /deck/{id}/search", searchHandler)
	mux.HandleFunc("POST /deck/{id}/cards", addCardHandler)
	mux.HandleFunc("POST /deck/{id}/prices", refreshPricesHandler)
	mux.HandleFunc("GET /deck/{id}/export", exportHandler)

	mux.HandleFunc("POST /cards/{id}/toggle", toggleHandler)
	mux.HandleFunc("DELETE /cards/{id}", deleteCardHandler)
	return mux
}

func render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := c.Render(r.Context(), w); err != nil {
		log.Printf("render: %v", err)
	}
}

func renderChecklist(w http.ResponseWriter, r *http.Request, deckID int64) {
	name, err := deckName(db, deckID)
	if fail(w, err) {
		return
	}
	cards, err := deckCards(db, deckID)
	if fail(w, err) {
		return
	}
	render(w, r, checklist(Deck{ID: deckID, Name: name}, cards))
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "id non valido", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

// fail logga e risponde 500. Torna true se c'era un errore, per uscire dall'handler.
func fail(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	log.Printf("errore: %v", err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
	return true
}

// safeFilename: il nome del mazzo finisce in un header HTTP, niente virgolette o a capo.
func safeFilename(name string) string {
	clean := strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '_' ||
			(r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return r
		}
		return -1
	}, name)
	if clean = strings.TrimSpace(clean); clean == "" {
		return "mazzo"
	}
	return clean
}
