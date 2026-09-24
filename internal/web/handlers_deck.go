// Handler della pagina mazzo: ricerca, checklist, prezzi ed export.
package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"commander-deckbuilder/internal/ui"
)

func (s *server) deckPage(c echo.Context) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	d, cards, err := s.svc.Deck(id)
	if err != nil {
		return err
	}
	return render(c, ui.DeckPage(d, cards))
}

func (s *server) search(c echo.Context) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	q := strings.TrimSpace(c.QueryParam("q"))
	if q == "" {
		return c.NoContent(http.StatusOK) // campo svuotato: risultati vuoti, non un errore
	}
	prints, searchErr := s.svc.Search(c.Request().Context(), q)
	return render(c, ui.SearchResults(id, prints, searchErr))
}

// prints riempie la dialog delle ristampe: una pagina di griglia per volta.
// L'utente sceglie l'espansione invece di prendersi quella principale.
func (s *server) prints(c echo.Context) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	oracle, name := c.QueryParam("oracle"), c.QueryParam("name")
	if oracle == "" && name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "serve oracle o name")
	}
	num, _ := strconv.Atoi(c.QueryParam("page"))
	// ponytail: ogni cambio pagina richiede di nuovo l'elenco a Scryfall invece
	// di tenerlo in una cache. Una carta ha ~150 stampe: la pagina arriva in un
	// colpo solo. Se pesasse, nel servizio va una cache per oracle_id.
	prints, searchErr := s.svc.Prints(c.Request().Context(), oracle, name)
	if len(prints) > 0 {
		name = prints[0].Name // il nome vero, anche quando ho cercato per oracle_id
	}
	return render(c, ui.PrintsDialog(id, name, ui.Paginate(prints, num), c.Request().URL.Query(), searchErr))
}

func (s *server) addCard(c echo.Context) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	if err := s.svc.AddCard(c.Request().Context(), id, c.FormValue("scryfall_id")); err != nil {
		return err
	}
	return s.renderChecklist(c, id)
}

// purchased: una carta (la casella della riga) o tante ("seleziona tutte").
// Risponde solo con contatori e statistiche out-of-band: le caselle sono già
// giuste nel browser, e ridisegnare le righe cancellava la spunta di una carta
// cliccata mentre la richiesta precedente era ancora in volo.
func (s *server) purchased(c echo.Context) error {
	deckID, err := pathID(c)
	if err != nil {
		return err
	}
	var ids []int64
	for _, v := range strings.Split(c.FormValue("ids"), ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "ids non validi")
		}
		ids = append(ids, id)
	}
	if err := s.svc.SetPurchased(deckID, ids, c.FormValue("purchased") != ""); err != nil {
		return err
	}
	cards, err := s.svc.Cards(deckID)
	if err != nil {
		return err
	}
	return render(c, ui.PurchaseStats(cards))
}

func (s *server) deleteCard(c echo.Context) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	deckID, err := s.svc.DeleteCard(id)
	if err != nil {
		return err
	}
	return s.renderChecklist(c, deckID)
}

func (s *server) refreshPrices(c echo.Context) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	if err := s.svc.RefreshPrices(c.Request().Context(), id); err != nil {
		return err
	}
	return s.renderChecklist(c, id)
}

func (s *server) export(c echo.Context) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	name, list, err := s.svc.Export(id)
	if err != nil {
		return err
	}
	c.Response().Header().Set(echo.HeaderContentDisposition,
		`attachment; filename="`+safeFilename(name)+`.txt"`)
	return c.String(http.StatusOK, list)
}
