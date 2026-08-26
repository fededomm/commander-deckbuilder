package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Scryfall chiede uno User-Agent identificabile e ~100ms fra le richieste.
// Dal browser lo faceva il browser, qui tocca a noi.
const userAgent = "commander-deckbuilder/0.2 (https://github.com/local)"

var scryClient = &http.Client{Timeout: 20 * time.Second}

// Su una ricerca senza risultati Scryfall risponde 404: non è un errore da mostrare.
var errNotFound = errors.New("nessun risultato")

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

func scryfallGet(path string, out any) error {
	req, err := http.NewRequest("GET", "https://api.scryfall.com"+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	res, err := scryClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return errNotFound
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("Scryfall %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

// scryfallSearch: 0 risultati non è un errore, Scryfall risponde 404 su "nessun match".
func scryfallSearch(q string) ([]scryCard, error) {
	var body struct {
		Data []scryCard `json:"data"`
	}
	err := scryfallGet("/cards/search?unique=cards&q="+url.QueryEscape(q), &body)
	if err == errNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(body.Data) > 30 {
		body.Data = body.Data[:30]
	}
	return body.Data, nil
}

func scryfallCard(id string) (scryCard, error) {
	var c scryCard
	err := scryfallGet("/cards/"+url.PathEscape(id), &c)
	return c, err
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
func scryfallCollection(ids []identifier) ([]scryCard, error) {
	var out []scryCard
	for i := 0; i < len(ids); i += 75 {
		chunk := ids[i:min(i+75, len(ids))]
		payload, err := json.Marshal(map[string]any{"identifiers": chunk})
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequest("POST", "https://api.scryfall.com/cards/collection", bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "application/json") // Scryfall rifiuta senza, anche in POST
		req.Header.Set("Content-Type", "application/json")
		res, err := scryClient.Do(req)
		if err != nil {
			return nil, err
		}
		var body struct {
			Data    []scryCard `json:"data"`
			Details string     `json:"details"`
		}
		err = json.NewDecoder(res.Body).Decode(&body)
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("Scryfall %d: %s", res.StatusCode, body.Details)
		}
		if err != nil {
			return nil, err
		}
		out = append(out, body.Data...)
		if i+75 < len(ids) {
			time.Sleep(100 * time.Millisecond) // rate limit chiesto da Scryfall
		}
	}
	return out, nil
}
