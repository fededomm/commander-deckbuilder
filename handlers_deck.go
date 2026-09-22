// Handler della pagina mazzo: ricerca, checklist, prezzi ed export.
package main

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

func deckPageHandler(c echo.Context) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	name, err := deckName(db, id)
	if err == sql.ErrNoRows {
		return echo.ErrNotFound
	}
	if err != nil {
		return err
	}
	cards, err := deckCards(db, id)
	if err != nil {
		return err
	}
	return render(c, deckPage(Deck{ID: id, Name: name}, cards))
}

func searchHandler(c echo.Context) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	q := strings.TrimSpace(c.QueryParam("q"))
	if q == "" {
		return c.NoContent(http.StatusOK) // campo svuotato: risultati vuoti, non un errore
	}
	cards, searchErr := scryfallSearch(c.Request().Context(), q)
	return render(c, searchResults(id, cards, searchErr))
}

// printsHandler riempie la dialog delle ristampe: una pagina di griglia per volta.
// L'utente sceglie l'espansione invece di prendersi quella principale.
func printsHandler(c echo.Context) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	// oracle_id identifica la carta a prescindere dalla stampa; manca solo sulle
	// pochissime "reversible", per quelle ripiego sul nome esatto.
	name := c.QueryParam("name")
	q := c.QueryParam("oracle")
	if q != "" {
		q = "oracleid:" + q
	} else if name != "" {
		q = `!"` + strings.ReplaceAll(name, `"`, "") + `"`
	} else {
		return echo.NewHTTPError(http.StatusBadRequest, "serve oracle o name")
	}
	query := c.Request().URL.Query()
	num, _ := strconv.Atoi(c.QueryParam("page"))

	cards, searchErr := scryfallPrints(c.Request().Context(), q)
	if len(cards) > 0 {
		name = cards[0].Name // il nome vero, anche quando ho cercato per oracle_id
	}
	// ponytail: ogni cambio pagina richiede di nuovo l'elenco a Scryfall invece
	// di tenerlo in una cache. App locale, una carta ha ~150 stampe: la pagina
	// arriva in un colpo solo. Se pesasse, qui va una cache per oracle_id.
	return render(c, printsDialog(id, name, paginate(cards, num), query, searchErr))
}

func addCardHandler(c echo.Context) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	// Il client manda solo l'id: la carta la rileggo da Scryfall, così non ho
	// prezzi e immagini che rimbalzano avanti e indietro negli attributi HTML.
	card, err := scryfallCard(c.Request().Context(), c.FormValue("scryfall_id"))
	if err != nil {
		return err
	}
	if err := insertCards(db, id, []Card{card.toCard(1, false)}); err != nil {
		return err
	}
	return renderChecklist(c, id)
}

func toggleHandler(c echo.Context) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	deckID, err := togglePurchased(db, id)
	if err != nil {
		return err
	}
	return renderChecklist(c, deckID)
}

func deleteCardHandler(c echo.Context) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	deckID, err := deleteCard(db, id)
	if err != nil {
		return err
	}
	return renderChecklist(c, deckID)
}

// refreshPricesHandler riallinea i prezzi a Scryfall in un colpo solo.
func refreshPricesHandler(c echo.Context) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	cards, err := deckCards(db, id)
	if err != nil {
		return err
	}
	ids := make([]identifier, len(cards))
	for i, card := range cards {
		ids[i] = identifier{ID: card.ScryfallID}
	}
	fresh, err := scryfallCollection(c.Request().Context(), ids)
	if err != nil {
		return err
	}
	byID := make(map[string]scryCard, len(fresh))
	for _, card := range fresh {
		byID[card.ID] = card
	}
	for _, card := range cards {
		// il prezzo segue la stampa salvata, foil compreso: è quella che l'utente ha scelto
		if f, ok := byID[card.ScryfallID]; ok {
			if p := f.price(card.Foil); p != card.PriceEUR {
				if err := setPrice(db, card.ID, p); err != nil {
					return err
				}
			}
		}
	}
	return renderChecklist(c, id)
}

func exportHandler(c echo.Context) error {
	id, err := pathID(c)
	if err != nil {
		return err
	}
	name, err := deckName(db, id)
	if err == sql.ErrNoRows {
		return echo.ErrNotFound
	}
	if err != nil {
		return err
	}
	cards, err := deckCards(db, id)
	if err != nil {
		return err
	}
	c.Response().Header().Set(echo.HeaderContentDisposition,
		`attachment; filename="`+safeFilename(name)+`.txt"`)
	return c.String(http.StatusOK, toMoxfield(cards))
}
