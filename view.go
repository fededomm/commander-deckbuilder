package main

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/a-h/templ"
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

// --- Statistiche del mazzo: curva di mana, colori, avanzamento acquisti ---

// manaTokens estrae i simboli di un costo: "{2}{U}" -> ["2","U"]. Il testo fuori
// dalle graffe non è un simbolo e viene ignorato.
//
// ponytail: scansione a parte invece di riusare manaParts, che interleava i
// simboli col testo libero e costringerebbe a indovinare quale pezzo è cosa.
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

// manaValue è il costo convertito: i generici sommano il loro numero, ogni altro
// simbolo vale 1, {X} vale 0 — le stesse regole di Scryfall. Negli ibridi conta
// la metà più cara, quindi "{2/U}" vale 2 e "{W/U}" vale 1.
func manaValue(cost string) int {
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

// manaColors ritorna i colori presenti in un costo, sempre nell'ordine WUBRG.
func manaColors(cost string) string {
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

// deckColors sono i colori che compaiono nei costi di mana del mazzo. Non è la
// color identity di Scryfall, che guarda anche il testo delle carte: per i
// pallini in elenco la differenza non si vede, per una regola di formato sì.
// I costi sono autodelimitati dalle graffe, quindi concatenarli è lecito.
func deckColors(cards []Card) string {
	var all strings.Builder
	for _, c := range cards {
		all.WriteString(c.ManaCost)
	}
	return manaColors(all.String())
}

var colorNames = map[rune]string{'W': "Bianco", 'U': "Blu", 'B': "Nero", 'R': "Rosso", 'G': "Verde"}

// colorName serve alle etichette: "WU" -> "Bianco, Blu".
func colorName(code string) string {
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
	Code string // "W", "U", …
	Name string // "Bianco", "Blu", …
	Qty  int
}

// colorStats conta le carte per colore; una multicolore conta in ogni suo colore,
// quindi la somma può superare il numero di carte.
func colorStats(cards []Card) []ColorStat {
	counts := map[rune]int{}
	for _, c := range cards {
		for _, r := range manaColors(c.ManaCost) {
			counts[r] += max(c.Qty, 1)
		}
	}
	var out []ColorStat
	for _, r := range "WUBRG" {
		if counts[r] > 0 {
			out = append(out, ColorStat{Code: string(r), Name: colorNames[r], Qty: counts[r]})
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

// manaCurve esclude le terre: non hanno costo e schiaccerebbero la curva sullo 0.
// Nil se non c'è niente da disegnare, così il template salta il blocco.
func manaCurve(cards []Card) []CurveBar {
	counts := make([]int, curveTop+1)
	for _, c := range cards {
		if categorize(c.TypeLine) == "Land" {
			continue
		}
		counts[min(manaValue(c.ManaCost), curveTop)] += max(c.Qty, 1)
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
		label := itoa(i)
		if i == curveTop {
			label = itoa(curveTop) + "+"
		}
		out = append(out, CurveBar{Label: label, Qty: n, Pct: n * 100 / peak})
	}
	return out
}

// curveLabel: la descrizione testuale del grafico per chi usa uno screen reader.
func curveLabel(bars []CurveBar) string {
	parts := make([]string, 0, len(bars))
	for _, b := range bars {
		parts = append(parts, "costo "+b.Label+": "+itoa(b.Qty))
	}
	return "Curva di mana — " + strings.Join(parts, ", ")
}

// pct arrotonda per difetto e non divide per zero: serve alle barre di avanzamento.
func pct(part, total int) int {
	if total <= 0 {
		return 0
	}
	return min(part*100/total, 100)
}

// Le percentuali qui sotto nascono da interi calcolati dal server, mai da input
// dell'utente: SafeCSS è una constatazione, non una scorciatoia.
func barHeight(p int) templ.SafeCSS { return templ.SafeCSS("height:" + itoa(p) + "%") }

func barWidth(p int) templ.SafeCSS { return templ.SafeCSS("width:" + itoa(p) + "%") }

// --- Stampe (ristampe) --------------------------------------------------

// printYear: da "2024-08-02" a "2024". Scryfall manda sempre la data intera,
// ma se un giorno mancasse non voglio un panic per quattro caratteri.
func printYear(released string) string {
	if len(released) < 4 {
		return ""
	}
	return released[:4]
}

// printsURL è la rotta che elenca le ristampe di una carta. oracle_id identifica
// la carta al di là della stampa; sulle "reversible" manca e ripiego sul nome.
func printsURL(deckID int64, c scryCard) string {
	v := url.Values{}
	if c.OracleID != "" {
		v.Set("oracle", c.OracleID)
	} else {
		v.Set("name", c.Name)
	}
	return "/deck/" + itoa64(deckID) + "/prints?" + v.Encode()
}

// Page è una fetta di ristampe con il suo posto nella sequenza: la griglia
// della dialog ne mostra una per volta.
type Page struct {
	Items []scryCard
	Num   int // pagina corrente, da 1
	Count int // pagine totali
	Total int // ristampe in tutto
}

const perPage = 12 // 12 rientra nella dialog senza scroll fino a 3 colonne

// paginate taglia la pagina chiesta. Una pagina fuori range non è un errore:
// il link può essere vecchio, meglio la prima (o l'ultima) che una schermata rotta.
func paginate(cards []scryCard, num int) Page {
	p := Page{Total: len(cards), Num: num, Count: (len(cards) + perPage - 1) / perPage}
	if p.Count == 0 {
		return Page{Num: 1, Count: 1}
	}
	p.Num = min(max(num, 1), p.Count)
	start := (p.Num - 1) * perPage
	p.Items = cards[start:min(start+perPage, len(cards))]
	return p
}

// pageURL riusa la query della richiesta cambiando solo la pagina: così la
// dialog non deve sapere se la carta è stata trovata per oracle o per nome.
func pageURL(base url.Values, deckID int64, num int) string {
	v := url.Values{}
	for k, vals := range base {
		if k != "page" {
			v[k] = vals
		}
	}
	v.Set("page", itoa(num))
	return "/deck/" + itoa64(deckID) + "/prints?" + v.Encode()
}
