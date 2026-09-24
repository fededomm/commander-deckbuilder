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
| DB | cartella dati dell'utente | Turso: `DB_URL=libsql://…` + `TURSO_AUTH_TOKEN` |
| Accesso | libero | pagina di login se c'è `AUTH_PASSWORD` |

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
- Installer Windows: `makensis` (`sudo apt install nsis`, non è installato in questa
  WSL: si prova con `gh workflow run publish.yml --ref <branch>`, che costruisce tutto
  senza pubblicare). Wizard MUI2: welcome → directory → components → instfiles → finish.
  Il binario è `-H windowsgui`, quindi niente stderr — logga in `deckbuilder.log` accanto al DB.
- macOS: su un runner Mac lo script firma ad-hoc il `.app` e fa un `.dmg` con il
  collegamento ad Applicazioni. Da Linux escono solo gli zip.
- Linux: amd64 e arm64.
- Tutto si cross-compila da Linux (`CGO_ENABLED=0`, SQLite in Go puro). Non
  introdurre dipendenze cgo: romperebbero la build multi-target.

## Docker / Render

```sh
docker build -t cdb . && docker run -p 10000:10000 -v cdb-data:/data cdb
```

- Multi-stage: `golang:1.26-alpine` → `alpine` + `ca-certificates` (serve per HTTPS
  verso Scryfall). I `*_templ.go` sono committati, quindi il build non genera nulla.
- La `CMD` è in forma shell per espandere `$PORT` (Render lo imposta, default 10000).
- Su Render (piano free, servizio `commander-deckbuilder`, branch `go-templ`,
  auto-deploy a ogni push): il disco del container sparisce a ogni spin-down, per
  questo il DB è su Turso (`DB_URL`, `TURSO_AUTH_TOKEN`). Senza `DB_URL` la `CMD`
  ripiega su `/data/data.db`, che ha senso solo con un Disk (piano a pagamento).
- Accesso: `AUTH_PASSWORD` attiva la pagina di login (`auth.go`, cookie firmato).
  Le variabili si cambiano dall'MCP di Render o dalla dashboard.
- Docker non è installato in questa WSL: verifica con `CGO_ENABLED=0 go build` e
  lanciando il binario con gli stessi flag della `CMD`.
