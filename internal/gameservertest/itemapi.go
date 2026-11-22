package gameservertest

import (
	"net/http"

	"github.com/fatal10110/acis_golang/internal/gameserver/itemapi"
)

// ItemAPI is the character item API over the server's game client link, as
// the debug listener serves it.
func (s *Server) ItemAPI() http.Handler {
	return itemapi.Handler(s.itemEditor)
}
