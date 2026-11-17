package network

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/testsupport/linkpeer"
)

// referenceLinkPayload returns the payload of the golden line of kind whose
// attributes or accounts read key, as the reference sent it.
func referenceLinkPayload(t *testing.T, kind, key string) []byte {
	t.Helper()
	data, err := os.ReadFile("../../link/testdata/link.golden")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == kind && fields[1] == key {
			payload, err := hex.DecodeString(fields[2])
			if err != nil {
				t.Fatal(err)
			}
			return payload
		}
	}
	t.Fatalf("link.golden has no %s %s line", kind, key)
	return nil
}

// TestInitialServerStatusOnTheWire registers with a login server that records
// what comes next: the first frame is the reference's six-attribute
// ServerStatus (status, clock, brackets, age limit, test server, pvp server,
// in that order, no player cap), with the status the reservation gives, and a
// PlayerInGame sent after it arrives after it.
func TestInitialServerStatusOnTheWire(t *testing.T) {
	t.Parallel()
	on, off, age := true, false, int32(15)
	for _, tc := range []struct {
		name   string
		gmOnly bool
		attrs  string
	}{
		{"open", false, "1:0,2:1,3:0,4:15,5:0,6:1"},
		{"gm only", true, "1:5,2:1,3:0,4:15,5:0,6:1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			peer := linkpeer.Start(t, 3, "Giran")
			base := LoginServerAuth{ServerID: 3, HostName: "*", Port: 7777, MaxPlayers: 300, InitialStatus: link.ServerStatus{
				ShowClock: &on, ShowBrackets: &off, AgeLimit: &age, TestServer: &off, Pvp: &on,
			}}
			auth := NewLoginStatus(tc.gmOnly, 300, func(link.ServerStatus) {}).Auth(base)
			l, err := DialLoginLink(context.Background(), peer.Addr(), auth, LoginLinkHandlers{}, zerolog.Nop())
			if err != nil {
				t.Fatalf("DialLoginLink: %v", err)
			}
			defer l.Close()
			if l.ServerID != 3 || l.ServerName != "Giran" {
				t.Fatalf("registered as %d %q, want 3 Giran", l.ServerID, l.ServerName)
			}
			if err := l.SendPlayerInGame([]string{"carol"}); err != nil {
				t.Fatal(err)
			}

			// The reference's frames end in the checksum and padding its
			// transport adds; only the payload is compared.
			const statusLen, rosterLen = 1 + 4 + 6*8, 1 + 2 + len("carol")*2 + 2
			status := peer.Next(t)
			want := referenceLinkPayload(t, "serverstatus", tc.attrs)
			if len(status) != len(want) || !bytes.Equal(status[:statusLen], want[:statusLen]) {
				t.Errorf("first frame after registration = %x, want the reference's ServerStatus %x", status, want)
			}
			roster := peer.Next(t)
			wantRoster := referenceLinkPayload(t, "playeringame", "carol")
			if len(roster) != len(wantRoster) || !bytes.Equal(roster[:rosterLen], wantRoster[:rosterLen]) {
				t.Errorf("second frame = %x, want the reference's PlayerInGame %x", roster, wantRoster)
			}
		})
	}
}
