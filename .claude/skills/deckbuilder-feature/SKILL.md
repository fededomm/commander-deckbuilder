---
name: deckbuilder-feature
description: Come si aggiunge o modifica una funzionalità in Commander Deckbuilder (questo repo, Go + templ + htmx + echo + SQLite in Go puro). Usare SEMPRE quando si tocca una rotta, un handler, una query in db.go, un file .templ, lo stile, app.js, il client Scryfall o l'import/export Moxfield — anche per richieste vaghe tipo "aggiungi un bottone", "mostra anche X nella checklist", "fai in modo che…", "ordina per…", "salva anche…", "non funziona quando…". Contiene la catena rotta→handler→db→templ→htmx, le convenzioni del codice, i test e le trappole (templ generato, swap di #checklist, HX-Redirect, rate limit Scryfall).
---

# Sviluppare una feature in Commander Deckbuilder

App locale monoutente: un binario, niente JSON, niente framework JS. Le rotte
rispondono con **frammenti HTML** che htmx incolla nella pagina. Prima di scrivere,
leggi i file che la modifica tocca — sono piccoli, si leggono in un minuto.

## La mappa

| Serve… | Tocchi… |
|---|---|
| una rotta nuova | `routes.go` (registrazione) + `handlers_home.go` o `handlers_deck.go` |
| dati nuovi / query nuova | `db.go` (schema, `migrate`, funzione `xxx(db, …)`) |
| HTML nuovo | `home.templ` / `deck.templ` (+ `layout.templ` solo per `<head>`) |
| calcoli per la vista (totali, categorie, formattazione) | `view.go` |
| chiamate a Scryfall | `scryfall.go` — vedi `references/scryfall.md` |
| formato Moxfield | `moxfield.go` |
| CSS / comportamento client | `static/style.css`, `static/app.js` (embed, niente build) |
| un test | `app_test.go` (un file solo, tabelle e `TestDBEIRoutes`) |

Tutto è `package main`, tutto è nella root. Non creare sottocartelle o package
per una feature: non è quello che fa il resto del codice.

## Il ciclo

1. **Handler** in `handlers_*.go`: `pathID(c)` per l'`:id`, `c.FormValue`/`c.QueryParam`
   per l'input, una funzione di `db.go`, poi `render(c, componente(...))`.
   Errori: `return err` per i 500 (finiscono nel log), `echo.ErrNotFound` /
   `echo.NewHTTPError(400, "…")` per quelli dell'utente. `sql.ErrNoRows` → `echo.ErrNotFound`.
2. **Query** in `db.go`: funzioni piatte `nome(db *sql.DB, …)`, SQL inline, niente ORM.
   Aggregati (conteggi, totali) si fanno in SQL, non in Go — vedi `listDecks`.
   Colonna nuova → aggiungila sia allo `schema` (per i DB nuovi) sia alla lista di
   `migrate()` (per quelli esistenti): il DB dell'utente non si ricrea mai.
3. **Vista** nel `.templ`, poi **`go generate ./...`** e committa anche il `*_templ.go`.
   Non editare mai i `_templ.go` a mano. Helper per i template (`itoa`, `itoa64`,
   `eur`, `price0`) stanno in `view.go`: aggiungine lì se servono.
4. **Rotta** in `routes()`, vicino alle sue sorelle.
5. **Test** in `app_test.go`: logica pura → test a tabella; rotte/DB → estendi
   `TestDBEIRoutes` (DB in `t.TempDir()`, `httptest.NewServer(routes())`, helper
   `post`/`del`). Scryfall non si chiama nei test: le funzioni che lo usano
   prendono dati già risolti (`insertCards` riceve `[]Card`, non id).
6. `gofmt -l . && go vet . && go test .` — devono stare zitti.

## Convenzioni htmx (le cose che si rompono)

- **La checklist si rimpiazza intera.** `#checklist` ha `hx-target="this" hx-swap="outerHTML"`;
  ogni azione sul mazzo (aggiungi, toggle, rimuovi, prezzi) risponde con
  `renderChecklist(c, deckID)`. Un'azione nuova sul mazzo fa lo stesso: non inventare
  swap parziali di una riga, il conteggio e i totali in testa cambierebbero senza aggiornarsi.
- **La home si rimpiazza per `#list`**: `deleteDeckHandler` risponde con `deckList(decks)`
  e il bottone ha `hx-target="#list"`.
- **Creare qualcosa che ha una pagina sua** → header `HX-Redirect` + `204 No Content`
  (vedi `createDeckHandler`). htmx segue il redirect, un `302` no.
- **Risultati di ricerca**: `hx-trigger="input changed delay:300ms"`; query vuota →
  `c.NoContent(200)`, non un errore. Un `hit` fa `hx-post` con `hx-vals={ scryVals(id) }`
  e punta a `#checklist`. Il client manda **solo lo `scryfall_id`**: prezzi, immagini e
  il resto si rileggono dal server, mai rimbalzati negli attributi HTML.
- **Conferme**: `hx-confirm="…"` sul bottone, niente modali.
- **Bottoni che chiamano la rete**: `hx-disabled-elt="this"` così non si clicca due volte.
- **JS**: `app.js` delega sul `document` perché htmx rimpiazza i nodi di continuo.
  Un listener attaccato a un elemento sparisce al primo swap.
- Le query su una carta (`/cards/:id/…`) leggono prima il `deck_id` e lo restituiscono
  (`togglePurchased`, `deleteCard`): serve per rirenderizzare la checklist giusta.

## Stile del codice

- Commenti e messaggi in **italiano**, brevi, che spiegano il *perché* (leggi quelli
  esistenti prima di scriverne). Niente commenti che ripetono il codice.
- Funzioni corte, niente interfacce con una sola implementazione, niente package.
- Scorciatoie deliberate con un tetto noto → commento `// ponytail: …` che dice il
  limite e come alzarlo (es. `SetMaxOpenConns(1)` in `openDB`).
- Ogni funzione con un `if` in più lascia un test dietro di sé; le one-liner no.
- Prezzi in EUR (Cardmarket via Scryfall); foil ripiega sul normale se non quotato
  (`scryCard.price`). Formattazione solo con `eur`/`price0` di `view.go`.
- CSS: variabili in `:root`, tema scuro, sezioni `/* --- nome --- */`. Nessun framework.

## Avvio in sviluppo

`go run . -idle-quit 0` (altrimenti esce 5 s dopo aver chiuso la scheda). Flag nel README.

## Quando qualcosa non torna

- Il template non cambia → non hai fatto `go generate ./...`.
- `go generate` fallisce → `go tool templ` è in `go.mod` (`tool (…)`), non serve installarlo.
- Totali sbagliati in home ma giusti nel mazzo → `listDecks` calcola in SQL, `totals()`
  in Go: devono seguire la stessa formula (`qty * price_eur`, `purchased` come 0/1).
- Scryfall 404 su una ricerca = nessun risultato, non un errore. Il resto in
  `references/scryfall.md`.
- L'app esce mentre provi con curl → è `watchIdle`: usa `-idle-quit 0`.
