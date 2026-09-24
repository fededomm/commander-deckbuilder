package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"commander-deckbuilder/internal/ui"
)

// Password unica, niente utenti: l'app online la usa solo il proprietario.
// Il cookie è l'HMAC di una costante con la password come chiave: niente
// tabella di sessioni, e cambiare la password su Render butta fuori tutti.
const authCookie = "auth"

func authToken(pw string) string {
	m := hmac.New(sha256.New, []byte(pw))
	m.Write([]byte("commander-deckbuilder"))
	return hex.EncodeToString(m.Sum(nil))
}

func requireAuth(pw string) echo.MiddlewareFunc {
	want := authToken(pw)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			p := c.Request().URL.Path
			if p == "/login" || strings.HasPrefix(p, "/static/") {
				return next(c)
			}
			if ck, err := c.Cookie(authCookie); err == nil &&
				subtle.ConstantTimeCompare([]byte(ck.Value), []byte(want)) == 1 {
				return next(c)
			}
			// htmx incollerebbe la pagina di login dentro la checklist: va rediretto lui.
			if c.Request().Header.Get("HX-Request") != "" {
				c.Response().Header().Set("HX-Redirect", "/login")
				return c.NoContent(http.StatusUnauthorized)
			}
			return c.Redirect(http.StatusSeeOther, "/login")
		}
	}
}

// ponytail: nessun limite ai tentativi. Regge finché la password è lunga e
// casuale; se diventa una parola, aggiungi un rate limit su POST /login.
func loginHandler(pw string) echo.HandlerFunc {
	return func(c echo.Context) error {
		if c.Request().Method == http.MethodGet {
			return render(c, ui.LoginPage(false))
		}
		if subtle.ConstantTimeCompare([]byte(c.FormValue("password")), []byte(pw)) != 1 {
			return render(c, ui.LoginPage(true))
		}
		c.SetCookie(&http.Cookie{
			Name:     authCookie,
			Value:    authToken(pw),
			Path:     "/",
			MaxAge:   int((90 * 24 * time.Hour).Seconds()),
			HttpOnly: true,
			Secure:   true, // online si passa sempre dall'HTTPS di Render
			SameSite: http.SameSiteLaxMode,
		})
		return c.Redirect(http.StatusSeeOther, "/")
	}
}
