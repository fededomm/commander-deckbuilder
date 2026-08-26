# Commander Deckbuilder

Ricerca carte via [Scryfall](https://scryfall.com/docs/api), checklist acquisti e prezzi Cardmarket (EUR).
Zero dipendenze: `node:sqlite` + `node:http` della stdlib, richiede Node ≥ 22.5.

```sh
npm start              # app Electron (finestra desktop)
npm run dist:linux     # dist/*.AppImage
npm run dist:win       # dist/*.exe portable  (funziona anche da Linux/WSL)
npm run dist:mac       # dist/*.dmg + *.zip   -- SOLO da macOS, vedi sotto

node server.mjs        # solo browser: http://localhost:8090, crea data.db al primo avvio
node test.mjs          # check su categorie, prezzi, totali, Moxfield e API
node symbols.mjs       # riscarica gli SVG dei simboli di mana (solo se Scryfall ne aggiunge)
```

- `server.mjs` — schema SQLite + API REST + serve i file statici
- `public/index.html` — home: lista mazzi, creazione, totale per mazzo
- `public/deck.html?id=N` — builder: ricerca Scryfall + checklist acquisti
- `public/symbols/` + `symbols.js` — SVG dei simboli di mana, generati da `symbols.mjs`
- `main.js` — avvio Electron: fa partire `server.mjs` su porta libera e apre la finestra
- `data.db` — tutto qui dentro; per ripartire da zero basta cancellarlo

**Attenzione ai due database.** `node server.mjs` usa `./data.db`; l'app Electron usa
la cartella dati dell'utente (`~/.config/commander-deckbuilder/` su Linux,
`%APPDATA%\commander-deckbuilder\` su Windows), perché la directory di
installazione è di sola lettura. Sono separati: per travasare, copia il file.

Import/export in formato Moxfield (`1 Sol Ring (EOC) 57 *F*`): "Importa da Moxfield"
in home, "↓ esporta" nella pagina mazzo. Il round-trip conserva quantità, stampa e foil.

Tabelle: `decks` (nome) e `cards` (`deck_id` con `ON DELETE CASCADE`,
unique su `(deck_id, scryfall_id)`, più `qty`, `set_code`, `collector_number`, `foil`).
Le colonne mancanti vengono aggiunte all'avvio, i DB esistenti non vanno ricreati.

| | |
|---|---|
| `GET /api/decks` | mazzi con conteggi e totali (calcolati in SQL) |
| `POST /api/decks` | `{name}` |
| `GET /api/decks/:id` | mazzo + carte |
| `DELETE /api/decks/:id` | elimina mazzo e carte |
| `POST /api/decks/:id/cards` | aggiunge una carta o un array (import, in transazione); doppione ignorato |
| `PATCH /api/cards/:id` | `{purchased}` / `{price_eur}` |
| `DELETE /api/cards/:id` | |


## macOS

Il `.dmg` va costruito **su un Mac**: electron-builder usa `hdiutil` e `sips`, che
esistono solo lì. Da Linux esce il `.app` (target `zip`) ma non l'installer.

```sh
# su un Mac, nella cartella del progetto
npm ci && npm run dist:mac      # dist/*.dmg per arm64 e x64
```

Senza un certificato Apple Developer, electron-builder firma ad-hoc: l'app parte,
ma al primo avvio Gatekeeper la blocca. L'utente deve fare clic destro → Apri
(una volta sola), oppure:

```sh
xattr -dr com.apple.quarantine "/Applications/Commander Deckbuilder.app"
```

Per distribuirla senza questo passaggio servono un Apple Developer ID e la
notarizzazione (`APPLE_ID`, `APPLE_APP_SPECIFIC_PASSWORD`, `APPLE_TEAM_ID`).
