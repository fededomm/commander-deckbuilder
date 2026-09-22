package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
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
		if got := categorize(line); got != want {
			t.Errorf("categorize(%q) = %q, voglio %q", line, got, want)
		}
	}
}

func TestPriceEImmagine(t *testing.T) {
	p := func(eur, foil string) scryCard {
		var c scryCard
		if eur != "" {
			c.Prices.EUR = &eur
		}
		if foil != "" {
			c.Prices.EURFoil = &foil
		}
		return c
	}
	for _, tc := range []struct {
		card scryCard
		foil bool
		want float64
	}{
		{p("3.50", ""), false, 3.5},
		{p("", ""), false, 0}, // carta non quotata
		{p("1", "9"), true, 9},
		{p("1", ""), true, 1}, // niente eur_foil: ripiega sul normale
		{p("1", "9"), false, 1},
	} {
		if got := tc.card.price(tc.foil); got != tc.want {
			t.Errorf("price(foil=%v) = %v, voglio %v", tc.foil, got, tc.want)
		}
	}

	front := scryCard{ImageURIs: map[string]string{"normal": "a"}}
	if front.image() != "a" {
		t.Error("image() non legge image_uris")
	}
	var back scryCard
	back.CardFaces = append(back.CardFaces, struct {
		ImageURIs map[string]string `json:"image_uris"`
	}{ImageURIs: map[string]string{"normal": "b"}})
	if back.image() != "b" { // doppia faccia
		t.Error("image() non legge card_faces")
	}
	if (scryCard{}).image() != "" {
		t.Error("image() dovrebbe essere vuota")
	}
}

