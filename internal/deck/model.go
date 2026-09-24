// Package deck è la business logic: il modello (mazzi, carte, stampe) e le
// regole che ci girano sopra — categorie, totali, curva di mana, colori, formato
// Moxfield. Niente I/O: né database, né rete, né HTML. Tutti gli altri pacchetti
// dipendono da questo, questo da nessuno.
package deck

import "errors"

// ErrNotFound: il mazzo o la carta chiesti non esistono. Lo store lo restituisce,
// il layer web lo trasforma in 404.
var ErrNotFound = errors.New("non trovato")

type Deck struct {
	ID        int64
	Name      string
	Cards     int     // somma delle qty
	Purchased int     // somma delle qty già acquistate
	Total     float64 // valore del mazzo
	Todo      float64 // quanto resta da comprare
	Colors    string  // colori dei costi di mana, in ordine WUBRG, es. "WUB"
}

type Card struct {
	ID              int64
	DeckID          int64
	ScryfallID      string
	Name            string
	TypeLine        string
	ManaCost        string
	Image           string
	PriceEUR        float64
	Purchased       bool
	Qty             int
	SetCode         string
	CollectorNumber string
	Foil            bool
}

// Print è una stampa di una carta come la mostra l'interfaccia: un risultato di
// ricerca o una ristampa nella dialog. Il servizio la ricava dalla risposta di
// Scryfall, così la UI non dipende dal formato dell'API esterna.
type Print struct {
	ScryfallID      string
	OracleID        string // identifica la carta al di là della stampa
	Name            string
	TypeLine        string
	Set             string
	SetName         string
	ReleasedAt      string // "2024-08-02"
	CollectorNumber string
	Image           string
	PriceEUR        float64 // 0 se non quotata
}
