package service

import (
	"testing"

	"commander-deckbuilder/internal/deck"
	"commander-deckbuilder/internal/scryfall"
)

// La stampa scritta nella lista fa fede: se Scryfall non ha quell'espansione la
// riga è mancante, non diventa un'altra ristampa della stessa carta.
func TestMatchPrintings(t *testing.T) {
	found := []scryfall.Card{
		{ID: "1", Name: "Sol Ring", Set: "eoc", CollectorNumber: "57"},
		{ID: "2", Name: "Sol Ring", Set: "c21", CollectorNumber: "263"},
		{ID: "3", Name: "Kefka, Court Mage // Kefka, Ruler of Ruin", Set: "fin", CollectorNumber: "97"},
	}
	lines := []deck.MoxfieldLine{
		{Qty: 1, Name: "Sol Ring", SetCode: "c21", CollectorNumber: "263"}, // stampa esatta
		{Qty: 1, Name: "Sol Ring"}, // senza stampa: vale il nome
		{Qty: 1, Name: "Sol Ring", SetCode: "lea", CollectorNumber: "270"}, // stampa che non c'è
		{Qty: 1, Name: "Kefka, Court Mage / Kefka, Ruler of Ruin"},         // bifacciale alla Moxfield
	}
	rows, missing := matchPrintings(lines, found)

	if len(rows) != 3 {
		t.Fatalf("%d righe risolte, ne voglio 3", len(rows))
	}
	if rows[0].SetCode != "c21" || rows[0].CollectorNumber != "263" {
		t.Errorf("stampa esatta ignorata: %s %s", rows[0].SetCode, rows[0].CollectorNumber)
	}
	if rows[1].ScryfallID != "1" { // senza stampa prende la prima che Scryfall ha dato
		t.Errorf("fallback sul nome = %q", rows[1].ScryfallID)
	}
	if rows[2].ScryfallID != "3" {
		t.Errorf("bifacciale non riconosciuta: %q", rows[2].ScryfallID)
	}
	if len(missing) != 1 || missing[0].SetCode != "lea" {
		t.Errorf("la stampa inesistente deve finire fra le mancanti: %+v", missing)
	}
}
