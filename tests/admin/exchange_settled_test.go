package admin

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// exchangeSettled sends payload, when there is one, and returns every frame
// the server queued to c in answer, in wire order. Unlike exchangeZones it
// reads once the server has handled what was sent and every actor queue has
// run what was posted to it: a manor-list barrier is answered on the
// connection goroutine, which no queued task orders it after, so a debug text
// a game master's queue draws could still be missing when it came back.
func exchangeSettled(t *testing.T, c *testsupport.ScriptedClient, payload []byte) [][]byte {
	t.Helper()
	if payload != nil {
		c.Send(payload)
	}
	return c.ReadQueued()
}
