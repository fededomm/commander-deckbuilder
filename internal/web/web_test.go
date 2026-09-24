package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"commander-deckbuilder/internal/deck"
	"commander-deckbuilder/internal/service"
	"commander-deckbuilder/internal/store"
)

// newServer: server vero su un DB vero in una cartella temporanea. Scryfall non
// si chiama nei test: le carte si mettono direttamente nello store.
func newServer(t *testing.T, password string) (*httptest.Server, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(service.New(st), password))
	t.Cleanup(func() { srv.Close(); st.Close() })
	return srv, st
}

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }

func TestRoutes(t *testing.T) {
	srv, st := newServer(t, "")
	id, _ := st.CreateDeck("Inalla")
	st.InsertCards(id, []deck.Card{
		{ScryfallID: "abc", Name: "Sol Ring", TypeLine: "Artifact", PriceEUR: 1.55, Qty: 1},
		{ScryfallID: "isl", Name: "Island", TypeLine: "Basic Land — Island", PriceEUR: 0.10, Qty: 5},
	})
	cards, _ := st.DeckCards(id)
	sol := cards[1]

	get := func(path string) (int, string) {
		t.Helper()
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(body)
	}
	if code, _ := get("/deck/" + itoa64(id) + "/prints"); code != http.StatusBadRequest {
		t.Errorf("GET /prints senza oracle né nome -> %d, voglio 400", code) // non si va su Scryfall
	}
	if code, _ := get("/deck/999"); code != http.StatusNotFound {
		t.Errorf("mazzo inesistente -> %d, voglio 404", code)
	}
	if code, body := get("/deck/" + itoa64(id)); code != 200 || !strings.Contains(body, "Sol Ring") {
		t.Errorf("pagina mazzo -> %d", code)
	}

	// Spunte: lo stato voluto, non "inverti". Ripetere la stessa richiesta (doppio
	// clic, due richieste in volo) non deve ribaltarla.
	purchase := func(ids string, on bool) (int, string) {
		t.Helper()
		v := url.Values{"ids": {ids}}
		if on {
			v.Set("purchased", "1")
		}
		res, err := http.PostForm(srv.URL+"/deck/"+itoa64(id)+"/purchased", v)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(body)
	}
	_, body := purchase(itoa64(sol.ID), true)
	// solo contatori e statistiche, out-of-band: le righe non tornano
	if !strings.Contains(body, `id="deck-counts" class="deck-counts" hx-swap-oob="true"`) ||
		!strings.Contains(body, `id="deck-stats"`) || strings.Contains(body, `class="row"`) {
		t.Errorf("risposta alla spunta inattesa:\n%s", body)
	}
	purchase(itoa64(sol.ID), true)
	if cards, _ := st.DeckCards(id); !cards[1].Purchased {
		t.Error("due spunte uguali devono lasciare la carta acquistata")
	}
	purchase(itoa64(sol.ID), false)
	if cards, _ := st.DeckCards(id); cards[1].Purchased {
		t.Error("purchased vuoto deve togliere la spunta")
	}
	if code, _ := purchase("1,abc", true); code != 400 {
		t.Errorf("ids non validi: %d, voglio 400", code)
	}

	// export in formato Moxfield
	if _, body := get("/deck/" + itoa64(id) + "/export"); !strings.Contains(body, "1 Sol Ring") {
		t.Errorf("export = %q", body)
	}

	req, _ := http.NewRequest("DELETE", srv.URL+"/decks/"+itoa64(id), nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("DELETE mazzo: %v %v", err, res)
	}
	res.Body.Close()
	if decks, _ := st.ListDecks(); len(decks) != 0 {
		t.Errorf("il mazzo cancellato è ancora in elenco")
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

func TestAuthPassword(t *testing.T) {
	srv, _ := newServer(t, "segreta")
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	do := func(method, path string, body url.Values, hdr map[string]string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		res, err := noFollow.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res
	}

	if res := do("GET", "/deck/x", nil, nil); res.StatusCode != 303 || res.Header.Get("Location") != "/login" {
		t.Errorf("senza login: %d %q, voglio 303 verso /login", res.StatusCode, res.Header.Get("Location"))
	}
	if res := do("POST", "/deck/1/purchased", nil, map[string]string{"HX-Request": "true"}); res.StatusCode != 401 ||
		res.Header.Get("HX-Redirect") != "/login" {
		t.Errorf("htmx senza login: %d, voglio 401 con HX-Redirect", res.StatusCode)
	}
	for _, p := range []string{"/login", "/static/style.css"} {
		if res := do("GET", p, nil, nil); res.StatusCode != 200 {
			t.Errorf("%s deve essere pubblica: %d", p, res.StatusCode)
		}
	}
	if res := do("POST", "/login", url.Values{"password": {"sbagliata"}}, nil); len(res.Cookies()) != 0 {
		t.Error("password sbagliata: niente cookie")
	}
	res := do("POST", "/login", url.Values{"password": {"segreta"}}, nil)
	if res.StatusCode != 303 || len(res.Cookies()) != 1 {
		t.Fatalf("login: %d, %d cookie", res.StatusCode, len(res.Cookies()))
	}
	ck := res.Cookies()[0]
	// col cookie si passa: /deck/x arriva all'handler, che rifiuta l'id (400)
	if res := do("GET", "/deck/x", nil, map[string]string{"Cookie": ck.Name + "=" + ck.Value}); res.StatusCode != 400 {
		t.Errorf("col cookie: %d, voglio 400 dall'handler", res.StatusCode)
	}
	if res := do("GET", "/deck/x", nil, map[string]string{"Cookie": ck.Name + "=falso"}); res.StatusCode != 303 {
		t.Errorf("cookie falso: %d, voglio 303", res.StatusCode)
	}
}
