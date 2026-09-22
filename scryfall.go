package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"slices"
	"strconv"
	"time"

	"github.com/go-resty/resty/v2"
)

// Scryfall chiede uno User-Agent identificabile e ~100ms fra le richieste.
// Dal browser lo faceva il browser, qui tocca a noi. L'URL dev'essere vero:
// è il recapito con cui Scryfall ci scrive invece di bannarci e basta.
var userAgent = "commander-deckbuilder/" + buildVersion() +
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

// Quante carte mostrare: oltre non si scorre, si ricerca meglio.
const maxResults = 30

// scryfallSearch: una riga per carta, la stampa che Scryfall considera principale.
func scryfallSearch(ctx context.Context, q string) ([]scryCard, error) {
	cards, err := scryfallQuery(ctx, map[string]string{"unique": "cards", "q": q})
	return cards[:min(len(cards), maxResults)], err
}

// scryfallPrints elenca tutte le ristampe di una carta. Le chiedo dalla più
// recente e le riordino per prezzo: è una lista della spesa, e "order=eur" di
// Scryfall mette in testa le non quotate, cioè proprio quelle che non si comprano.
// Qui non taglio: la griglia le impagina tutte.
func scryfallPrints(ctx context.Context, q string) ([]scryCard, error) {
	cards, err := scryfallQuery(ctx, map[string]string{
		"unique": "prints", "order": "released", "dir": "desc", "q": q})
	if err != nil {
		return nil, err
	}
	// ponytail: una pagina sola di Scryfall (175 stampe). Nessuna carta ne ha
	// di più; se un giorno succedesse, qui si segue "next_page".
	sortByPrice(cards)
	return cards, nil
}

// sortByPrice: le quotate prima, dalla più economica; le altre in coda nell'ordine
// in cui sono arrivate (dalla più recente). Stabile, così il secondo criterio tiene.
func sortByPrice(cards []scryCard) {
	unpriced := func(p float64) int {
		if p == 0 {
			return 1
		}
		return 0
	}
	slices.SortStableFunc(cards, func(a, b scryCard) int {
		pa, pb := a.price(false), b.price(false)
		if c := cmp.Compare(unpriced(pa), unpriced(pb)); c != 0 {
			return c
		}
		return cmp.Compare(pa, pb)
	})
}

// scryfallQuery: 0 risultati non è un errore, Scryfall risponde 404 su "nessun match".
func scryfallQuery(ctx context.Context, params map[string]string) ([]scryCard, error) {
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
