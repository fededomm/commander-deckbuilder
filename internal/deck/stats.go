package deck

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
)

// Ordine di priorità: il primo tipo che matcha vince (Land prima di Creature
// così "Land Creature — Forest Dryad" finisce tra le terre).
var categories = []string{"Land", "Creature", "Planeswalker", "Battle", "Instant", "Sorcery", "Artifact", "Enchantment"}

func Categorize(typeLine string) string {
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

// GroupByCategory raggruppa mantenendo l'ordine di categories, con "Altro" in coda.
func GroupByCategory(cards []Card) []Group {
	byCat := map[string][]Card{}
	for _, c := range cards {
		cat := Categorize(c.TypeLine)
		byCat[cat] = append(byCat[cat], c)
	}
	var out []Group
	for _, cat := range append(append([]string{}, categories...), "Altro") {
		g, ok := byCat[cat]
		if !ok {
			continue
		}
		out = append(out, Group{Name: cat, Cards: g, Qty: Qty(g), Total: Sum(g).Total})
	}
	return out
}

func Qty(cards []Card) int {
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

// Sum calcola i totali in Go. La home li calcola in SQL (store.ListDecks):
// le due formule devono restare la stessa, qty * price_eur.
func Sum(cards []Card) Totals {
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

func PurchasedQty(cards []Card) int {
	n := 0
	for _, c := range cards {
		if c.Purchased {
			n += max(c.Qty, 1)
		}
	}
	return n
}

// --- curva di mana e colori ----------------------------------------------

// manaTokens estrae i simboli di un costo: "{2}{U}" -> ["2","U"]. Il testo fuori
// dalle graffe non è un simbolo e viene ignorato.
func manaTokens(cost string) []string {
	var out []string
	for {
		open := strings.IndexByte(cost, '{')
		if open < 0 {
			return out
		}
		shut := strings.IndexByte(cost[open:], '}')
		if shut < 0 {
			return out
		}
		shut += open
		out = append(out, cost[open+1:shut])
		cost = cost[shut+1:]
	}
}

// ManaValue è il costo convertito: i generici sommano il loro numero, ogni altro
// simbolo vale 1, {X} vale 0 — le stesse regole di Scryfall. Negli ibridi conta
// la metà più cara, quindi "{2/U}" vale 2 e "{W/U}" vale 1.
func ManaValue(cost string) int {
	v := 0
	for _, t := range manaTokens(cost) {
		half, _, _ := strings.Cut(t, "/")
		if n, err := strconv.Atoi(half); err == nil {
			v += n
			continue
		}
		if half == "X" || half == "Y" || half == "Z" {
			continue
		}
		v++
	}
	return v
}

// ManaColors ritorna i colori presenti in un costo, sempre nell'ordine WUBRG.
func ManaColors(cost string) string {
	toks := manaTokens(cost)
	var out strings.Builder
	for _, c := range "WUBRG" {
		for _, t := range toks {
			if strings.ContainsRune(t, c) {
				out.WriteRune(c)
				break
			}
		}
	}
	return out.String()
}

// DeckColors sono i colori che compaiono nei costi di mana del mazzo. Non è la
// color identity di Scryfall, che guarda anche il testo delle carte: per i
// pallini in elenco la differenza non si vede, per una regola di formato sì.
// I costi sono autodelimitati dalle graffe, quindi concatenarli è lecito.
func DeckColors(cards []Card) string {
	var all strings.Builder
	for _, c := range cards {
		all.WriteString(c.ManaCost)
	}
	return ManaColors(all.String())
}

var colorNames = map[rune]string{'W': "Bianco", 'U': "Blu", 'B': "Nero", 'R': "Rosso", 'G': "Verde"}

// ColorName serve alle etichette: "WU" -> "Bianco, Blu".
func ColorName(code string) string {
	var names []string
	for _, r := range code {
		if n, ok := colorNames[r]; ok {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return "Incolore"
	}
	return strings.Join(names, ", ")
}

type ColorStat struct {
	Code string // "W", "U", …, "C" per incolore
	Name string // "Bianco", "Blu", …
	Qty  int
}

// ColorStats conta le carte per colore; una multicolore conta in ogni suo colore,
// quindi la somma può superare il numero di carte. Le carte senza simboli colorati
// (Sol Ring, i Signet) vanno in "C", incolore. Le terre restano fuori come nella
// curva: non hanno costo, e contarle tutte come incolori non direbbe niente.
func ColorStats(cards []Card) []ColorStat {
	counts := map[rune]int{}
	for _, c := range cards {
		if Categorize(c.TypeLine) == "Land" {
			continue
		}
		colors := ManaColors(c.ManaCost)
		if colors == "" {
			colors = "C"
		}
		for _, r := range colors {
			counts[r] += max(c.Qty, 1)
		}
	}
	var out []ColorStat
	for _, r := range "WUBRGC" {
		if counts[r] > 0 {
			out = append(out, ColorStat{Code: string(r), Name: ColorName(string(r)), Qty: counts[r]})
		}
	}
	return out
}

type CurveBar struct {
	Label string // "0", "1", … "7+"
	Qty   int
	Pct   int // altezza relativa alla colonna più alta, 0-100
}

const curveTop = 7 // tutto da 7 in su finisce nell'ultima colonna

// ManaCurve esclude le terre: non hanno costo e schiaccerebbero la curva sullo 0.
// Nil se non c'è niente da disegnare, così il template salta il blocco.
func ManaCurve(cards []Card) []CurveBar {
	counts := make([]int, curveTop+1)
	for _, c := range cards {
		if Categorize(c.TypeLine) == "Land" {
			continue
		}
		counts[min(ManaValue(c.ManaCost), curveTop)] += max(c.Qty, 1)
	}
	peak := 0
	for _, n := range counts {
		peak = max(peak, n)
	}
	if peak == 0 {
		return nil
	}
	out := make([]CurveBar, 0, len(counts))
	for i, n := range counts {
		label := strconv.Itoa(i)
		if i == curveTop {
			label += "+"
		}
		out = append(out, CurveBar{Label: label, Qty: n, Pct: n * 100 / peak})
	}
	return out
}

// SortByPrice: le quotate prima, dalla più economica; le altre in coda nell'ordine
// in cui sono arrivate. È una lista della spesa: "order=eur" di Scryfall mette in
// testa le non quotate, cioè proprio quelle che non si comprano. Stabile, così
// il secondo criterio (dalla più recente) tiene.
func SortByPrice(prints []Print) {
	unpriced := func(p float64) int {
		if p == 0 {
			return 1
		}
		return 0
	}
	slices.SortStableFunc(prints, func(a, b Print) int {
		if c := cmp.Compare(unpriced(a.PriceEUR), unpriced(b.PriceEUR)); c != 0 {
			return c
		}
		return cmp.Compare(a.PriceEUR, b.PriceEUR)
	})
}
