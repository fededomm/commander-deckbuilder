package store

import (
	"errors"
	"math"
	"path/filepath"
	"testing"

	"commander-deckbuilder/internal/deck"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// Giro completo su un DB vero: crea, aggiunge, conta, spunta, cancella.
func TestStore(t *testing.T) {
	s := open(t)
	if decks, _ := s.ListDecks(); len(decks) != 0 {
		t.Fatal("il DB nuovo deve essere vuoto")
	}

	id, err := s.CreateDeck("Inalla")
	if err != nil {
		t.Fatal(err)
	}
	sol := deck.Card{ScryfallID: "abc", Name: "Sol Ring", TypeLine: "Artifact", PriceEUR: 1.55, Qty: 1}
	if err := s.InsertCards(id, []deck.Card{sol}); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertCards(id, []deck.Card{sol}); err != nil { // doppione: ignorato, non un errore
		t.Fatal(err)
	}
	if err := s.InsertCards(id, []deck.Card{{Name: "senza id"}}); err == nil {
		t.Error("una carta senza scryfall_id deve fallire")
	}
	if err := s.InsertCards(id, []deck.Card{
		{ScryfallID: "isl", Name: "Island", TypeLine: "Basic Land — Island", PriceEUR: 0.10, Qty: 5},
		{ScryfallID: "cmd", Name: "Command Tower", TypeLine: "Land", PriceEUR: 2.00, Qty: 1, Purchased: true},
	}); err != nil {
		t.Fatal(err)
	}

	cards, _ := s.DeckCards(id)
	if len(cards) != 3 || cards[2].Name != "Sol Ring" { // ordinate per nome
		t.Fatalf("carte = %+v", cards)
	}
	// i totali della home si calcolano in SQL: devono tornare con deck.Sum
	decks, _ := s.ListDecks()
	d := decks[0]
	if d.Cards != 7 { // 1 + 5 + 1
		t.Errorf("carte = %d, voglio 7", d.Cards)
	}
	if round2(d.Total) != 4.05 || round2(d.Total) != round2(deck.Sum(cards).Total) { // 1.55 + 5*0.10 + 2.00
		t.Errorf("totale = %v, voglio 4.05", d.Total)
	}
	if round2(d.Todo) != 2.05 { // solo le non acquistate
		t.Errorf("da comprare = %v, voglio 2.05", d.Todo)
	}

	// SetPurchased scrive lo stato, non lo inverte; un id di un altro mazzo non si tocca
	other, _ := s.CreateDeck("Altro")
	s.InsertCards(other, []deck.Card{{ScryfallID: "x", Name: "Estranea"}})
	stranger, _ := s.DeckCards(other)
	for range 2 {
		if err := s.SetPurchased(id, []int64{cards[2].ID, stranger[0].ID}, true); err != nil {
			t.Fatal(err)
		}
	}
	if cards, _ = s.DeckCards(id); !cards[2].Purchased {
		t.Error("due spunte uguali devono lasciare la carta acquistata")
	}
	if stranger, _ = s.DeckCards(other); stranger[0].Purchased {
		t.Error("una carta di un altro mazzo non deve cambiare")
	}

	// cancellare il mazzo porta via anche le carte (a mano: su Turso il CASCADE non c'è)
	if err := s.DeleteDeck(id); err != nil {
		t.Fatal(err)
	}
	if cards, _ := s.DeckCards(id); len(cards) != 0 {
		t.Errorf("%d carte sopravvissute alla cancellazione del mazzo", len(cards))
	}
	if _, err := s.DeckName(id); !errors.Is(err, deck.ErrNotFound) {
		t.Errorf("mazzo cancellato: %v, voglio deck.ErrNotFound", err)
	}
	if _, err := s.DeleteCard(999); !errors.Is(err, deck.ErrNotFound) {
		t.Errorf("carta inesistente: %v, voglio deck.ErrNotFound", err)
	}
}
