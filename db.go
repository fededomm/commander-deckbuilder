package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
	_ "modernc.org/sqlite"
)

type Deck struct {
	ID        int64
	Name      string
	Cards     int     // somma delle qty
	Purchased int     // somma delle qty già acquistate
	Total     float64 // valore del mazzo
	Todo      float64 // quanto resta da comprare
	Colors    string  // colori dei costi di mana, in ordine WUBRG, es. "WUB"
}

type Card struct {
	ID              int64
	DeckID          int64
	ScryfallID      string
	Name            string
	TypeLine        string
	ManaCost        string
	Image           string
	PriceEUR        float64
	Purchased       bool
	Qty             int
	SetCode         string
	CollectorNumber string
	Foil            bool
}

const schema = `
CREATE TABLE IF NOT EXISTS decks (
  id      INTEGER PRIMARY KEY,
  name    TEXT NOT NULL,
  created TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE IF NOT EXISTS cards (
  id          INTEGER PRIMARY KEY,
  deck_id     INTEGER NOT NULL REFERENCES decks(id) ON DELETE CASCADE,
  scryfall_id TEXT NOT NULL,
  name        TEXT NOT NULL,
  type_line   TEXT NOT NULL DEFAULT '',
  mana_cost   TEXT NOT NULL DEFAULT '',
  image       TEXT NOT NULL DEFAULT '',
  price_eur   REAL NOT NULL DEFAULT 0,
  purchased   INTEGER NOT NULL DEFAULT 0,
  qty         INTEGER NOT NULL DEFAULT 1,
  set_code    TEXT NOT NULL DEFAULT '',
  collector_number TEXT NOT NULL DEFAULT '',
  foil        INTEGER NOT NULL DEFAULT 0,
  UNIQUE (deck_id, scryfall_id)
);`

// openDB: un percorso è un file SQLite locale; un URL libsql:// è Turso (deploy
// su Render free, dove il disco del container sparisce a ogni spin-down).
// Il token di Turso arriva da TURSO_AUTH_TOKEN, mai dal flag: finirebbe nei log.
func openDB(path string) (*sql.DB, error) {
	driver, dsn := "sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	if strings.HasPrefix(path, "libsql://") {
		driver, dsn = "libsql", path+"?authToken="+os.Getenv("TURSO_AUTH_TOKEN")
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	// ponytail: una connessione sola. App locale monoutente, così niente SQLITE_BUSY
	// e niente pool da accordare. Se un giorno diventa multiutente, alza il limite.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}
	return db, migrate(db)
}

// DB creati prima dell'import Moxfield: aggiungo le colonne mancanti.
func migrate(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(cards)`)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		have[name] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, c := range [][2]string{
		{"qty", "INTEGER NOT NULL DEFAULT 1"},
		{"set_code", "TEXT NOT NULL DEFAULT ''"},
		{"collector_number", "TEXT NOT NULL DEFAULT ''"},
		{"foil", "INTEGER NOT NULL DEFAULT 0"},
	} {
		if !have[c[0]] {
			if _, err := db.Exec("ALTER TABLE cards ADD COLUMN " + c[0] + " " + c[1]); err != nil {
				return err
			}
		}
	}
	return nil
}

func listDecks(db *sql.DB) ([]Deck, error) {
	rows, err := db.Query(`
		SELECT d.id, d.name,
		       COALESCE(SUM(c.qty), 0),
		       COALESCE(SUM(c.qty * c.purchased), 0),
		       COALESCE(SUM(c.qty * c.price_eur), 0),
		       COALESCE(SUM(c.qty * c.price_eur * (1 - c.purchased)), 0),
		       COALESCE(GROUP_CONCAT(c.mana_cost, ''), '')
		FROM decks d LEFT JOIN cards c ON c.deck_id = d.id
		GROUP BY d.id ORDER BY d.created DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Deck
	for rows.Next() {
		var d Deck
		var costs string // i costi di tutte le carte, concatenati: li riduco a WUBRG
		if err := rows.Scan(&d.ID, &d.Name, &d.Cards, &d.Purchased, &d.Total, &d.Todo, &costs); err != nil {
			return nil, err
		}
		d.Colors = manaColors(costs)
		out = append(out, d)
	}
	return out, rows.Err()
}

