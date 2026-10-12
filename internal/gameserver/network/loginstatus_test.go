package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/link"
)

// TestLoginStatusRegistration pins what a new registration advertises: the
// player cap and reservation as last changed, and the reservation's status
// rather than a down listing the server had before it lost its link.
func TestLoginStatusRegistration(t *testing.T) {
	var sent []link.ServerStatus
	s := NewLoginStatus(true, 100, func(st link.ServerStatus) { sent = append(sent, st) })
	check := func(step string, wantStatus link.ServerType, wantMax int32) {
		t.Helper()
		auth := s.Auth(LoginServerAuth{ServerID: 3})
		if auth.ServerID != 3 || auth.MaxPlayers != wantMax || auth.InitialStatus.Status == nil || *auth.InitialStatus.Status != wantStatus {
			t.Fatalf("%s: auth = %+v, want server 3 capped at %d registering as %v", step, auth, wantMax, wantStatus)
		}
	}
	if s.Type() != link.ServerTypeAuto {
		t.Fatalf("boot type = %v, want Auto", s.Type())
	}
	check("boot", link.ServerTypeGMOnly, 100)

	s.SetType(link.ServerTypeDown)
	check("down", link.ServerTypeGMOnly, 100)
	s.SetGMOnly(false)
	s.SetMaxPlayers(7)
	check("opened", link.ServerTypeAuto, 7)

	if len(sent) != 3 || *sent[0].Status != link.ServerTypeDown || *sent[1].Status != link.ServerTypeAuto || *sent[2].MaxPlayers != 7 {
		t.Fatalf("sent = %+v, want Down, Auto, then a cap of 7", sent)
	}
}
