package deck

import (
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

// ParseMoxfield ignora righe vuote e commenti; restituisce a parte le righe non
// riconosciute (intestazioni tipo "Deck" o "Sideboard") così l'utente le vede.
func ParseMoxfield(text string) (cards []MoxfieldLine, skipped []string) {
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

func ToMoxfield(cards []Card) string {
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
