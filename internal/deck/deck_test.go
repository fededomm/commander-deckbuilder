package deck

import (
	"slices"
	"strings"
	"testing"
)

func TestCategorize(t *testing.T) {
	for line, want := range map[string]string{
		"Legendary Creature — Human Wizard": "Creature",
		"Artifact Creature — Golem":         "Creature",
		"Land Creature — Forest Dryad":      "Land", // Dryad Arbor
		"Artifact Land":                     "Land",
		"Instant — Arcane":                  "Instant",
		"Enchantment — Aura":                "Enchantment",
		"Creature — Elf Artificer":          "Creature", // "Artificer" != Artifact
		"Kindred Sorcery — Goblin":          "Sorcery",
		"":                                  "Altro",
	} {
		if got := Categorize(line); got != want {
			t.Errorf("Categorize(%q) = %q, voglio %q", line, got, want)
		}
	}
}

func TestSum(t *testing.T) {
	got := Sum([]Card{
		{PriceEUR: 10, Qty: 1, Purchased: true},
		{PriceEUR: 2.5, Qty: 1},
		{PriceEUR: 0, Qty: 1}, // senza quotazione
	})
	if got != (Totals{Total: 12.5, Todo: 2.5, Unpriced: 1}) {
		t.Errorf("Sum = %+v", got)
	}
	if got := Sum(nil); got != (Totals{}) {
		t.Errorf("Sum(nil) = %+v", got)
	}
	// il totale moltiplica per la quantità
	if got := Sum([]Card{{PriceEUR: 0.1, Qty: 5}}); got != (Totals{Total: 0.5, Todo: 0.5}) {
		t.Errorf("Sum(qty 5) = %+v", got)
	}
}

const lista = `1 Inalla, Archmage Ritualist (SLD) 1639
1 Archmage Emeritus (STX) 377 *F*
1 Curiosity (PLST) A25-52
1 Ral, Crackling Wit (PBLB) 230p
1 Kefka, Court Mage / Kefka, Ruler of Ruin (FIN) 231
5 Island (SNC) 264
1 Sol Ring

// commento
Deck`

func TestParseMoxfield(t *testing.T) {
	cards, skipped := ParseMoxfield(lista)
	if len(cards) != 7 {
		t.Fatalf("%d righe, ne voglio 7", len(cards))
	}
	// intestazioni segnalate, non ingoiate
	if len(skipped) != 1 || skipped[0] != "Deck" {
		t.Errorf("skipped = %v", skipped)
	}
	n := 0
	for _, c := range cards {
		n += c.Qty
	}
	if n != 11 { // 5 Island contano 5
		t.Errorf("quantità totale = %d, voglio 11", n)
	}

	want := MoxfieldLine{Qty: 1, Name: "Archmage Emeritus", SetCode: "stx", CollectorNumber: "377", Foil: true}
	if cards[1] != want {
		t.Errorf("cards[1] = %+v", cards[1])
	}
	if cards[2].CollectorNumber != "A25-52" { // numero col trattino (PLST)
		t.Errorf("cards[2].CollectorNumber = %q", cards[2].CollectorNumber)
	}
	if cards[3].CollectorNumber != "230p" { // numero con lettera
		t.Errorf("cards[3].CollectorNumber = %q", cards[3].CollectorNumber)
	}
	if cards[4].Name != "Kefka, Court Mage / Kefka, Ruler of Ruin" { // bifacciale
		t.Errorf("cards[4].Name = %q", cards[4].Name)
	}
	if (cards[6] != MoxfieldLine{Qty: 1, Name: "Sol Ring"}) { // senza stampa
		t.Errorf("cards[6] = %+v", cards[6])
	}
}

func TestMoxfieldRoundTrip(t *testing.T) {
	parsed, _ := ParseMoxfield(lista)
	cards := make([]Card, len(parsed))
	for i, l := range parsed {
		cards[i] = Card{
			Name:            strings.ReplaceAll(l.Name, " / ", " // "), // come lo tiene Scryfall
			Qty:             l.Qty,
			SetCode:         l.SetCode,
			CollectorNumber: l.CollectorNumber,
			Foil:            l.Foil,
		}
	}
	out := ToMoxfield(cards)

	var want []string
	for _, l := range strings.Split(lista, "\n") {
		if l != "" && !strings.HasPrefix(l, "//") && l != "Deck" {
			want = append(want, l)
		}
	}
	if out != strings.Join(want, "\n")+"\n" {
		t.Errorf("export:\n%s\nvoglio:\n%s", out, strings.Join(want, "\n"))
	}
	again, _ := ParseMoxfield(out)
	for i := range parsed {
		if again[i] != parsed[i] {
			t.Errorf("round-trip riga %d: %+v != %+v", i, again[i], parsed[i])
		}
	}
}

