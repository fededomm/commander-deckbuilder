package ui

import (
	"net/url"
	"testing"

	"commander-deckbuilder/internal/deck"
)

func TestPaginate(t *testing.T) {
	prints := make([]deck.Print, 14) // 14 stampe = 2 pagine da 12
	for i := range prints {
		prints[i].Set = itoa(i)
	}
	p := Paginate(prints, 2)
	if p.Count != 2 || p.Total != 14 || len(p.Items) != 2 {
		t.Fatalf("pagina 2 = %d/%d, %d elementi", p.Num, p.Count, len(p.Items))
	}
	if p.Items[0].Set != "12" {
		t.Errorf("la pagina 2 comincia da %q", p.Items[0].Set)
	}
	// fuori range non è un errore: mi riporta dentro
	if Paginate(prints, 0).Num != 1 || Paginate(prints, 99).Num != 2 {
		t.Error("le pagine fuori range devono rientrare")
	}
	if e := Paginate(nil, 1); e.Count != 1 || len(e.Items) != 0 {
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
	oracle := deck.Print{OracleID: "1e4a7ae3", Name: "Sol Ring"}
	if got := printsURL(7, oracle); got != "/deck/7/prints?oracle=1e4a7ae3" {
		t.Errorf("printsURL = %q", got)
	}
	// senza oracle_id (carte "reversible") ripiega sul nome, con l'escape giusto
	if got := printsURL(7, deck.Print{Name: "Bind // Liberate"}); got != "/deck/7/prints?name=Bind+%2F%2F+Liberate" {
		t.Errorf("printsURL senza oracle = %q", got)
	}
	if got := printYear("2024-08-02"); got != "2024" {
		t.Errorf("printYear = %q", got)
	}
	if printYear("") != "" {
		t.Error("printYear su data vuota non deve esplodere")
	}
}

func TestEur(t *testing.T) {
	for v, want := range map[float64]string{
		0: "0,00 €", 2.5: "2,50 €", 1234.5: "1.234,50 €",
		1234567: "1.234.567,00 €", -3: "-3,00 €",
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
	// simbolo che Scryfall ha aggiunto dopo l'ultimo giro di tools/symbols: resta testo
	unknown := manaParts("{NUOVO}")
	if len(unknown) != 1 || unknown[0].SVG != "" || unknown[0].Text != "{NUOVO}" {
		t.Errorf("simbolo ignoto = %+v", unknown)
	}
	if len(manaParts("")) != 0 {
		t.Error("costo vuoto deve dare zero pezzi")
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
