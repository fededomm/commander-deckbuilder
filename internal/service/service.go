// Package service contiene i casi d'uso dell'app: quello che succede quando
// l'utente importa una lista, aggiunge una carta o aggiorna i prezzi. Mette
// insieme lo store, il client Scryfall e le regole di deck; non sa niente di
// HTTP né di HTML.
package service

import (
	"context"
	"strings"

	"commander-deckbuilder/internal/deck"
	"commander-deckbuilder/internal/scryfall"
	"commander-deckbuilder/internal/store"
)

type Service struct {
	store *store.Store
}

func New(s *store.Store) *Service { return &Service{store: s} }

// --- mazzi -----------------------------------------------------------------

func (s *Service) Decks() ([]deck.Deck, error) { return s.store.ListDecks() }

func (s *Service) CreateDeck(name string) (int64, error) { return s.store.CreateDeck(name) }

func (s *Service) DeleteDeck(id int64) error { return s.store.DeleteDeck(id) }

// Deck restituisce il mazzo con le sue carte; deck.ErrNotFound se non esiste.
func (s *Service) Deck(id int64) (deck.Deck, []deck.Card, error) {
	name, err := s.store.DeckName(id)
	if err != nil {
		return deck.Deck{}, nil, err
	}
	cards, err := s.store.DeckCards(id)
	return deck.Deck{ID: id, Name: name}, cards, err
}

func (s *Service) Cards(deckID int64) ([]deck.Card, error) { return s.store.DeckCards(deckID) }

// --- carte -----------------------------------------------------------------

// AddCard rilegge la carta da Scryfall invece di fidarsi del client: così
// prezzi e immagini non rimbalzano avanti e indietro negli attributi HTML.
func (s *Service) AddCard(ctx context.Context, deckID int64, scryfallID string) error {
	c, err := scryfall.Get(ctx, scryfallID)
	if err != nil {
		return err
	}
	return s.store.InsertCards(deckID, []deck.Card{toCard(c, 1, false)})
}

func (s *Service) SetPurchased(deckID int64, ids []int64, purchased bool) error {
	return s.store.SetPurchased(deckID, ids, purchased)
}

// DeleteCard restituisce il mazzo della carta: serve a ridisegnare la checklist giusta.
func (s *Service) DeleteCard(id int64) (int64, error) { return s.store.DeleteCard(id) }

// RefreshPrices riallinea i prezzi a Scryfall in un colpo solo. Il prezzo segue
// la stampa salvata, foil compreso: è quella che l'utente ha scelto.
func (s *Service) RefreshPrices(ctx context.Context, deckID int64) error {
	cards, err := s.store.DeckCards(deckID)
	if err != nil {
		return err
	}
	ids := make([]scryfall.Identifier, len(cards))
	for i, c := range cards {
		ids[i] = scryfall.Identifier{ID: c.ScryfallID}
	}
	fresh, err := scryfall.Collection(ctx, ids)
	if err != nil {
		return err
	}
	byID := make(map[string]scryfall.Card, len(fresh))
	for _, f := range fresh {
		byID[f.ID] = f
	}
	for _, c := range cards {
		if f, ok := byID[c.ScryfallID]; ok {
			if p := f.Price(c.Foil); p != c.PriceEUR {
				if err := s.store.SetPrice(c.ID, p); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// --- ricerca su Scryfall ---------------------------------------------------

// Quante carte mostrare: oltre non si scorre, si ricerca meglio.
const maxResults = 30

func (s *Service) Search(ctx context.Context, q string) ([]deck.Print, error) {
	found, err := scryfall.Search(ctx, q)
	return toPrints(found[:min(len(found), maxResults)]), err
}

// Prints elenca le ristampe di una carta, dalla più economica. oracleID
// identifica la carta a prescindere dalla stampa; manca solo sulle pochissime
// "reversible", per quelle ripiego sul nome esatto.
func (s *Service) Prints(ctx context.Context, oracleID, name string) ([]deck.Print, error) {
	q := "oracleid:" + oracleID
	if oracleID == "" {
		q = `!"` + strings.ReplaceAll(name, `"`, "") + `"`
	}
	found, err := scryfall.Prints(ctx, q)
	if err != nil {
		return nil, err
	}
	prints := toPrints(found)
	deck.SortByPrice(prints)
	return prints, nil
}

// --- Moxfield ----------------------------------------------------------------

// ImportResult: com'è andato un import. Problem, se c'è, è un errore da mostrare
// all'utente (lista vuota, Scryfall giù) e il mazzo non è stato creato.
type ImportResult struct {
	DeckID   int64
	Imported int      // carte entrate nel mazzo
	Lines    int      // righe riconosciute nella lista
	Lost     []string // righe non riconosciute o non trovate su Scryfall
	Problem  string
}

// Import crea un mazzo da una lista Moxfield. L'inserimento è transazionale:
// o entra tutto quello che Scryfall ha riconosciuto, o niente.
func (s *Service) Import(ctx context.Context, name, list string) (ImportResult, error) {
	lines, skipped := deck.ParseMoxfield(list)
	if len(lines) == 0 {
		return ImportResult{Problem: "nessuna riga riconosciuta"}, nil
	}
	ids := make([]scryfall.Identifier, len(lines))
	for i, l := range lines {
		ids[i] = identifier(l)
	}
	found, err := scryfall.Collection(ctx, ids)
	if err != nil {
		return ImportResult{Problem: err.Error()}, nil
	}
	rows, missing := matchPrintings(lines, found)
	if len(rows) == 0 {
		return ImportResult{Problem: "nessuna carta risolta"}, nil
	}
	id, err := s.store.CreateDeck(name)
	if err != nil {
		return ImportResult{}, err
	}
	if err := s.store.InsertCards(id, rows); err != nil {
		return ImportResult{}, err
	}
	// se qualcosa non è stato importato l'utente deve saperlo prima di andarsene
	lost := skipped
	for _, m := range missing {
		lost = append(lost, m.Name)
	}
	return ImportResult{DeckID: id, Imported: len(rows), Lines: len(lines), Lost: lost}, nil
}

// Export restituisce il nome del mazzo e la sua lista in formato Moxfield.
func (s *Service) Export(deckID int64) (name, list string, err error) {
	d, cards, err := s.Deck(deckID)
	if err != nil {
		return "", "", err
	}
	return d.Name, deck.ToMoxfield(cards), nil
}
