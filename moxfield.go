package main

import (
	"context"
	"regexp"
	"strconv"
	"strings"
)

// MoxfieldLine è una riga della lista: quantità, nome, stampa (opzionale) e foil.
type MoxfieldLine struct {
	Qty             int
	Name            string
	SetCode         string
	CollectorNumber string
	Foil            bool
}

// "1 Sol Ring (EOC) 57 *F*" — set e numero opzionali, i flag in coda possono essere più d'uno.
var moxfield = regexp.MustCompile(`^\s*(\d+)x?\s+(.+?)(?:\s+\(([\w-]+)\)\s+(\S+))?((?:\s+\*\w+\*)*)\s*$`)

// parseMoxfield ignora righe vuote e commenti; restituisce a parte le righe non
// riconosciute (intestazioni tipo "Deck" o "Sideboard") così l'utente le vede.
func parseMoxfield(text string) (cards []MoxfieldLine, skipped []string) {
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimRight(line, "\r")
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "//") {
			continue
		}
		m := moxfield.FindStringSubmatch(line)
		if m == nil {
			skipped = append(skipped, t)
			continue
		}
		qty, _ := strconv.Atoi(m[1])
		cards = append(cards, MoxfieldLine{
			Qty:             qty,
			Name:            strings.TrimSpace(m[2]),
			SetCode:         strings.ToLower(m[3]),
			CollectorNumber: m[4],
			Foil:            strings.Contains(strings.ToUpper(m[5]), "*F*"),
		})
	}
	return cards, skipped
}

func toMoxfield(cards []Card) string {
	var b strings.Builder
	for _, c := range cards {
		// Scryfall usa "A // B" per le bifacciali, Moxfield "A / B"
		b.WriteString(strconv.Itoa(max(c.Qty, 1)))
		b.WriteByte(' ')
		b.WriteString(strings.ReplaceAll(c.Name, " // ", " / "))
		if c.SetCode != "" {
			b.WriteString(" (" + strings.ToUpper(c.SetCode) + ") " + c.CollectorNumber)
		}
		if c.Foil {
			b.WriteString(" *F*")
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// identifiers: se ho set e numero uso quelli (è la stampa esatta), altrimenti il nome.
func (l MoxfieldLine) identifier() identifier {
	if l.SetCode != "" {
		return identifier{Set: l.SetCode, CollectorNumber: l.CollectorNumber}
	}
	return identifier{Name: l.Name}
}

// resolvePrintings mappa le righe Moxfield sulle stampe Scryfall.
// missing sono le righe che Scryfall non ha riconosciuto.
func resolvePrintings(ctx context.Context, lines []MoxfieldLine) (rows []Card, missing []MoxfieldLine, err error) {
	ids := make([]identifier, len(lines))
	for i, l := range lines {
		ids[i] = l.identifier()
	}
	found, err := scryfallCollection(ctx, ids)
	if err != nil {
		return nil, nil, err
	}

	bySet := map[string]scryCard{}
	byName := map[string]scryCard{}
	for _, c := range found {
		bySet[c.Set+"|"+c.CollectorNumber] = c
		byName[strings.ToLower(c.Name)] = c
	}
	for _, l := range lines {
		c, ok := bySet[l.SetCode+"|"+l.CollectorNumber]
		if !ok {
			// Moxfield scrive "A / B" le bifacciali, Scryfall "A // B"
			c, ok = byName[strings.ToLower(strings.ReplaceAll(l.Name, " / ", " // "))]
		}
		if !ok {
			missing = append(missing, l)
			continue
		}
		rows = append(rows, c.toCard(l.Qty, l.Foil))
	}
	return rows, missing, nil
}
