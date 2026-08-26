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
	"time"
)

//go:generate go tool templ generate

//go:embed static
var staticFS embed.FS

var db *sql.DB

func main() {
	noBrowser := flag.Bool("no-browser", false, "non aprire il browser all'avvio")
	dbPath := flag.String("db", defaultDBPath(), "percorso del file SQLite")
	port := flag.Int("port", 8090, "porta HTTP (0 = una libera qualsiasi)")
	idleQuit := flag.Duration("idle-quit", 5*time.Second,
		"esce dopo questo tempo senza finestre aperte (0 = resta acceso)")
	flag.Parse()

	dataDir := filepath.Dir(*dbPath)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatalf("cartella dati: %v", err)
	}
	logToFile(dataDir)
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
	if *idleQuit > 0 {
		go watchIdle(*idleQuit)
	}
	log.Fatal(http.Serve(ln, routes()))
}

// logToFile: l'installer Windows avvia il binario senza console (-H windowsgui)
// e il .app macOS senza terminale. Senza questo, un errore all'avvio — porta
// occupata, DB illeggibile — sparirebbe nel nulla.
func logToFile(dir string) {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "deckbuilder.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		log.SetOutput(f)
	}
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
