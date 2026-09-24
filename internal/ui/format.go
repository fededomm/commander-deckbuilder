// Package ui è l'interfaccia: i template templ, i CSS/JS/icone in static/ e le
// funzioni che trasformano i dati in testo da mostrare (prezzi, percentuali,
// URL). Riceve i tipi di deck già calcolati; non legge né scrive niente.
package ui

import (
	"embed"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"commander-deckbuilder/internal/deck"
)

// Static sono CSS, JS, icone e simboli di mana, dentro il binario.
//
//go:embed static
var Static embed.FS

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
	return sign + b.String() + "," + dec + " €" // spazio non separabile: "€" non va a capo da solo
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
// (simbolo che Scryfall ha aggiunto dopo l'ultimo giro di tools/symbols).
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

func itoa(n int) string     { return strconv.Itoa(n) }
func itoa64(n int64) string { return strconv.FormatInt(n, 10) }

func joinLines(s []string) string { return strings.Join(s, ", ") }

// scryVals: il pulsante manda solo l'id, la carta la rilegge il server da Scryfall.
// ponytail: una richiesta in più per non impacchettare mezza carta negli attributi.
func scryVals(id string) string {
	b, _ := json.Marshal(map[string]string{"scryfall_id": id})
	return string(b)
}

// curveLabel: la descrizione testuale del grafico per chi usa uno screen reader.
func curveLabel(bars []deck.CurveBar) string {
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
func barWidth(p int) templ.SafeCSS  { return templ.SafeCSS("width:" + itoa(p) + "%") }

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
func printsURL(deckID int64, p deck.Print) string {
	v := url.Values{}
	if p.OracleID != "" {
		v.Set("oracle", p.OracleID)
	} else {
		v.Set("name", p.Name)
	}
	return "/deck/" + itoa64(deckID) + "/prints?" + v.Encode()
}

// Page è una fetta di ristampe con il suo posto nella sequenza: la griglia
// della dialog ne mostra una per volta.
type Page struct {
	Items []deck.Print
	Num   int // pagina corrente, da 1
	Count int // pagine totali
	Total int // ristampe in tutto
}

const perPage = 12 // 12 rientra nella dialog senza scroll fino a 3 colonne

// Paginate taglia la pagina chiesta. Una pagina fuori range non è un errore:
// il link può essere vecchio, meglio la prima (o l'ultima) che una schermata rotta.
func Paginate(prints []deck.Print, num int) Page {
	p := Page{Total: len(prints), Num: num, Count: (len(prints) + perPage - 1) / perPage}
	if p.Count == 0 {
		return Page{Num: 1, Count: 1}
	}
	p.Num = min(max(num, 1), p.Count)
	start := (p.Num - 1) * perPage
	p.Items = prints[start:min(start+perPage, len(prints))]
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
