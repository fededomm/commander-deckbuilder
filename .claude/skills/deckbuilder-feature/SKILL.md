---
name: deckbuilder-feature
description: Come si aggiunge o modifica una funzionalità in Commander Deckbuilder (questo repo, Go + templ + htmx + echo + SQLite/Turso in Go puro, diviso per livelli in internal/ — web, service, deck, store, scryfall, ui). Usare SEMPRE quando si tocca una rotta, un handler, un servizio, una query dello store, un file .templ, lo stile, app.js, il client Scryfall o l'import/export Moxfield — anche per richieste vaghe tipo "aggiungi un bottone", "mostra anche X nella checklist", "fai in modo che…", "ordina per…", "salva anche…", "non funziona quando…". Contiene la mappa dei livelli e chi può importare chi, la catena rotta→handler→servizio→store→templ→htmx, le convenzioni del codice, i test e le trappole (templ generato, swap di #checklist, spunte out-of-band, HX-Redirect, rate limit Scryfall).
---

# Sviluppare una feature in Commander Deckbuilder

Un binario, niente JSON, niente framework JS. Le rotte rispondono con **frammenti
HTML** che htmx incolla nella pagina. Prima di scrivere, leggi i file che la
modifica tocca: sono piccoli, si leggono in un minuto.

## I livelli (compartimenti stagni)

Un pacchetto per livello in `internal/`. Le dipendenze vanno in un verso solo:

```
main.go ──► web ──► service ──► store ──► deck
             │         └──────► scryfall
             └──► ui ─────────────────► deck
```

| Livello | Pacchetto | Contiene | Non deve |
|---|---|---|---|
| avvio | `main.go` | flag, apertura store, `web.New(service.New(st), password)` | contenere logica |
| HTTP | `internal/web` | rotte (`server.go`), handler (`handlers_*.go`), login (`auth.go`), `/alive` (`lifecycle.go`) | fare SQL, chiamare Scryfall, calcolare totali |
| servizi | `internal/service` | casi d'uso (`service.go`), conversione Scryfall → modello (`convert.go`) | sapere di HTTP o HTML |
| business logic | `internal/deck` | modello (`Deck`, `Card`, `Print`, `ErrNotFound`), regole (`stats.go`), Moxfield (`moxfield.go`) | fare I/O di qualsiasi tipo |
| persistenza | `internal/store` | schema, `migrate`, metodi di `*Store` | restituire tipi che non siano di `deck` |
| servizio esterno | `internal/scryfall` | client resty, `scryfall.Card` | importare altri pacchetti del progetto |
| interfaccia | `internal/ui` | `*.templ`, `format.go` (eur, url, paginazione), `static/` (embed) | leggere o scrivere dati |

Se un import va contro la freccia, la cosa sta nel livello sbagliato. Controllo:
`go list -f '{{.ImportPath}}: {{join .Imports " "}}' ./internal/...`

Niente interfacce con una sola implementazione: `service` usa `*store.Store` e
le funzioni di `scryfall` direttamente. Un nuovo pacchetto solo se è un livello
nuovo, non per una feature.

## La mappa

| Serve… | Tocchi… |
|---|---|
| una rotta nuova | `web/server.go` (registrazione in `New`) + metodo su `*server` in `web/handlers_home.go` o `handlers_deck.go` |
| un'operazione nuova (import, aggiorna, calcola e salva…) | metodo di `*Service` in `service/service.go` |
| una regola nuova (categoria, statistica, formato) | `deck/stats.go` o `deck/moxfield.go`, funzione pura esportata |
| dati nuovi / query nuova | `store/store.go` (schema, `migrate`, metodo `(s *Store) Xxx`) |
| un campo nuovo da Scryfall | `references/scryfall.md`, sezione "Campi" |
| HTML nuovo | `ui/home.templ` / `ui/deck.templ` (+ `ui/layout.templ` solo per `<head>`) |
| formattazione per la vista (euro, url, percentuali) | `ui/format.go` |
| CSS / comportamento client | `ui/static/style.css`, `ui/static/app.js` (embed, niente build) |

## Il ciclo

1. **Handler** (`web`): `pathID(c)` per l'`:id`, `c.FormValue`/`c.QueryParam` per
   l'input, **un** metodo di `s.svc`, poi `render(c, ui.Componente(...))`.
   Errori: `return err` per i 500 (finiscono nel log), `echo.NewHTTPError(400, "…")`
   per l'input sbagliato. Un id che non esiste arriva dallo store come
   `deck.ErrNotFound` e l'error handler in `server.go` lo fa diventare 404: non
   controllarlo nell'handler.
2. **Servizio** (`service`): mette insieme store, Scryfall e regole di `deck`.
   Gli errori da mostrare all'utente (non 500) tornano come dato, vedi
   `ImportResult.Problem`.
3. **Query** (`store`): SQL inline, niente ORM. Aggregati (conteggi, totali) in SQL,
   non in Go: vedi `ListDecks`. Colonna nuova → nello `schema` (DB nuovi) **e** in
   `migrate()` (DB esistenti): il DB dell'utente non si ricrea mai. `sql.ErrNoRows`
   non esce dallo store: passa da `notFound(err)`.
