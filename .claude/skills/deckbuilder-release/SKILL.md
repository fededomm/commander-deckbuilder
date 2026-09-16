---
name: deckbuilder-release
description: Come si costruisce, pubblica e deploya Commander Deckbuilder (questo repo) — pacchetti Windows/macOS/Linux con packaging/build.sh, release GitHub su tag con publish.yml, immagine Docker e deploy su Render. Usare quando si parla di release, versione, tag, build dei pacchetti, installer, .dmg, .exe, workflow GitHub Actions, Dockerfile, Render, deploy, "mettilo online", "fai uscire la 0.4", "la release non è partita", o quando si tocca packaging/, .github/workflows/ o il Dockerfile.
---

# Release e deploy

Il README (sezione "Distribuzione") è la fonte di verità e va tenuto aggiornato
quando cambi qualcosa qui. Questo è il riassunto operativo.

## Due modi di distribuire, non mischiarli

| | Desktop (installer) | Web (Render) |
|---|---|---|
| Ascolta su | `localhost`, porta 8090 o libera | `-host ''` su `$PORT` |
| Browser | lo apre da solo | `-no-browser` |
| Spegnimento | `-idle-quit 5s` a finestre chiuse | `-idle-quit 0` |
| DB | cartella dati dell'utente | `/data/data.db` su disco persistente |

Il codice è lo stesso; cambiano solo i flag. Una feature che presume una
delle due (es. "apri un file sul disco dell'utente") va segnalata.

## Release desktop

```sh
go test ./... && ./packaging/build.sh          # prova locale, esce in dist/
git tag -a v0.4.0 -m v0.4.0 && git push origin v0.4.0
gh workflow run publish.yml --ref v0.4.0       # se dopo un minuto il push del tag non ha avviato nulla
gh run watch                                   # per seguirla
```

- Versione = ultimo tag (`git describe`), non è scritta da nessuna parte nel codice.
- Il job è idempotente: rilanciarlo riallega i file invece di fallire.
- Il job macOS può saltare (`continue-on-error`, runner in coda): la release esce
  comunque con Windows e Linux; il `.dmg` si rifà su un Mac con `./packaging/build.sh macos`.
- Installer Windows: `makensis` (`sudo apt install nsis`); il binario è `-H windowsgui`,
  quindi niente stderr — logga in `deckbuilder.log` accanto al DB.
- Tutto si cross-compila da Linux (`CGO_ENABLED=0`, SQLite in Go puro). Non
  introdurre dipendenze cgo: romperebbero la build multi-target.

## Docker / Render

```sh
docker build -t cdb . && docker run -p 10000:10000 -v cdb-data:/data cdb
```

- Multi-stage: `golang:1.26-alpine` → `alpine` + `ca-certificates` (serve per HTTPS
  verso Scryfall). I `*_templ.go` sono committati, quindi il build non genera nulla.
- La `CMD` è in forma shell per espandere `$PORT` (Render lo imposta, default 10000).
- Su Render: Web Service → Docker, **Disk** con mount path `/data`; senza disco
  il DB sparisce a ogni deploy.
- Non c'è autenticazione: online chiunque può modificare i mazzi. Se serve,
  è un middleware echo (`middleware.BasicAuth`) in `routes()`, non altro.
- Docker non è installato in questa WSL: verifica con `CGO_ENABLED=0 go build` e
  lanciando il binario con gli stessi flag della `CMD`.