func TestTotals(t *testing.T) {
	got := totals([]Card{
		{PriceEUR: 10, Qty: 1, Purchased: true},
		{PriceEUR: 2.5, Qty: 1},
		{PriceEUR: 0, Qty: 1}, // senza quotazione
	})
	if got != (Totals{Total: 12.5, Todo: 2.5, Unpriced: 1}) {
		t.Errorf("totals = %+v", got)
	}
	if got := totals(nil); got != (Totals{}) {
		t.Errorf("totals(nil) = %+v", got)
	}
	// il totale moltiplica per la quantità
	if got := totals([]Card{{PriceEUR: 0.1, Qty: 5}}); got != (Totals{Total: 0.5, Todo: 0.5}) {
		t.Errorf("totals(qty 5) = %+v", got)
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
	cards, skipped := parseMoxfield(lista)
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
	parsed, _ := parseMoxfield(lista)
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
	out := toMoxfield(cards)

	var want []string
	for _, l := range strings.Split(lista, "\n") {
		if l != "" && !strings.HasPrefix(l, "//") && l != "Deck" {
			want = append(want, l)
		}
	}
	if out != strings.Join(want, "\n")+"\n" {
		t.Errorf("export:\n%s\nvoglio:\n%s", out, strings.Join(want, "\n"))
	}
	again, _ := parseMoxfield(out)
	for i := range parsed {
		if again[i] != parsed[i] {
			t.Errorf("round-trip riga %d: %+v != %+v", i, again[i], parsed[i])
		}
	}
}

// La stampa scritta nella lista fa fede: se Scryfall non ha quell'espansione la
// riga è mancante, non diventa un'altra ristampa della stessa carta.
func TestMatchPrintings(t *testing.T) {
	found := []scryCard{
		{ID: "1", Name: "Sol Ring", Set: "eoc", CollectorNumber: "57"},
		{ID: "2", Name: "Sol Ring", Set: "c21", CollectorNumber: "263"},
		{ID: "3", Name: "Kefka, Court Mage // Kefka, Ruler of Ruin", Set: "fin", CollectorNumber: "97"},
	}
	lines := []MoxfieldLine{
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

func TestSortByPrice(t *testing.T) {
	priced := func(set, p string) scryCard {
		c := scryCard{Set: set}
		c.Prices.EUR = &p
		return c
	}
	cards := []scryCard{
		{Set: "sld"}, // non quotata, arrivata per prima
		priced("c21", "1.50"),
		{Set: "slz"}, // non quotata
		priced("eoc", "0.80"),
	}
	sortByPrice(cards)
	var got []string
	for _, c := range cards {
		got = append(got, c.Set)
	}
	// quotate dalla più economica, le altre in coda nell'ordine d'arrivo
	if want := []string{"eoc", "c21", "sld", "slz"}; !slices.Equal(got, want) {
		t.Errorf("ordine = %v, voglio %v", got, want)
	}
}

func TestPaginate(t *testing.T) {
	cards := make([]scryCard, 14) // 14 stampe = 2 pagine da 12
	for i := range cards {
		cards[i].Set = itoa(i)
	}
	p := paginate(cards, 2)
	if p.Count != 2 || p.Total != 14 || len(p.Items) != 2 {
		t.Fatalf("pagina 2 = %d/%d, %d elementi", p.Num, p.Count, len(p.Items))
	}
	if p.Items[0].Set != "12" {
		t.Errorf("la pagina 2 comincia da %q", p.Items[0].Set)
	}
	// fuori range non è un errore: mi riporta dentro
	if paginate(cards, 0).Num != 1 || paginate(cards, 99).Num != 2 {
		t.Error("le pagine fuori range devono rientrare")
	}
	if e := paginate(nil, 1); e.Count != 1 || len(e.Items) != 0 {
		t.Errorf("elenco vuoto = %+v", e)
	}
}

func TestPageURL(t *testing.T) {
	q := url.Values{"oracle": {"abc"}, "page": {"3"}}
	if got := pageURL(q, 5, 4); got != "/deck/5/prints?oracle=abc&page=4" {
		t.Errorf("pageURL = %q", got)
	}
	if len(q["page"]) != 1 || q["page"][0] != "3" {
		t.Error("pageURL non deve toccare la query di partenza")
	}
}

func TestPrintsURL(t *testing.T) {
	oracle := scryCard{OracleID: "1e4a7ae3", Name: "Sol Ring"}
	if got := printsURL(7, oracle); got != "/deck/7/prints?oracle=1e4a7ae3" {
		t.Errorf("printsURL = %q", got)
	}
	// senza oracle_id (carte "reversible") ripiega sul nome, con l'escape giusto
	if got := printsURL(7, scryCard{Name: "Bind // Liberate"}); got != "/deck/7/prints?name=Bind+%2F%2F+Liberate" {
		t.Errorf("printsURL senza oracle = %q", got)
	}
	if got := printYear("2024-08-02"); got != "2024" {
		t.Errorf("printYear = %q", got)
	}
	if printYear("") != "" {
		t.Error("printYear su data vuota non deve esplodere")
	}
}

// Scryfall identifica l'app da qui: nome, versione e un recapito raggiungibile.
// Con un URL finto il ban è a loro discrezione.
func TestUserAgent(t *testing.T) {
	if !strings.HasPrefix(userAgent, "commander-deckbuilder/") {
		t.Errorf("User-Agent senza nome app: %q", userAgent)
	}
	if !strings.Contains(userAgent, "(+https://github.com/fededomm/commander-deckbuilder)") {
		t.Errorf("User-Agent senza recapito: %q", userAgent)
	}
	if buildVersion() == "" {
		t.Error("la versione non deve essere vuota")
	}
}

func TestEur(t *testing.T) {
	for v, want := range map[float64]string{
		0: "0,00\u00a0€", 2.5: "2,50\u00a0€", 1234.5: "1.234,50\u00a0€",
		1234567: "1.234.567,00\u00a0€", -3: "-3,00\u00a0€",
	} {
		if got := eur(v); got != want {
			t.Errorf("eur(%v) = %q, voglio %q", v, got, want)
		}
	}
	if price0(0) != "—" {
		t.Error("le carte non quotate devono mostrare —")
	}
}

func TestManaParts(t *testing.T) {
	got := manaParts("{2}{U}{R}")
	if len(got) != 3 || got[0].SVG == "" || got[1].SVG == "" {
		t.Fatalf("manaParts = %+v", got)
	}
	// simbolo che Scryfall ha aggiunto dopo l'ultimo giro di symbols.mjs: resta testo
	unknown := manaParts("{NUOVO}")
	if len(unknown) != 1 || unknown[0].SVG != "" || unknown[0].Text != "{NUOVO}" {
		t.Errorf("simbolo ignoto = %+v", unknown)
	}
	if len(manaParts("")) != 0 {
		t.Error("costo vuoto deve dare zero pezzi")
	}
}

func TestSafeFilename(t *testing.T) {
	// il nome finisce in un header HTTP: niente virgolette, a capo o accenti
	if got := safeFilename(`Mazzo "Inalla"` + "\r\nX-Evil: 1"); strings.ContainsAny(got, "\"\r\n") {
		t.Errorf("safeFilename = %q", got)
	}
	if got := safeFilename("♥♦♣♠"); got != "mazzo" {
		t.Errorf("nome tutto simboli = %q, voglio il fallback", got)
	}
}

// Giro completo su un DB vero: crea, aggiunge, spunta, cancella.
func TestDBEIRoutes(t *testing.T) {
	var err error
	if db, err = openDB(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	srv := httptest.NewServer(routes())
	defer srv.Close()

	decks, _ := listDecks(db)
	if len(decks) != 0 {
		t.Fatal("il DB nuovo deve essere vuoto")
	}

	id, err := createDeck(db, "Inalla")
	if err != nil {
		t.Fatal(err)
	}
	card := Card{ScryfallID: "abc", Name: "Sol Ring", TypeLine: "Artifact", PriceEUR: 1.55, Qty: 1}
	if err := insertCards(db, id, []Card{card}); err != nil {
		t.Fatal(err)
	}
	if err := insertCards(db, id, []Card{card}); err != nil { // doppione: ignorato, non un errore
		t.Fatal(err)
	}
	if err := insertCards(db, id, []Card{{Name: "senza id"}}); err == nil {
		t.Error("una carta senza scryfall_id deve fallire")
	}
	if err := insertCards(db, id, []Card{
		{ScryfallID: "isl", Name: "Island", TypeLine: "Basic Land — Island", PriceEUR: 0.10, Qty: 5},
		{ScryfallID: "cmd", Name: "Command Tower", TypeLine: "Land", PriceEUR: 2.00, Qty: 1, Purchased: true},
	}); err != nil {
		t.Fatal(err)
	}

	res, err := http.Get(srv.URL + "/deck/" + itoa64(id) + "/prints")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest { // senza oracle né nome non si va su Scryfall
		t.Errorf("GET /prints senza parametri -> %d", res.StatusCode)
	}

	cards, _ := deckCards(db, id)
	if len(cards) != 3 {
		t.Fatalf("%d carte, ne voglio 3", len(cards))
	}
	decks, _ = listDecks(db)
	d := decks[0]
	if d.Cards != 7 { // 1 + 5 + 1
		t.Errorf("carte = %d, voglio 7", d.Cards)
	}
	if round2(d.Total) != 4.05 { // 1.55 + 5*0.10 + 2.00
		t.Errorf("totale = %v, voglio 4.05", d.Total)
	}
	if round2(d.Todo) != 2.05 { // solo le non acquistate
		t.Errorf("da comprare = %v, voglio 2.05", d.Todo)
	}

	// spunta e rimuovi passando dagli handler, così provo anche le rotte
	sol := cards[2]
	if sol.Name != "Sol Ring" {
		t.Fatalf("ordinamento inatteso: %q", sol.Name)
	}
	post(t, srv.URL+"/cards/"+itoa64(sol.ID)+"/toggle")
	cards, _ = deckCards(db, id)
	if !cards[2].Purchased {
		t.Error("toggle non ha spuntato la carta")
	}
	post(t, srv.URL+"/cards/"+itoa64(sol.ID)+"/toggle")
	cards, _ = deckCards(db, id)
	if cards[2].Purchased {
		t.Error("il secondo toggle deve togliere la spunta")
	}

	// export in formato Moxfield
	res, err = http.Get(srv.URL + "/deck/" + itoa64(id) + "/export")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(body), "1 Sol Ring") {
		t.Errorf("export = %q", body)
	}

	// cancellare il mazzo porta via anche le carte (ON DELETE CASCADE)
	del(t, srv.URL+"/decks/"+itoa64(id))
	if cards, _ := deckCards(db, id); len(cards) != 0 {
		t.Errorf("%d carte sopravvissute alla cancellazione del mazzo", len(cards))
	}
}

func round2(v float64) float64 {
	var out float64
	b, _ := json.Marshal(v)
	json.Unmarshal(b, &out)
	return float64(int(out*100+0.5)) / 100
}

func post(t *testing.T, url string) {
	t.Helper()
	res, err := http.Post(url, "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode >= 400 {
		t.Fatalf("POST %s -> %d", url, res.StatusCode)
	}
}

func del(t *testing.T, url string) {
	t.Helper()
	req, _ := http.NewRequest("DELETE", url, nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode >= 400 {
		t.Fatalf("DELETE %s -> %d", url, res.StatusCode)
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
		if got := manaValue(c.cost); got != c.want {
			t.Errorf("manaValue(%q) = %d, voglio %d", c.cost, got, c.want)
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
		if got := manaColors(c.cost); got != c.want {
			t.Errorf("manaColors(%q) = %q, voglio %q", c.cost, got, c.want)
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
	bars := manaCurve(cards)
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
	if manaCurve(cards[:1]) != nil {
		t.Error("solo terre: voglio nil, così il template salta il grafico")
	}
}

func TestColorStatsEDeckColors(t *testing.T) {
	cards := []Card{
		{ManaCost: "{1}{U}", Qty: 2},
		{ManaCost: "{W}{U}", Qty: 1}, // multicolore: conta sia in W sia in U
		{ManaCost: "{2}", Qty: 5},    // incolore: non conta da nessuna parte
	}
	stats := colorStats(cards)
	if len(stats) != 2 {
		t.Fatalf("voglio 2 colori, ho %+v", stats)
	}
	if stats[0].Code != "W" || stats[0].Qty != 1 {
		t.Errorf("primo colore = %+v, voglio W/1", stats[0])
	}
	if stats[1].Code != "U" || stats[1].Qty != 3 {
		t.Errorf("secondo colore = %+v, voglio U/3", stats[1])
	}
	if got := deckColors(cards); got != "WU" {
		t.Errorf("deckColors = %q, voglio \"WU\"", got)
	}
}

func TestPctNonDividePerZero(t *testing.T) {
	for _, c := range []struct{ part, total, want int }{
		{0, 0, 0}, {0, 10, 0}, {5, 10, 50}, {10, 10, 100}, {99, 100, 99},
		{11, 10, 100}, // qty incoerenti non devono sfondare la barra
	} {
		if got := pct(c.part, c.total); got != c.want {
			t.Errorf("pct(%d, %d) = %d, voglio %d", c.part, c.total, got, c.want)
		}
	}
}
