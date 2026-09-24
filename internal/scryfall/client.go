// Package scryfall è il client dell'API di Scryfall: ricerca, ristampe, carta
// singola e /cards/collection. Parla solo il formato di Scryfall; tradurlo nel
// modello dell'app è compito del servizio.
package scryfall

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/go-resty/resty/v2"
)

// Scryfall chiede uno User-Agent identificabile e ~100ms fra le richieste.
// Dal browser lo faceva il browser, qui tocca a noi. L'URL dev'essere vero:
// è il recapito con cui Scryfall ci scrive invece di bannarci e basta.
var UserAgent = "commander-deckbuilder/" + buildVersion() +
	" (+https://github.com/fededomm/commander-deckbuilder)"

// buildVersion legge la revisione che il go tool incastona nel binario da sé
// (-buildvcs, attivo di default). Un numero scritto a mano resta indietro alla
// prima release e ci ritroviamo a dichiarare una versione che non esiste.
func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && len(s.Value) >= 7 {
			return s.Value[:7]
		}
	}
	return "dev" // go test e go run non sempre incastonano il VCS
}

var client = resty.New().
	SetBaseURL("https://api.scryfall.com").
	SetHeader("User-Agent", UserAgent).
	SetHeader("Accept", "application/json"). // Scryfall rifiuta senza, anche in POST
	SetTimeout(20 * time.Second)

// Su una ricerca senza risultati Scryfall risponde 404: non è un errore da mostrare.
var ErrNotFound = errors.New("nessun risultato")

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

type Card struct {
	ID              string            `json:"id"`
	OracleID        string            `json:"oracle_id"`
	Name            string            `json:"name"`
	TypeLine        string            `json:"type_line"`
	ManaCost        string            `json:"mana_cost"`
	Set             string            `json:"set"`
	SetName         string            `json:"set_name"`
	ReleasedAt      string            `json:"released_at"`
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

// Image: le bifacciali non hanno image_uris in cima, sta nella prima faccia.
func (c Card) Image() string {
	if u := c.ImageURIs["normal"]; u != "" {
		return u
	}
	if len(c.CardFaces) > 0 {
		return c.CardFaces[0].ImageURIs["normal"]
	}
	return ""
}

// Price: quotazione Cardmarket in EUR, 0 se la carta non è quotata.
// Per le foil, se manca eur_foil si ripiega sul prezzo normale.
func (c Card) Price(foil bool) float64 {
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

// cardList è la forma di risposta di /cards/search e /cards/collection.
type cardList struct {
	Data []Card `json:"data"`
}

func request(ctx context.Context, result any) *resty.Request {
	return client.R().SetContext(ctx).SetResult(result).SetError(&scryError{})
}

// Quante carte mostrare: oltre non si scorre, si ricerca meglio.
const maxResults = 30

// Search: una riga per carta, la stampa che Scryfall considera principale.
func Search(ctx context.Context, q string) ([]Card, error) {
	cards, err := query(ctx, map[string]string{"unique": "cards", "q": q})
	return cards[:min(len(cards), maxResults)], err
}

// Prints elenca tutte le ristampe di una carta, dalla più recente. Qui non taglio:
// l'ordine per prezzo e la paginazione li decide chi le mostra.
//
// ponytail: una pagina sola di Scryfall (175 stampe). Nessuna carta ne ha di più;
// se un giorno succedesse, qui si segue "next_page".
func Prints(ctx context.Context, q string) ([]Card, error) {
	return query(ctx, map[string]string{
		"unique": "prints", "order": "released", "dir": "desc", "q": q})
}

// query: 0 risultati non è un errore, Scryfall risponde 404 su "nessun match".
func query(ctx context.Context, params map[string]string) ([]Card, error) {
	var list cardList
	res, err := request(ctx, &list).
		SetQueryParams(params).
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
	return list.Data, nil
}

// Get legge una stampa dal suo id Scryfall.
func Get(ctx context.Context, id string) (Card, error) {
	var c Card
	res, err := request(ctx, &c).SetPathParam("id", id).Get("/cards/{id}")
	if err != nil {
		return c, err
	}
	if res.StatusCode() == http.StatusNotFound {
		return c, ErrNotFound
	}
	if res.IsError() {
		return c, restErr(res)
	}
	return c, nil
}

// Identifier è una riga della richiesta /cards/collection: id, oppure set+numero,
// oppure solo il nome. I campi vuoti spariscono dal JSON.
type Identifier struct {
	ID              string `json:"id,omitempty"`
	Set             string `json:"set,omitempty"`
	CollectorNumber string `json:"collector_number,omitempty"`
	Name            string `json:"name,omitempty"`
}

// Collection risolve identificatori in stampe. L'endpoint accetta max 75
// identifiers per richiesta, quindi spezza in blocchi.
func Collection(ctx context.Context, ids []Identifier) ([]Card, error) {
	var out []Card
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
