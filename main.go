// Commander Deckbuilder: un binario solo. Serve l'interfaccia su localhost,
// apre il browser e tiene tutto in uno SQLite nella cartella dati dell'utente.
package main

import (
	"database/sql"
	"embed"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/a-h/templ"
)

//go:generate go tool templ generate

//go:embed static
var staticFS embed.FS

var db *sql.DB

func main() {
	noBrowser := flag.Bool("no-browser", false, "non aprire il browser all'avvio")
	dbPath := flag.String("db", defaultDBPath(), "percorso del file SQLite")
	port := flag.Int("port", 8090, "porta HTTP (0 = una libera qualsiasi)")
	flag.Parse()

	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o755); err != nil {
		log.Fatalf("cartella dati: %v", err)
	}
	var err error
	if db, err = openDB(*dbPath); err != nil {
		log.Fatalf("database %s: %v", *dbPath, err)
	}
	defer db.Close()

	ln, err := listen(*port)
	if err != nil {
		log.Fatalf("porta: %v", err)
	}
	url := fmt.Sprintf("http://localhost:%d", ln.Addr().(*net.TCPAddr).Port)
	log.Printf("%s — db: %s", url, *dbPath)
	if !*noBrowser {
		openBrowser(url)
	}
	log.Fatal(http.Serve(ln, routes()))
}

// listen prova la porta chiesta; se è occupata ne prende una libera invece di
// morire — l'utente ha fatto doppio clic due volte, non è un errore da segnalare.
func listen(port int) (net.Listener, error) {
	ln, err := net.Listen("tcp", "localhost:"+strconv.Itoa(port))
	if err == nil || port == 0 {
		return ln, err
	}
	log.Printf("porta %d occupata, ne uso una libera", port)
	return net.Listen("tcp", "localhost:0")
}

// defaultDBPath: se c'è un data.db nella cartella corrente uso quello (sviluppo,
// o l'utente che ha copiato il DB accanto al binario), altrimenti la cartella
// dati dell'utente — la directory d'installazione è di sola lettura.
func defaultDBPath() string {
	if _, err := os.Stat("data.db"); err == nil {
		return "data.db"
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "data.db"
	}
	return filepath.Join(dir, "commander-deckbuilder", "data.db")
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("apri tu il browser su %s (%v)", url, err)
	}
}

func routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.FileServerFS(staticFS))

	mux.HandleFunc("GET /{$}", home)
	mux.HandleFunc("POST /decks", createDeckHandler)
	mux.HandleFunc("DELETE /decks/{id}", deleteDeckHandler)
	mux.HandleFunc("POST /import", importHandler)

	mux.HandleFunc("GET /deck/{id}", deckPageHandler)
	mux.HandleFunc("GET /deck/{id}/search", searchHandler)
	mux.HandleFunc("POST /deck/{id}/cards", addCardHandler)
	mux.HandleFunc("POST /deck/{id}/prices", refreshPricesHandler)
	mux.HandleFunc("GET /deck/{id}/export", exportHandler)

	mux.HandleFunc("POST /cards/{id}/toggle", toggleHandler)
	mux.HandleFunc("DELETE /cards/{id}", deleteCardHandler)
	return mux
}

/* ---------- handler ---------- */

func home(w http.ResponseWriter, r *http.Request) {
	decks, err := listDecks(db)
	if fail(w, err) {
		return
	}
	render(w, r, homePage(decks))
}

func createDeckHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "nome obbligatorio", http.StatusBadRequest)
		return
	}
	id, err := createDeck(db, name)
	if fail(w, err) {
		return
	}
	w.Header().Set("HX-Redirect", "/deck/"+strconv.FormatInt(id, 10))
	w.WriteHeader(http.StatusNoContent)
}

func deleteDeckHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if fail(w, deleteDeck(db, id)) { // le carte seguono via ON DELETE CASCADE
		return
	}
	decks, err := listDecks(db)
	if fail(w, err) {
		return
	}
	render(w, r, deckList(decks))
}

func deckPageHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	name, err := deckName(db, id)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if fail(w, err) {
		return
	}
	cards, err := deckCards(db, id)
	if fail(w, err) {
		return
	}
	render(w, r, deckPage(Deck{ID: id, Name: name}, cards))
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		return // campo svuotato: risultati vuoti, non un errore
	}
	cards, err := scryfallSearch(q)
	render(w, r, searchResults(id, cards, err))
}

func addCardHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	// Il client manda solo l'id: la carta la rileggo da Scryfall, così non ho
	// prezzi e immagini che rimbalzano avanti e indietro negli attributi HTML.
	card, err := scryfallCard(r.FormValue("scryfall_id"))
	if fail(w, err) {
		return
	}
	if fail(w, insertCards(db, id, []Card{card.toCard(1, false)})) {
		return
	}
	renderChecklist(w, r, id)
}

func toggleHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	deckID, err := togglePurchased(db, id)
	if fail(w, err) {
		return
	}
	renderChecklist(w, r, deckID)
}

func deleteCardHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	deckID, err := deleteCard(db, id)
	if fail(w, err) {
		return
	}
	renderChecklist(w, r, deckID)
}

// refreshPricesHandler riallinea i prezzi a Scryfall in un colpo solo.
func refreshPricesHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	cards, err := deckCards(db, id)
	if fail(w, err) {
		return
	}
	ids := make([]identifier, len(cards))
	for i, c := range cards {
		ids[i] = identifier{ID: c.ScryfallID}
	}
	fresh, err := scryfallCollection(ids)
	if fail(w, err) {
		return
	}
	prices := make(map[string]float64, len(fresh))
	for _, c := range fresh {
		prices[c.ID] = c.price(false)
	}
	for _, c := range cards {
		if p, ok := prices[c.ScryfallID]; ok && p != c.PriceEUR {
			if fail(w, setPrice(db, c.ID, p)) {
				return
			}
		}
	}
	renderChecklist(w, r, id)
}

func exportHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	name, err := deckName(db, id)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if fail(w, err) {
		return
	}
	cards, err := deckCards(db, id)
	if fail(w, err) {
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeFilename(name)+`.txt"`)
	fmt.Fprint(w, toMoxfield(cards))
}

func importHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	lines, skipped := parseMoxfield(r.FormValue("list"))
	if len(lines) == 0 {
		render(w, r, importError("nessuna riga riconosciuta"))
		return
	}
	rows, missing, err := resolvePrintings(lines)
	if err != nil {
		render(w, r, importError(err.Error()))
		return
	}
	if len(rows) == 0 {
		render(w, r, importError("nessuna carta risolta"))
		return
	}
	id, err := createDeck(db, name)
	if fail(w, err) {
		return
	}
	if fail(w, insertCards(db, id, rows)) {
		return
	}
	// se qualcosa non è stato importato l'utente deve saperlo prima di andarsene
	lost := skipped
	for _, m := range missing {
		lost = append(lost, m.Name)
	}
	if len(lost) == 0 {
		w.Header().Set("HX-Redirect", "/deck/"+strconv.FormatInt(id, 10))
	}
	render(w, r, importResult(id, len(rows), len(lines), lost))
}

/* ---------- utilità ---------- */

func render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := c.Render(r.Context(), w); err != nil {
		log.Printf("render: %v", err)
	}
}

func renderChecklist(w http.ResponseWriter, r *http.Request, deckID int64) {
	name, err := deckName(db, deckID)
	if fail(w, err) {
		return
	}
	cards, err := deckCards(db, deckID)
	if fail(w, err) {
		return
	}
	render(w, r, checklist(Deck{ID: deckID, Name: name}, cards))
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "id non valido", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

// fail logga e risponde 500. Torna true se c'era un errore, per uscire dall'handler.
func fail(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	log.Printf("errore: %v", err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
	return true
}

// safeFilename: il nome del mazzo finisce in un header HTTP, niente virgolette o a capo.
func safeFilename(name string) string {
	clean := strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '_' ||
			(r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return r
		}
		return -1
	}, name)
	if clean = strings.TrimSpace(clean); clean == "" {
		return "mazzo"
	}
	return clean
}
