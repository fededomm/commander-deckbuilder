// Package web è il livello HTTP: rotte, handler, login e ciclo di vita del
// processo. Un handler legge l'input dalla richiesta, chiama il servizio e
// rende un componente di ui: niente SQL, niente chiamate a Scryfall, niente HTML
// scritto a mano.
package web

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"commander-deckbuilder/internal/deck"
	"commander-deckbuilder/internal/service"
	"commander-deckbuilder/internal/ui"
)

type server struct {
	svc *service.Service
}

// New monta le rotte. password non vuota (AUTH_PASSWORD, online) attiva il login;
// vuota, l'app resta aperta come in locale.
func New(svc *service.Service, password string) *echo.Echo {
	s := &server{svc: svc}
	e := echo.New()
	e.HideBanner, e.HidePort = true, true
	e.Use(middleware.Recover()) // un panic in un handler non deve buttare giù l'app

	if password != "" {
		e.Use(requireAuth(password))
		e.Match([]string{http.MethodGet, http.MethodPost}, "/login", loginHandler(password))
	}

	// Un errore che esce da un handler diventa 500 e finisce nel log; per gli altri
	// codici gli handler usano echo.ErrNotFound & co., che non vanno loggati.
	// deck.ErrNotFound arriva dallo store per un id che non esiste: è un 404.
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if errors.Is(err, deck.ErrNotFound) {
			err = echo.ErrNotFound
		}
		if _, ok := err.(*echo.HTTPError); !ok {
			log.Printf("errore: %v", err)
		}
		e.DefaultHTTPErrorHandler(err, c)
	}

	e.StaticFS("/static", echo.MustSubFS(ui.Static, "static"))

	e.GET("/", s.home)
	e.POST("/decks", s.createDeck)
	e.DELETE("/decks/:id", s.deleteDeck)
	e.POST("/import", s.importDeck)
	e.GET("/alive", aliveHandler) // resta aperta finché la pagina è aperta

	e.GET("/deck/:id", s.deckPage)
	e.GET("/deck/:id/search", s.search)
	e.GET("/deck/:id/prints", s.prints)
	e.POST("/deck/:id/cards", s.addCard)
	e.POST("/deck/:id/prices", s.refreshPrices)
	e.POST("/deck/:id/purchased", s.purchased)
	e.GET("/deck/:id/export", s.export)

	e.DELETE("/cards/:id", s.deleteCard)
	return e
}

func render(c echo.Context, comp templ.Component) error {
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	c.Response().WriteHeader(http.StatusOK)
	return comp.Render(c.Request().Context(), c.Response())
}

func (s *server) renderChecklist(c echo.Context, deckID int64) error {
	d, cards, err := s.svc.Deck(deckID)
	if err != nil {
		return err
	}
	return render(c, ui.Checklist(d, cards))
}

func pathID(c echo.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return 0, echo.NewHTTPError(http.StatusBadRequest, "id non valido")
	}
	return id, nil
}

// safeFilename: il nome del mazzo finisce in un header HTTP, niente virgolette o a capo.
func safeFilename(name string) string {
	clean := strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '_' ||
			(r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return r
		}
		return -1
	}, name)
	if clean = strings.TrimSpace(clean); clean == "" {
		return "mazzo"
	}
	return clean
}