func TestSortByPrice(t *testing.T) {
	prints := []Print{
		{Set: "sld"}, // non quotata, arrivata per prima
		{Set: "c21", PriceEUR: 1.50},
		{Set: "slz"}, // non quotata
		{Set: "eoc", PriceEUR: 0.80},
	}
	SortByPrice(prints)
	var got []string
	for _, p := range prints {
		got = append(got, p.Set)
	}
	// quotate dalla più economica, le altre in coda nell'ordine d'arrivo
	if want := []string{"eoc", "c21", "sld", "slz"}; !slices.Equal(got, want) {
		t.Errorf("ordine = %v, voglio %v", got, want)
	}
}

func TestManaValue(t *testing.T) {
	for _, c := range []struct {
		cost string
		want int
	}{
		{"", 0},
		{"{0}", 0},
		{"{3}", 3},
		{"{2}{U}{R}", 4},
		{"{W}{W}", 2},
		{"{X}{R}", 1},     // X vale 0
		{"{2/U}{2/U}", 4}, // ibrido monocolore: conta la metà generica
		{"{W/U}{W/U}", 2}, // ibrido fra colori: vale 1 a simbolo
		{"{U/P}", 1},      // phyrexiano
		{"{10}{G}", 11},   // generico a due cifre
		{"{C}", 1},        // incolore
		{"{2}{U", 2},      // graffa non chiusa: mi fermo, non impazzisco
	} {
		if got := ManaValue(c.cost); got != c.want {
			t.Errorf("ManaValue(%q) = %d, voglio %d", c.cost, got, c.want)
		}
	}
}

func TestManaColors(t *testing.T) {
	for _, c := range []struct{ cost, want string }{
		{"", ""},
		{"{3}", ""},
		{"{R}{G}{W}", "WRG"}, // riordinato in WUBRG, non nell'ordine del costo
		{"{U}{U}{U}", "U"},   // niente doppioni
		{"{W/U}{B}", "WUB"},  // l'ibrido porta entrambi i colori
		{"{2/R}", "R"},
	} {
		if got := ManaColors(c.cost); got != c.want {
			t.Errorf("ManaColors(%q) = %q, voglio %q", c.cost, got, c.want)
		}
	}
}

func TestManaCurveEscludeLeTerre(t *testing.T) {
	cards := []Card{
		{TypeLine: "Basic Land — Island", ManaCost: "", Qty: 10},
		{TypeLine: "Creature — Human", ManaCost: "{1}{U}", Qty: 1},
		{TypeLine: "Creature — Human", ManaCost: "{1}{U}", Qty: 3}, // la qty conta
		{TypeLine: "Sorcery", ManaCost: "{9}{U}", Qty: 1},          // oltre il 7 finisce in coda
	}
	bars := ManaCurve(cards)
	if len(bars) != curveTop+1 {
		t.Fatalf("voglio %d colonne, ho %d", curveTop+1, len(bars))
	}
	if bars[0].Qty != 0 {
		t.Errorf("colonna 0 = %d, le terre non devono entrarci", bars[0].Qty)
	}
	if bars[2].Qty != 4 {
		t.Errorf("colonna 2 = %d, voglio 4", bars[2].Qty)
	}
	if bars[curveTop].Label != "7+" || bars[curveTop].Qty != 1 {
		t.Errorf("ultima colonna = %q/%d, voglio \"7+\"/1", bars[curveTop].Label, bars[curveTop].Qty)
	}
	if bars[2].Pct != 100 {
		t.Errorf("la colonna più alta deve stare al 100%%, sta al %d", bars[2].Pct)
	}
	// Un mazzo di sole terre non ha curva da disegnare.
	if ManaCurve(cards[:1]) != nil {
		t.Error("solo terre: voglio nil, così il template salta il grafico")
	}
}

func TestColorStatsEDeckColors(t *testing.T) {
	cards := []Card{
		{ManaCost: "{1}{U}", Qty: 2},
		{ManaCost: "{W}{U}", Qty: 1},               // multicolore: conta sia in W sia in U
		{ManaCost: "{2}", Qty: 5},                  // incolore: va in C
		{TypeLine: "Basic Land — Island", Qty: 30}, // terra: fuori, come nella curva
	}
	stats := ColorStats(cards)
	if len(stats) != 3 {
		t.Fatalf("voglio W, U e C, ho %+v", stats)
	}
	if stats[0].Code != "W" || stats[0].Qty != 1 {
		t.Errorf("primo colore = %+v, voglio W/1", stats[0])
	}
	if stats[1].Code != "U" || stats[1].Qty != 3 {
		t.Errorf("secondo colore = %+v, voglio U/3", stats[1])
	}
	if stats[2].Code != "C" || stats[2].Qty != 5 || stats[2].Name != "Incolore" {
		t.Errorf("incolore = %+v, voglio C/5", stats[2])
	}
	if got := DeckColors(cards); got != "WU" {
		t.Errorf("DeckColors = %q, voglio \"WU\"", got)
	}
}
