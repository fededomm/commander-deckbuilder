# Scryfall in questo progetto

Client resty in `scryfall.go`, base `https://api.scryfall.com`. Tre funzioni,
non aggiungerne una quarta se una di queste basta:

| Funzione | Endpoint | Note |
|---|---|---|
| `scryfallSearch(ctx, q)` | `GET /cards/search?unique=cards&q=` | sintassi Scryfall (`t:creature c:g`); 404 = zero risultati → `nil, nil`; tronca a 30 |
| `scryfallCard(ctx, id)` | `GET /cards/{id}` | 404 → `errNotFound` |
| `scryfallCollection(ctx, ids)` | `POST /cards/collection` | max **75 identifier** a chiamata, spezza e dorme 100 ms fra i blocchi |

`identifier` accetta `id`, oppure `set`+`collector_number`, oppure `name`:
i campi vuoti spariscono dal JSON (`omitempty`). È così che l'import Moxfield
risolve `1 Sol Ring (EOC) 57` in una stampa precisa (`moxfield.go: resolvePrintings`).

## Regole di Scryfall che il codice rispetta

- `User-Agent` identificabile e `Accept: application/json`: senza, rifiuta (anche i POST).
- ~10 richieste/secondo massimo: da qui la `Sleep(100ms)` fra i blocchi. Una feature
  che fa **una chiamata per carta** in un loop è sbagliata: usa `/cards/collection`.
- Le immagini non vanno scaricate né cachate lato server: si usa l'URL `image_uris.normal`
  (`thumbURL` in `view.go` ricava la `small` cambiando il path).
- Prezzi: `prices.eur` / `prices.eur_foil` sono **stringhe o null** → `*string` nella struct.
  `price(foil)` fa il parse e il fallback.

## Campi della risposta usati

`id`, `name`, `type_line`, `mana_cost`, `set`, `collector_number`, `image_uris`,
`card_faces[].image_uris` (le bifacciali non hanno `image_uris` in cima),
`prices.eur`, `prices.eur_foil`. Per un campo nuovo: aggiungilo a `scryCard`,
a `Card` (+ colonna in `db.go` schema **e** `migrate`), e a `toCard`.

Documentazione: https://scryfall.com/docs/api — `/cards/search` per la sintassi
delle query, `/cards/collection` per il batch.

## Nei test

Scryfall non si chiama mai. Le funzioni che dipendono da lui prendono dati già
risolti in ingresso (`insertCards(db, id, []Card)`), così `TestDBEIRoutes` gira
offline. Per testare parsing/prezzi si costruisce una `scryCard` a mano
(vedi `TestPriceEImmagine`).
