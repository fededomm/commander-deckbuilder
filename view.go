package main

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Ordine di priorità: il primo tipo che matcha vince (Land prima di Creature
// così "Land Creature — Forest Dryad" finisce tra le terre).
var categories = []string{"Land", "Creature", "Planeswalker", "Battle", "Instant", "Sorcery", "Artifact", "Enchantment"}

func categorize(typeLine string) string {
	main, _, _ := strings.Cut(typeLine, "—")
	for _, c := range categories {
		if strings.Contains(main, c) {
			return c
		}
	}
	return "Altro"
}

type Group struct {
	Name  string
	Cards []Card
	Qty   int
	Total float64
}

// groupByCategory raggruppa mantenendo l'ordine di categories, con "Altro" in coda.
func groupByCategory(cards []Card) []Group {
	byCat := map[string][]Card{}
	for _, c := range cards {
		cat := categorize(c.TypeLine)
		byCat[cat] = append(byCat[cat], c)
	}
	var out []Group
	for _, cat := range append(append([]string{}, categories...), "Altro") {
		g, ok := byCat[cat]
		if !ok {
			continue
		}
		t := totals(g)
		out = append(out, Group{Name: cat, Cards: g, Qty: qty(g), Total: t.Total})
	}
	return out
}

func qty(cards []Card) int {
	n := 0
	for _, c := range cards {
		n += max(c.Qty, 1)
	}
	return n
}

type Totals struct {
	Total    float64 // valore complessivo
	Todo     float64 // quanto resta da comprare
	Unpriced int     // carte senza quotazione Cardmarket
}

func totals(cards []Card) Totals {
	var t Totals
	for _, c := range cards {
		if c.PriceEUR == 0 {
			t.Unpriced++
			continue
		}
		line := c.PriceEUR * float64(max(c.Qty, 1))
		t.Total += line
		if !c.Purchased {
			t.Todo += line
		}
	}
	return t
}

func purchasedQty(cards []Card) int {
	n := 0
	for _, c := range cards {
		if c.Purchased {
			n += max(c.Qty, 1)
		}
	}
	return n
}

// eur formatta all'italiana: 1234.5 -> "1.234,50 €". Lo stdlib non ha i locale,
// e x/text per una riga di output non vale 10 MB di binario.
func eur(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	whole, dec, _ := strings.Cut(s, ".")
	sign := ""
	if strings.HasPrefix(whole, "-") {
		sign, whole = "-", whole[1:]
	}
	var b strings.Builder
	for i, c := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	return sign + b.String() + "," + dec + " €"
}

// price0 mostra "—" invece di "0,00 €" per le carte non quotate.
func price0(v float64) string {
	if v == 0 {
		return "—"
	}
	return eur(v)
}

// Stessa immagine in formato piccolo (~10 KB invece di ~100): con 100 carte
// in pagina è la differenza fra 1 MB e 10 MB. Se il path cambia resta il grande.
func thumbURL(u string) string {
	return strings.ReplaceAll(u, "/normal/", "/small/")
}

// ManaPart è un pezzo di costo di mana: o un SVG da symbols/, o testo così com'è
// (simbolo che Scryfall ha aggiunto dopo l'ultimo giro di symbols.mjs).
type ManaPart struct {
	SVG  string
	Text string
}

// manaParts spezza "{2}{U}{R}" nei simboli da disegnare, senza generare HTML:
// così il template resta al riparo da injection.
func manaParts(cost string) []ManaPart {
	var out []ManaPart
	for {
		open := strings.IndexByte(cost, '{')
		if open < 0 {
			break
		}
		close := strings.IndexByte(cost[open:], '}')
		if close < 0 {
			break
		}
		close += open
		if open > 0 {
			out = append(out, ManaPart{Text: cost[:open]})
		}
		sym := cost[open : close+1]
		if file, ok := symbols[sym]; ok {
			out = append(out, ManaPart{SVG: file, Text: sym})
		} else {
			out = append(out, ManaPart{Text: sym})
		}
		cost = cost[close+1:]
	}
	if cost != "" {
		out = append(out, ManaPart{Text: cost})
	}
	return out
}

func itoa(n int) string { return strconv.Itoa(n) }

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }

func joinLines(s []string) string { return strings.Join(s, ", ") }

// totalsLine è la riga sotto l'intestazione: vuota se il mazzo è vuoto.
func totalsLine(cards []Card) string {
	if len(cards) == 0 {
		return ""
	}
	t := totals(cards)
	s := "Totale " + eur(t.Total) + " · da comprare " + eur(t.Todo)
	if t.Unpriced > 0 {
		s += " · " + strconv.Itoa(t.Unpriced) + " senza quotazione"
	}
	return s
}

// scryVals: il pulsante manda solo l'id, la carta la rilegge il server da Scryfall.
// ponytail: una richiesta in più per non impacchettare mezza carta negli attributi.
func scryVals(id string) string {
	b, _ := json.Marshal(map[string]string{"scryfall_id": id})
	return string(b)
}
