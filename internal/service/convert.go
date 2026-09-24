package service

import (
	"strings"

	"commander-deckbuilder/internal/deck"
	"commander-deckbuilder/internal/scryfall"
)

// Qui il formato di Scryfall diventa il modello dell'app. È l'unico punto che
// conosce entrambi: se Scryfall cambia un campo, si tocca solo questo file.

func toCard(c scryfall.Card, qty int, foil bool) deck.Card {
	return deck.Card{
		ScryfallID:      c.ID,
		Name:            c.Name,
		TypeLine:        c.TypeLine,
		ManaCost:        c.ManaCost,
		Image:           c.Image(),
		PriceEUR:        c.Price(foil),
		Qty:             qty,
		SetCode:         c.Set,
		CollectorNumber: c.CollectorNumber,
		Foil:            foil,
	}
}

func toPrints(cards []scryfall.Card) []deck.Print {
	out := make([]deck.Print, len(cards))
	for i, c := range cards {
		out[i] = deck.Print{
			ScryfallID:      c.ID,
			OracleID:        c.OracleID,
			Name:            c.Name,
			TypeLine:        c.TypeLine,
			Set:             c.Set,
			SetName:         c.SetName,
			ReleasedAt:      c.ReleasedAt,
			CollectorNumber: c.CollectorNumber,
			Image:           c.Image(),
			PriceEUR:        c.Price(false),
		}
	}
	return out
}

// identifier: se ho set e numero uso quelli (è la stampa esatta), altrimenti il nome.
func identifier(l deck.MoxfieldLine) scryfall.Identifier {
	if l.SetCode != "" {
		return scryfall.Identifier{Set: l.SetCode, CollectorNumber: l.CollectorNumber}
	}
	return scryfall.Identifier{Name: l.Name}
}

// matchPrintings accoppia le righe alle stampe che Scryfall ha restituito.
// La stampa scritta nella lista fa fede: una riga con "(EOC) 57" o prende
// quella stampa o finisce fra le mancanti. Ripiegare sul nome darebbe una
// ristampa a caso — con l'espansione sbagliata e il prezzo di un'altra carta.
// Il nome vale solo per le righe che la stampa non ce l'avevano proprio.
func matchPrintings(lines []deck.MoxfieldLine, found []scryfall.Card) (rows []deck.Card, missing []deck.MoxfieldLine) {
	bySet := map[string]scryfall.Card{}
	byName := map[string]scryfall.Card{}
	for _, c := range found {
		bySet[c.Set+"|"+c.CollectorNumber] = c
		if _, seen := byName[strings.ToLower(c.Name)]; !seen {
			byName[strings.ToLower(c.Name)] = c
		}
	}
	for _, l := range lines {
		var c scryfall.Card
		var ok bool
		if l.SetCode != "" {
			c, ok = bySet[l.SetCode+"|"+l.CollectorNumber]
		} else {
			// Moxfield scrive "A / B" le bifacciali, Scryfall "A // B"
			c, ok = byName[strings.ToLower(strings.ReplaceAll(l.Name, " / ", " // "))]
		}
		if !ok {
			missing = append(missing, l)
			continue
		}
		rows = append(rows, toCard(c, l.Qty, l.Foil))
	}
	return rows, missing
}
