// Handler della pagina mazzo: ricerca, checklist, prezzi ed export.
package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
)

func deckPageHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	name, err := deckName(db, id)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if fail(w, err) {
		return
	}
	cards, err := deckCards(db, id)
	if fail(w, err) {
		return
	}
	render(w, r, deckPage(Deck{ID: id, Name: name}, cards))
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		return // campo svuotato: risultati vuoti, non un errore
	}
	cards, err := scryfallSearch(r.Context(), q)
	render(w, r, searchResults(id, cards, err))
}

func addCardHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	// Il client manda solo l'id: la carta la rileggo da Scryfall, così non ho
	// prezzi e immagini che rimbalzano avanti e indietro negli attributi HTML.
	card, err := scryfallCard(r.Context(), r.FormValue("scryfall_id"))
	if fail(w, err) {
		return
	}
	if fail(w, insertCards(db, id, []Card{card.toCard(1, false)})) {
		return
	}
	renderChecklist(w, r, id)
}

func toggleHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	deckID, err := togglePurchased(db, id)
	if fail(w, err) {
		return
	}
	renderChecklist(w, r, deckID)
}

func deleteCardHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	deckID, err := deleteCard(db, id)
	if fail(w, err) {
		return
	}
	renderChecklist(w, r, deckID)
}

// refreshPricesHandler riallinea i prezzi a Scryfall in un colpo solo.
func refreshPricesHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	cards, err := deckCards(db, id)
	if fail(w, err) {
		return
	}
	ids := make([]identifier, len(cards))
	for i, c := range cards {
		ids[i] = identifier{ID: c.ScryfallID}
	}
	fresh, err := scryfallCollection(r.Context(), ids)
	if fail(w, err) {
		return
	}
	prices := make(map[string]float64, len(fresh))
	for _, c := range fresh {
		prices[c.ID] = c.price(false)
	}
	for _, c := range cards {
		if p, ok := prices[c.ScryfallID]; ok && p != c.PriceEUR {
			if fail(w, setPrice(db, c.ID, p)) {
				return
			}
		}
	}
	renderChecklist(w, r, id)
}

func exportHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	name, err := deckName(db, id)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if fail(w, err) {
		return
	}
	cards, err := deckCards(db, id)
	if fail(w, err) {
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeFilename(name)+`.txt"`)
	fmt.Fprint(w, toMoxfield(cards))
}
