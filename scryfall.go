package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-resty/resty/v2"
)

// Scryfall chiede uno User-Agent identificabile e ~100ms fra le richieste.
// Dal browser lo faceva il browser, qui tocca a noi.
const userAgent = "commander-deckbuilder/0.2 (https://github.com/local)"

var scryfall = resty.New().
	SetBaseURL("https://api.scryfall.com").
	SetHeader("User-Agent", userAgent).
	SetHeader("Accept", "application/json"). // Scryfall rifiuta senza, anche in POST
	SetTimeout(20 * time.Second)

// Su una ricerca senza risultati Scryfall risponde 404: non è un errore da mostrare.
var errNotFound = errors.New("nessun risultato")

// scryError è il corpo che Scryfall manda sugli errori: "details" spiega cosa manca.
type scryError struct {
	Details string `json:"details"`
}

func restErr(res *resty.Response) error {
	if e, ok := res.Error().(*scryError); ok && e.Details != "" {
		return fmt.Errorf("Scryfall %d: %s", res.StatusCode(), e.Details)
	}
	return fmt.Errorf("Scryfall %d", res.StatusCode())
}

type scryCard struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	TypeLine        string            `json:"type_line"`
	ManaCost        string            `json:"mana_cost"`
	Set             string            `json:"set"`
	CollectorNumber string            `json:"collector_number"`
	ImageURIs       map[string]string `json:"image_uris"`
	CardFaces       []struct {
		ImageURIs map[string]string `json:"image_uris"`
	} `json:"card_faces"`
	Prices struct {
		EUR     *string `json:"eur"`
		EURFoil *string `json:"eur_foil"`
	} `json:"prices"`
}

// image: le bifacciali non hanno image_uris in cima, sta nella prima faccia.
func (c scryCard) image() string {
	if u := c.ImageURIs["normal"]; u != "" {
		return u
	}
	if len(c.CardFaces) > 0 {
		return c.CardFaces[0].ImageURIs["normal"]
	}
	return ""
}

// price: quotazione Cardmarket in EUR, 0 se la carta non è quotata.
// Per le foil, se manca eur_foil si ripiega sul prezzo normale.
func (c scryCard) price(foil bool) float64 {
	p := c.Prices.EUR
	if foil && c.Prices.EURFoil != nil {
		p = c.Prices.EURFoil
	}
	if p == nil {
		return 0
	}
	v, _ := strconv.ParseFloat(*p, 64)
	return v
}

func (c scryCard) toCard(qty int, foil bool) Card {
	return Card{
		ScryfallID:      c.ID,
		Name:            c.Name,
		TypeLine:        c.TypeLine,
		ManaCost:        c.ManaCost,
		Image:           c.image(),
		PriceEUR:        c.price(foil),
		Qty:             qty,
		SetCode:         c.Set,
		CollectorNumber: c.CollectorNumber,
		Foil:            foil,
	}
}

// cardList è la forma di risposta di /cards/search e /cards/collection.
type cardList struct {
	Data []scryCard `json:"data"`
}

func request(ctx context.Context, result any) *resty.Request {
	return scryfall.R().SetContext(ctx).SetResult(result).SetError(&scryError{})
}

// scryfallSearch: 0 risultati non è un errore, Scryfall risponde 404 su "nessun match".
func scryfallSearch(ctx context.Context, q string) ([]scryCard, error) {
	var list cardList
	res, err := request(ctx, &list).
		SetQueryParams(map[string]string{"unique": "cards", "q": q}).
		Get("/cards/search")
	if err != nil {
		return nil, err
	}
	if res.StatusCode() == http.StatusNotFound {
		return nil, nil
	}
	if res.IsError() {
		return nil, restErr(res)
	}
	if len(list.Data) > 30 {
		list.Data = list.Data[:30]
	}
	return list.Data, nil
}

func scryfallCard(ctx context.Context, id string) (scryCard, error) {
	var c scryCard
	res, err := request(ctx, &c).SetPathParam("id", id).Get("/cards/{id}")
	if err != nil {
		return c, err
	}
	if res.StatusCode() == http.StatusNotFound {
		return c, errNotFound
	}
	if res.IsError() {
		return c, restErr(res)
	}
	return c, nil
}

// identifier è una riga della richiesta /cards/collection: id, oppure set+numero,
// oppure solo il nome. I campi vuoti spariscono dal JSON.
type identifier struct {
	ID              string `json:"id,omitempty"`
	Set             string `json:"set,omitempty"`
	CollectorNumber string `json:"collector_number,omitempty"`
	Name            string `json:"name,omitempty"`
}

// scryfallCollection risolve identificatori in stampe. L'endpoint accetta max 75
// identifiers per richiesta, quindi spezza in blocchi.
func scryfallCollection(ctx context.Context, ids []identifier) ([]scryCard, error) {
	var out []scryCard
	for i := 0; i < len(ids); i += 75 {
		chunk := ids[i:min(i+75, len(ids))]
		var list cardList
		res, err := request(ctx, &list).
			SetBody(map[string]any{"identifiers": chunk}).
			Post("/cards/collection")
		if err != nil {
			return nil, err
		}
		if res.IsError() {
			return nil, restErr(res)
		}
		out = append(out, list.Data...)
		if i+75 < len(ids) {
			time.Sleep(100 * time.Millisecond) // rate limit chiesto da Scryfall
		}
	}
	return out, nil
}