func createDeck(db *sql.DB, name string) (int64, error) {
	res, err := db.Exec(`INSERT INTO decks (name) VALUES (?)`, name)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func deckName(db *sql.DB, id int64) (string, error) {
	var name string
	err := db.QueryRow(`SELECT name FROM decks WHERE id = ?`, id).Scan(&name)
	return name, err
}

func deckCards(db *sql.DB, id int64) ([]Card, error) {
	rows, err := db.Query(`
		SELECT id, deck_id, scryfall_id, name, type_line, mana_cost, image,
		       price_eur, purchased, qty, set_code, collector_number, foil
		FROM cards WHERE deck_id = ? ORDER BY name`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Card
	for rows.Next() {
		var c Card
		if err := rows.Scan(&c.ID, &c.DeckID, &c.ScryfallID, &c.Name, &c.TypeLine, &c.ManaCost,
			&c.Image, &c.PriceEUR, &c.Purchased, &c.Qty, &c.SetCode, &c.CollectorNumber, &c.Foil); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// insertCards è transazionale: l'import Moxfield o entra tutto o niente.
// Un doppione non è un errore, l'utente ha semplicemente ricliccato.
func insertCards(db *sql.DB, deckID int64, cards []Card) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO cards
		(deck_id, scryfall_id, name, type_line, mana_cost, image, price_eur, purchased, qty, set_code, collector_number, foil)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, c := range cards {
		if c.ScryfallID == "" || c.Name == "" {
			return fmt.Errorf("scryfall_id e name obbligatori")
		}
		if c.Qty < 1 {
			c.Qty = 1
		}
		if _, err := stmt.Exec(deckID, c.ScryfallID, c.Name, c.TypeLine, c.ManaCost, c.Image,
			c.PriceEUR, c.Purchased, c.Qty, strings.ToLower(c.SetCode), c.CollectorNumber, c.Foil); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Le carte le cancello a mano: su Turso via HTTP il PRAGMA foreign_keys non
// sopravvive tra una richiesta e l'altra, quindi il CASCADE non è garantito.
func deleteDeck(db *sql.DB, id int64) error {
	if _, err := db.Exec(`DELETE FROM cards WHERE deck_id = ?`, id); err != nil {
		return err
	}
	_, err := db.Exec(`DELETE FROM decks WHERE id = ?`, id)
	return err
}

func deleteCard(db *sql.DB, id int64) (int64, error) {
	var deckID int64
	if err := db.QueryRow(`SELECT deck_id FROM cards WHERE id = ?`, id).Scan(&deckID); err != nil {
		return 0, err
	}
	_, err := db.Exec(`DELETE FROM cards WHERE id = ?`, id)
	return deckID, err
}

// togglePurchased inverte il flag lato server: niente stato dal client, niente race
// se l'utente clicca due volte in fretta.
// setPurchased scrive lo stato voluto, non "inverti": due clic ravvicinati o una
// richiesta ripetuta non possono ribaltare la spunta. deck_id nel WHERE: un id
// che appartiene a un altro mazzo non viene toccato.
func setPurchased(db *sql.DB, deckID int64, ids []int64, purchased bool) error {
	if len(ids) == 0 {
		return nil
	}
	args := []any{purchased, deckID}
	for _, id := range ids {
		args = append(args, id)
	}
	_, err := db.Exec(`UPDATE cards SET purchased = ? WHERE deck_id = ? AND id IN (?`+
		strings.Repeat(",?", len(ids)-1)+`)`, args...)
	return err
}

func setPrice(db *sql.DB, id int64, price float64) error {
	_, err := db.Exec(`UPDATE cards SET price_eur = ? WHERE id = ?`, price, id)
	return err
}
