# Scryfall in questo progetto

Client resty in `internal/scryfall/client.go`, base `https://api.scryfall.com`.
Il pacchetto parla solo il formato di Scryfall (`scryfall.Card`) e non importa
nient'altro del progetto. Lo usa soltanto `internal/service`, che traduce le
risposte nel modello (`internal/service/convert.go`: `toCard`, `toPrints`).
Quattro funzioni, non aggiungerne una quinta se una di queste basta:

| Funzione | Endpoint | Note |
|---|---|---|
| `scryfall.Search(ctx, q)` | `GET /cards/search?unique=cards&q=` | sintassi Scryfall (`t:creature c:g`); 404 = zero risultati → `nil, nil`. Il taglio a 30 lo fa `service.Search` |
| `scryfall.Prints(ctx, q)` | `GET /cards/search?unique=prints&order=released` | tutte le ristampe; l'ordine per prezzo lo fa `service.Prints` con `deck.SortByPrice` |
| `scryfall.Get(ctx, id)` | `GET /cards/{id}` | 404 → `scryfall.ErrNotFound` |
| `scryfall.Collection(ctx, ids)` | `POST /cards/collection` | corpo `{"identifiers": [...]}`; max **75 identifier** a chiamata, spezza e dorme 100 ms fra i blocchi |

`scryfall.Identifier` accetta `id`, oppure `set`+`collector_number`, oppure `name`:
i campi vuoti spariscono dal JSON (`omitempty`). È così che l'import Moxfield
risolve `1 Sol Ring (EOC) 57` in una stampa precisa (`service.Import` →
`matchPrintings`).

## Regole di Scryfall che il codice rispetta

- `User-Agent` identificabile e `Accept: application/json`: senza, rifiuta (anche i POST).
- ~10 richieste/secondo massimo: da qui la `Sleep(100ms)` fra i blocchi. Una feature
  che fa **una chiamata per carta** in un loop è sbagliata: usa `Collection`.
- Le immagini non vanno scaricate né cachate lato server: si usa l'URL `image_uris.normal`
  (`thumbURL` in `internal/ui/format.go` ricava la `small` cambiando il path).
- Prezzi: `prices.eur` / `prices.eur_foil` sono **stringhe o null** → `*string` nella struct.
  `Card.Price(foil)` fa il parse e il fallback.

## Campi della risposta usati

`id`, `oracle_id`, `name`, `type_line`, `mana_cost`, `set`, `set_name`,
`released_at`, `collector_number`, `image_uris`, `card_faces[].image_uris` (le
bifacciali non hanno `image_uris` in cima), `prices.eur`, `prices.eur_foil`.

Per un campo nuovo, in ordine: `scryfall.Card` (tag JSON) → il tipo del modello in
`internal/deck/model.go` (`Card` se va salvato, `Print` se serve solo a mostrarlo)
→ la conversione in `internal/service/convert.go` → se va salvato, colonna in
`internal/store/store.go`, nello `schema` **e** in `migrate`.

Documentazione: https://scryfall.com/docs/api — `/cards/search` per la sintassi
delle query, `/cards/collection` per il batch.

## Nei test

Scryfall vero non si chiama mai. I test di store e web mettono le carte
direttamente nello store (`st.InsertCards(id, []deck.Card{…})`), così girano
offline. Per il client ci sono due modi:
- costruire una `scryfall.Card` a mano per parsing/prezzi (`TestPriceEImmagine`);
- puntare `client` a un `httptest.Server` per controllare la richiesta che parte
  (`TestCollectionRequest`: è quello che controlla la chiave `"identifiers"`).
