# Commander Deckbuilder

Ricerca carte via [Scryfall](https://scryfall.com/docs/api), checklist acquisti e prezzi Cardmarket (EUR).
Un binario solo: Go + [templ](https://templ.guide) + htmx, SQLite in Go puro,
[echo](https://echo.labstack.com) per le rotte e [resty](https://github.com/go-resty/resty)
per le chiamate a Scryfall. Niente Node, niente Electron.

```sh
go run .               # avvia e apre il browser su http://localhost:8090
go test ./...          # categorie, prezzi, totali, Moxfield, DB e rotte
go generate ./...      # rigenera i *_templ.go dopo aver toccato un .templ
go run ./tools/symbols # riscarica gli SVG dei simboli (solo se Scryfall ne aggiunge)
```

Flag: `-port` (0 = una libera qualsiasi), `-db` (percorso del file SQLite), `-no-browser`.

- `main.go` — avvio: flag, scelta del DB, porta, apertura del browser
- `routes.go` — le rotte echo, l'error handler e gli helper condivisi
- `handlers_home.go` — elenco mazzi, creazione, cancellazione, import Moxfield
- `handlers_deck.go` — pagina mazzo: ricerca, checklist, prezzi, export
- `db.go` — schema SQLite e query
- `scryfall.go` — client resty: ricerca, batch `/cards/collection`, prezzi e immagini
- `moxfield.go` — parser ed export del formato Moxfield
- `view.go` — categorie, totali, formato euro, simboli di mana
- `*.templ` — le pagine; i `*_templ.go` accanto sono generati, non si modificano
- `static/` — CSS, htmx, SVG dei simboli: finiscono **dentro** il binario con `go:embed`
- `packaging/` — icone, script NSIS e `build.sh` che produce i pacchetti
- `.github/workflows/release.yml` — build e release su tag

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

`CGO_ENABLED=0` grazie a `modernc.org/sqlite` (SQLite tradotto in Go, non un
binding C): tutti i target si compilano da Linux/WSL, ~12 MB l'uno.

### Release su GitHub

Un tag fa tutto da solo:

```sh
git tag v0.3.0 && git push origin v0.3.0
```

`.github/workflows/release.yml` gira i test, costruisce i pacchetti su due runner
(Ubuntu per Windows e Linux, macOS per il `.dmg`) e apre la release con i file
allegati. `workflow_dispatch` fa la stessa build senza pubblicare, per provare.

### In locale

```sh
sudo apt install nsis                    # una volta sola, per l'installer Windows
./packaging/build.sh                     # tutto
./packaging/build.sh windows linux       # solo alcuni target
VERSION=0.3.0 ./packaging/build.sh       # versione diversa da quella di default
```

Escono in `dist/`:

| | |
|---|---|
| `…-windows-setup.exe` | installer NSIS, ~4 MB |
| `…-macos-arm64.zip` / `-amd64.zip` | bundle `.app` pronto da trascinare in Applicazioni |
| `…-macos-*.dmg` | **solo su un Mac** (in CI ci pensa il runner macOS) |
| `…-linux-amd64.tar.gz` | il binario Linux |

Il binario semplice resta un `go build` normale, se ti basta quello.

### Windows

Installer **per utente singolo**: va in `%LOCALAPPDATA%\Programs`, non chiede
l'amministratore, mette la voce nel menu Start e in "App installate" con il suo
disinstallatore. L'eseguibile è compilato `-H windowsgui`, quindi niente finestra
nera del prompt; in cambio non ha stderr, e scrive `deckbuilder.log` accanto al
database. Disinstallando, i mazzi restano: li cancella solo se rispondi di sì
alla domanda esplicita.

### macOS

Il `.app` si costruisce da Linux (è solo una cartella con `Info.plist`, il
binario e l'icona), il **`.dmg` no**: vuole `hdiutil`, che esiste solo su macOS.
Lo script se ne accorge da solo e lo produce se lo lanci lì.

Senza un Apple Developer ID Gatekeeper blocca un `.app` non firmato scaricato dal
web: clic destro → Apri, una volta sola, oppure

```sh
xattr -dr com.apple.quarantine "/Applications/Commander Deckbuilder.app"
```

Per evitarlo servono firma e notarizzazione (`codesign`, `notarytool`) — e per
quelle serve un Mac.

### Fermare l'app

Installata non ha una console da chiudere: il server continua a girare anche
dopo aver chiuso la scheda del browser. In fondo alla home c'è
"⏻ chiudi l'applicazione", che è l'unico modo pulito di fermarlo.

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
