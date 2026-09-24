// Handler della home: elenco mazzi, creazione, cancellazione e import Moxfield.
package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"commander-deckbuilder/internal/ui"
)

func (s *server) home(c echo.Context) error {
	decks, err := s.svc.Decks()
	if err != nil {
		return err
	}
	return render(c, ui.HomePage(decks))
}

func (s *server) createDeck(c echo.Context) error {
	name := strings.TrimSpace(c.FormValue("name"))
	if name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "nome obbligatorio")
	}
	id, err := s.svc.CreateDeck(name)
	if err != nil {
		return err
	}
	c.Response().Header().Set("HX-Redirect", "/deck/"+strconv.FormatInt(id, 10))
	return c.NoContent(http.StatusNoContent)
}

func (s *server) deleteDeck(c echo.Context) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	if err := s.svc.DeleteDeck(id); err != nil {
		return err
	}
	decks, err := s.svc.Decks()
	if err != nil {
		return err
	}
	return render(c, ui.DeckList(decks))
}

func (s *server) importDeck(c echo.Context) error {
	res, err := s.svc.Import(c.Request().Context(),
		strings.TrimSpace(c.FormValue("name")), c.FormValue("list"))
	if err != nil {
		return err
	}
	if res.Problem != "" {
		return render(c, ui.ImportError(res.Problem))
	}
	// tutto importato: si va dritti al mazzo; altrimenti resta il riepilogo
	// con le righe perse, che l'utente deve vedere prima di andarsene
	if len(res.Lost) == 0 {
		c.Response().Header().Set("HX-Redirect", "/deck/"+strconv.FormatInt(res.DeckID, 10))
	}
	return render(c, ui.ImportResult(res.DeckID, res.Imported, res.Lines, res.Lost))
}
