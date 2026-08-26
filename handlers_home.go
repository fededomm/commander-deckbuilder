// Handler della home: elenco mazzi, creazione, cancellazione e import Moxfield.
package main

import (
	"net/http"
	"strconv"
	"strings"
)

func home(w http.ResponseWriter, r *http.Request) {
	decks, err := listDecks(db)
	if fail(w, err) {
		return
	}
	render(w, r, homePage(decks))
}

func createDeckHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "nome obbligatorio", http.StatusBadRequest)
		return
	}
	id, err := createDeck(db, name)
	if fail(w, err) {
		return
	}
	w.Header().Set("HX-Redirect", "/deck/"+strconv.FormatInt(id, 10))
	w.WriteHeader(http.StatusNoContent)
}

func deleteDeckHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if fail(w, deleteDeck(db, id)) { // le carte seguono via ON DELETE CASCADE
		return
	}
	decks, err := listDecks(db)
	if fail(w, err) {
		return
	}
	render(w, r, deckList(decks))
}

func importHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	lines, skipped := parseMoxfield(r.FormValue("list"))
	if len(lines) == 0 {
		render(w, r, importError("nessuna riga riconosciuta"))
		return
	}
	rows, missing, err := resolvePrintings(r.Context(), lines)
	if err != nil {
		render(w, r, importError(err.Error()))
		return
	}
	if len(rows) == 0 {
		render(w, r, importError("nessuna carta risolta"))
		return
	}
	id, err := createDeck(db, name)
	if fail(w, err) {
		return
	}
	if fail(w, insertCards(db, id, rows)) {
		return
	}
	// se qualcosa non è stato importato l'utente deve saperlo prima di andarsene
	lost := skipped
	for _, m := range missing {
		lost = append(lost, m.Name)
	}
	if len(lost) == 0 {
		w.Header().Set("HX-Redirect", "/deck/"+strconv.FormatInt(id, 10))
	}
	render(w, r, importResult(id, len(rows), len(lines), lost))
}
