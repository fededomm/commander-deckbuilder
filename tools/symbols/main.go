// Scarica gli SVG dei simboli di mana da Scryfall in static/symbols/ e rigenera
// symbols_gen.go con la mappa simbolo -> file.
// Da rilanciare solo quando Scryfall aggiunge simboli nuovi: go run ./tools/symbols
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const userAgent = "commander-deckbuilder/0.2 (https://github.com/local)"

func main() {
	var body struct {
		Data []struct {
			Symbol string `json:"symbol"`
			SVGURI string `json:"svg_uri"`
		} `json:"data"`
	}
	if err := get("https://api.scryfall.com/symbology", &body); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll("static/symbols", 0o755); err != nil {
		log.Fatal(err)
	}

	files := map[string]string{}
	var bytes int
	for _, s := range body.Data {
		if s.SVGURI == "" {
			continue
		}
		file := path.Base(strings.Split(s.SVGURI, "?")[0]) // il nome file lo decide Scryfall
		svg, err := download(s.SVGURI)
		if err != nil {
			log.Printf("salto %s: %v", s.Symbol, err)
			continue
		}
		if err := os.WriteFile(filepath.Join("static/symbols", file), svg, 0o644); err != nil {
			log.Fatal(err)
		}
		files[s.Symbol] = file
		bytes += len(svg)
	}

	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out strings.Builder
	out.WriteString("// Generato da tools/symbols — non modificare a mano.\npackage main\n\n")
	out.WriteString("// symbols mappa il simbolo Scryfall ({W}, {2/U}, …) al file SVG in static/symbols/.\nvar symbols = map[string]string{\n")
	for _, k := range keys {
		fmt.Fprintf(&out, "\t%q: %q,\n", k, files[k])
	}
	out.WriteString("}\n")
	if err := os.WriteFile("symbols_gen.go", []byte(out.String()), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d simboli, %d KB in static/symbols/\n", len(files), bytes/1024)
}

func get(url string, out any) error {
	b, err := download(url)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func download(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return io.ReadAll(res.Body)
}
