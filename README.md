# Commander Deckbuilder

Ricerca carte via [Scryfall](https://scryfall.com/docs/api), checklist acquisti e prezzi Cardmarket (EUR).
Un binario solo: Go + [templ](https://templ.guide) + htmx, SQLite in Go puro. Niente Node, niente Electron.

```sh
go run .               # avvia e apre il browser su http://localhost:8090
go test ./...          # categorie, prezzi, totali, Moxfield, DB e rotte
go generate ./...      # rigenera i *_templ.go dopo aver toccato un .templ
go run ./tools/symbols # riscarica gli SVG dei simboli (solo se Scryfall ne aggiunge)
```

Flag: `-port` (0 = una libera qualsiasi), `-db` (percorso del file SQLite), `-no-browser`.

- `main.go` — server HTTP, rotte, apertura del browser
- `db.go` — schema SQLite e query
- `scryfall.go` — ricerca, batch `/cards/collection`, prezzi e immagini
- `moxfield.go` — parser ed export del formato Moxfield
- `view.go` — categorie, totali, formato euro, simboli di mana
- `*.templ` — le pagine; i `*_templ.go` accanto sono generati, non si modificano
- `static/` — CSS, htmx, SVG dei simboli: finiscono **dentro** il binario con `go:embed`

## Dove sta il database

`-db` se lo passi; altrimenti `./data.db` se esiste nella cartella corrente
(sviluppo, o il DB copiato accanto al binario); altrimenti la cartella dati
dell'utente:

| | |
|---|---|
| Linux | `~/.config/commander-deckbuilder/data.db` |
| Windows | `%APPDATA%\commander-deckbuilder\data.db` |
| macOS | `~/Library/Application Support/commander-deckbuilder/data.db` |

Le colonne mancanti vengono aggiunte all'avvio: i DB creati dalla vecchia
versione Node si aprono così come sono. Per ripartire da zero, cancella il file.

Tabelle: `decks` (nome) e `cards` (`deck_id` con `ON DELETE CASCADE`,
unique su `(deck_id, scryfall_id)`, più `qty`, `set_code`, `collector_number`, `foil`).

## Distribuzione

Niente installer e niente firma: un file eseguibile per piattaforma, `CGO_ENABLED=0`
grazie a `modernc.org/sqlite` (SQLite tradotto in Go, non un binding C).

```sh
CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -ldflags="-s -w" -o dist/deckbuilder
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o dist/deckbuilder.exe
CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -ldflags="-s -w" -o dist/deckbuilder-mac
```

Tutte e tre si compilano da Linux/WSL, ~12 MB l'una. L'utente fa doppio clic e
il browser si apre da solo; se la 8090 è occupata ne prende un'altra.

Su macOS Gatekeeper blocca un binario non firmato scaricato dal web: l'utente fa
clic destro → Apri una volta sola, oppure `xattr -dr com.apple.quarantine ./deckbuilder-mac`.
Per evitarglielo servono un Apple Developer ID e la notarizzazione — e per quella
serve un Mac, `codesign` e `notarytool` non esistono altrove.

## Import/export Moxfield

Formato `1 Sol Ring (EOC) 57 *F*`: "Importa da Moxfield" in home, "↓ esporta" nella
pagina mazzo. Il round-trip conserva quantità, stampa e foil. Le righe che Scryfall
non riconosce vengono elencate invece di sparire.

## Rotte

Restituiscono frammenti HTML per htmx, non JSON.

| | |
|---|---|
| `GET /` | home: mazzi, conteggi e totali (calcolati in SQL) |
| `POST /decks` | crea (`name`), risponde con `HX-Redirect` |
| `DELETE /decks/:id` | elimina mazzo e carte, rirenderizza la lista |
| `POST /import` | lista Moxfield → nuovo mazzo |
| `GET /deck/:id` | pagina builder |
| `GET /deck/:id/search?q=` | risultati Scryfall |
| `POST /deck/:id/cards` | aggiunge (`scryfall_id`); doppione ignorato |
| `POST /deck/:id/prices` | riallinea i prezzi a Scryfall |
| `GET /deck/:id/export` | scarica in formato Moxfield |
| `POST /cards/:id/toggle` | inverte "acquistata" |
| `DELETE /cards/:id` | rimuove la carta |
