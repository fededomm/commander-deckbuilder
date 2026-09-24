// Handler della home: elenco mazzi, creazione, cancellazione e import Moxfield.
package main

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

func home(c echo.Context) error {
	decks, err := listDecks(db)
	if err != nil {
		return err
	}
	return render(c, homePage(decks))
}

func createDeckHandler(c echo.Context) error {
	name := strings.TrimSpace(c.FormValue("name"))
	if name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "nome obbligatorio")
	}
	id, err := createDeck(db, name)
	if err != nil {
		return err
	}
	c.Response().Header().Set("HX-Redirect", "/deck/"+strconv.FormatInt(id, 10))
	return c.NoContent(http.StatusNoContent)
}

func deleteDeckHandler(c echo.Context) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	if err := deleteDeck(db, id); err != nil { // le carte seguono via ON DELETE CASCADE
		return err
	}
	decks, err := listDecks(db)
	if err != nil {
		return err
	}
	return render(c, deckList(decks))
}

func importHandler(c echo.Context) error {
	name := strings.TrimSpace(c.FormValue("name"))
	lines, skipped := parseMoxfield(c.FormValue("list"))
	if len(lines) == 0 {
		return render(c, importError("nessuna riga riconosciuta"))
	}
	rows, missing, err := resolvePrintings(c.Request().Context(), lines)
	if err != nil {
		return render(c, importError(err.Error()))
	}
	if len(rows) == 0 {
		return render(c, importError("nessuna carta risolta"))
	}
	id, err := createDeck(db, name)
	if err != nil {
		return err
	}
	if err := insertCards(db, id, rows); err != nil {
		return err
	}
	// se qualcosa non è stato importato l'utente deve saperlo prima di andarsene
	lost := skipped
	for _, m := range missing {
		lost = append(lost, m.Name)
	}
	if len(lost) == 0 {
		c.Response().Header().Set("HX-Redirect", "/deck/"+strconv.FormatInt(id, 10))
	}
	return render(c, importResult(id, len(rows), len(lines), lost))
}
