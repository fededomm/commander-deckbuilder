package scryfall

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPriceEImmagine(t *testing.T) {
	p := func(eur, foil string) Card {
		var c Card
		if eur != "" {
			c.Prices.EUR = &eur
		}
		if foil != "" {
			c.Prices.EURFoil = &foil
		}
		return c
	}
	for _, tc := range []struct {
		card Card
		foil bool
		want float64
	}{
		{p("3.50", ""), false, 3.5},
		{p("", ""), false, 0}, // carta non quotata
		{p("1", "9"), true, 9},
		{p("1", ""), true, 1}, // niente eur_foil: ripiega sul normale
		{p("1", "9"), false, 1},
	} {
		if got := tc.card.Price(tc.foil); got != tc.want {
			t.Errorf("Price(foil=%v) = %v, voglio %v", tc.foil, got, tc.want)
		}
	}

	front := Card{ImageURIs: map[string]string{"normal": "a"}}
	if front.Image() != "a" {
		t.Error("Image() non legge image_uris")
	}
	var back Card
	back.CardFaces = append(back.CardFaces, struct {
		ImageURIs map[string]string `json:"image_uris"`
	}{ImageURIs: map[string]string{"normal": "b"}})
	if back.Image() != "b" { // doppia faccia
		t.Error("Image() non legge card_faces")
	}
	if (Card{}).Image() != "" {
		t.Error("Image() dovrebbe essere vuota")
	}
}

// Scryfall identifica l'app da qui: nome, versione e un recapito raggiungibile.
// Con un URL finto il ban è a loro discrezione.
func TestUserAgent(t *testing.T) {
	if !strings.HasPrefix(UserAgent, "commander-deckbuilder/") {
		t.Errorf("User-Agent senza nome app: %q", UserAgent)
	}
	if !strings.Contains(UserAgent, "(+https://github.com/fededomm/commander-deckbuilder)") {
		t.Errorf("User-Agent senza recapito: %q", UserAgent)
	}
	if buildVersion() == "" {
		t.Error("la versione non deve essere vuota")
	}
}

// Il contratto con /cards/collection: POST con {"identifiers": [...]} in minuscolo,
// campi vuoti omessi. Un refactor l'aveva rotto senza che nessun altro test se ne accorgesse.
func TestCollectionRequest(t *testing.T) {
	var got map[string][]map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/cards/collection" {
			t.Errorf("richiesta = %s %s", r.Method, r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"x","name":"Sol Ring"}]}`))
	}))
	defer srv.Close()
	client.SetBaseURL(srv.URL)
	defer client.SetBaseURL("https://api.scryfall.com")

	cards, err := Collection(t.Context(), []Identifier{{Set: "eoc", CollectorNumber: "57"}, {Name: "Sol Ring"}})
	if err != nil || len(cards) != 1 || cards[0].Name != "Sol Ring" {
		t.Fatalf("Collection = %+v, %v", cards, err)
	}
	ids := got["identifiers"]
	if len(ids) != 2 || ids[0]["set"] != "eoc" || ids[0]["collector_number"] != "57" || ids[1]["name"] != "Sol Ring" {
		t.Errorf("corpo della richiesta = %+v", got)
	}
	if _, ok := ids[1]["set"]; ok {
		t.Error("i campi vuoti devono sparire dal JSON")
	}
}