4. **Vista** (`ui`): il `.templ`, poi **`go generate ./...`** e committa anche il
   `*_templ.go`. Mai editare i `_templ.go` a mano. I componenti usati da `web` sono
   esportati (`ui.Checklist`, `ui.PurchaseStats`…); quelli interni no. Nei template
   il parametro del mazzo si chiama `d`: `deck` è il nome del pacchetto.
5. **Test**, nel pacchetto che verifichi:
   - `deck`: test a tabella sulle regole, niente setup;
   - `store`: DB vero in `t.TempDir()` (`open(t)`);
   - `service`: `matchPrintings` e conversioni, senza rete;
   - `web`: `newServer(t, password)` → server vero su DB vero; carte inserite
     nello store, Scryfall mai chiamato;
   - `ui`: formattazione e paginazione;
   - `scryfall`: struct costruite a mano, o `client` puntato a un `httptest.Server`.
6. `gofmt -l . && go vet ./... && go test ./...` — devono stare zitti.
7. Se la feature passa da Scryfall, **provala sull'app vera** (`go run . -idle-quit 0`
   e curl o browser): i test girano offline e non vedono il contratto con l'API.

## Convenzioni htmx (le cose che si rompono)

- **La checklist si rimpiazza intera.** `#checklist` ha `hx-target="this" hx-swap="outerHTML"`;
  ogni azione sul mazzo (aggiungi, rimuovi, prezzi) risponde con
  `s.renderChecklist(c, deckID)`. Un'azione nuova sul mazzo fa lo stesso: non inventare
  swap parziali di una riga, il conteggio e i totali in testa cambierebbero senza aggiornarsi.
  **Eccezione: le spunte** (`POST /deck/:id/purchased`). Rispondono solo con
  `ui.PurchaseStats` (contatori + statistiche, `hx-swap-oob`) e la casella ha `hx-swap="none"`
  e `hx-sync="#deck:queue all"`. Ridisegnare le righe a ogni spunta cancellava la spunta di
  una carta cliccata mentre la richiesta precedente era in volo. Il barrato della riga viene
  da `:has(.check:checked)`, non da una classe. Non tornare a `renderChecklist` lì.
- **La home si rimpiazza per `#list`**: `deleteDeck` risponde con `ui.DeckList(decks)`
  e il bottone ha `hx-target="#list"`.
- **Creare qualcosa che ha una pagina sua** → header `HX-Redirect` + `204 No Content`
  (vedi `createDeck`). htmx segue il redirect, un `302` no.
- **Risultati di ricerca**: `hx-trigger="input changed delay:300ms"`; query vuota →
  `c.NoContent(200)`, non un errore. Un `hit` fa `hx-post` con `hx-vals={ scryVals(id) }`
  e punta a `#checklist`. Il client manda **solo lo `scryfall_id`**: prezzi, immagini e
  il resto li rilegge il servizio da Scryfall, mai rimbalzati negli attributi HTML.
- **Conferme**: `hx-confirm="…"` sul bottone, niente modali.
- **Bottoni che chiamano la rete**: `hx-disabled-elt="this"` così non si clicca due volte.
- **JS**: `app.js` delega sul `document` perché htmx rimpiazza i nodi di continuo.
  Un listener attaccato a un elemento sparisce al primo swap.
- Le azioni su una carta (`/cards/:id/…`) si fanno restituire il `deck_id` dal
  servizio (`DeleteCard`): serve per ridisegnare la checklist giusta.

## Stile del codice

- Commenti e messaggi in **italiano**, brevi, che spiegano il *perché* (leggi quelli
  esistenti prima di scriverne). Niente commenti che ripetono il codice. Ogni
  pacchetto ha un commento `// Package x …` che dice cosa ci sta e cosa no.
- Funzioni corte, niente interfacce con una sola implementazione.
- Scorciatoie deliberate con un tetto noto → commento `// ponytail: …` che dice il
  limite e come alzarlo (es. `SetMaxOpenConns(1)` in `store.Open`).
- Ogni funzione con un `if` in più lascia un test dietro di sé; le one-liner no.
- Prezzi in EUR (Cardmarket via Scryfall); foil ripiega sul normale se non quotato
  (`scryfall.Card.Price`). Formattazione solo con `eur`/`price0` di `ui/format.go`.
  `eur` usa lo spazio **non separabile** prima di "€": non sostituirlo.
- CSS: variabili in `:root`, tema scuro, sezioni `/* --- nome --- */`. Nessun framework.

## Avvio in sviluppo

`go run . -idle-quit 0` (altrimenti esce 5 s dopo aver chiuso la scheda). Flag nel README.

## Quando qualcosa non torna

- Il template non cambia → non hai fatto `go generate ./...`.
- `go generate` fallisce → `go tool templ` è in `go.mod` (`tool (…)`), non serve installarlo.
- Totali sbagliati in home ma giusti nel mazzo → `store.ListDecks` calcola in SQL,
  `deck.Sum` in Go: devono seguire la stessa formula (`qty * price_eur`, `purchased`
  come 0/1). `TestStore` li confronta.
- Scryfall 404 su una ricerca = nessun risultato, non un errore. Il resto in
  `references/scryfall.md`.
- L'app esce mentre provi con curl → è `web.WatchIdle`: usa `-idle-quit 0`.
- `go run ./tools/symbols` scrive in `internal/ui/static/symbols/` e
  `internal/ui/symbols_gen.go` (package `ui`).
