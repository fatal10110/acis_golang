package gameservertest

import "testing"

// ShowDialogPage opens html on the live player objID's client the way a
// script shows a whole dialog page with no NPC: the window, then
// ActionFailed. Its links become the bypass commands the player may send
// back.
func (s *Server) ShowDialogPage(tb testing.TB, objID int32, html string) {
	tb.Helper()
	s.onlineCharacter(tb, objID).ShowDialogPage(0, "", html)
}
