package main

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func routes() *echo.Echo {
	e := echo.New()
	e.HideBanner, e.HidePort = true, true
	e.Use(middleware.Recover()) // un panic in un handler non deve buttare giù l'app

	// Un errore che esce da un handler diventa 500 e finisce nel log; per gli altri
	// codici gli handler usano echo.ErrNotFound & co., che non vanno loggati.
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if _, ok := err.(*echo.HTTPError); !ok {
			log.Printf("errore: %v", err)
		}
		e.DefaultHTTPErrorHandler(err, c)
	}

	e.StaticFS("/static", echo.MustSubFS(staticFS, "static"))

	e.GET("/", home)
	e.POST("/decks", createDeckHandler)
	e.DELETE("/decks/:id", deleteDeckHandler)
	e.POST("/import", importHandler)
	e.POST("/quit", quitHandler)

	e.GET("/deck/:id", deckPageHandler)
	e.GET("/deck/:id/search", searchHandler)
	e.POST("/deck/:id/cards", addCardHandler)
	e.POST("/deck/:id/prices", refreshPricesHandler)
	e.GET("/deck/:id/export", exportHandler)

	e.POST("/cards/:id/toggle", toggleHandler)
	e.DELETE("/cards/:id", deleteCardHandler)
	return e
}

func render(c echo.Context, comp templ.Component) error {
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	c.Response().WriteHeader(http.StatusOK)
	return comp.Render(c.Request().Context(), c.Response())
}

func renderChecklist(c echo.Context, deckID int64) error {
	name, err := deckName(db, deckID)
	if err != nil {
		return err
	}
	cards, err := deckCards(db, deckID)
	if err != nil {
		return err
	}
	return render(c, checklist(Deck{ID: deckID, Name: name}, cards))
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
