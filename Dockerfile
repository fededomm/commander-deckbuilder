FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# modernc.org/sqlite è Go puro: niente CGO, binario statico
RUN CGO_ENABLED=0 go build -o /commander-deckbuilder .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates # HTTPS verso Scryfall/Moxfield
COPY --from=build /commander-deckbuilder /commander-deckbuilder
# Render monta il disco persistente qui (Settings → Disks, mount path /data)
VOLUME /data
ENV PORT=10000
EXPOSE 10000
CMD ["sh", "-c", "exec /commander-deckbuilder -host '' -port $PORT -db /data/data.db -no-browser -idle-quit 0"]
