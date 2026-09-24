// Avvio e spegnimento: l'app si ferma quando non ha più finestre aperte.
package web

import (
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v4"
)

var clients atomic.Int64

// aliveHandler tiene aperta una connessione per ogni pagina. Quando chiudi la
// finestra il TCP cade, il context della richiesta salta e il conteggio scende:
// è così che il server si accorge di non servire più a nessuno.
func aliveHandler(c echo.Context) error {
	clients.Add(1)
	defer clients.Add(-1)

	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().WriteHeader(http.StatusOK)
	c.Response().Flush()

	<-c.Request().Context().Done()
	return nil
}

// WatchIdle spegne il server quando l'ultima finestra è chiusa da `grace`.
// L'attesa serve a non morire mentre si passa dalla home a un mazzo: fra le due
// pagine la connessione cade e si riapre.
//
// ponytail: un giro al secondo invece di timer da annullare a ogni connessione.
// Se il browser scarta la scheda in background l'app esce lo stesso: alzare
// -idle-quit, o passare a un timer vero se dà fastidio.
func WatchIdle(grace time.Duration, onExit func()) {
	var seen bool
	var idle time.Duration
	for range time.Tick(time.Second) {
		if clients.Load() > 0 {
			seen, idle = true, 0
			continue
		}
		if !seen {
			continue // il browser non si è ancora collegato: aspetto quanto serve
		}
		if idle += time.Second; idle >= grace {
			log.Printf("nessuna finestra aperta da %s, esco", grace)
			onExit()
			os.Exit(0)
		}
	}
}
