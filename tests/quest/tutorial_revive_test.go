package quest

import (
	"slices"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestTutorialHearsLowHPOnceOnRevive: with RespawnRestoreHP below 30%, a
// revive leaves the player below 30% of its maximum HP and raises CE45
// once. The tutorial hears it after the revive's locks are released: its
// hook asks the player for its resurrection offer, which takes the lock a
// revive holds, and would never return under it. Its answer lands between
// the revive's StatusUpdate and its Revive packet.
func TestTutorialHearsLowHPOnceOnRevive(t *testing.T) {
	t.Parallel()
	heard := &tutorialLog{}
	var probe atomic.Pointer[player.Character]
	hook := func(_ *script.Script, e script.Event) string {
		heard.add(e.Name)
		if c := probe.Load(); c != nil && e.Name == "CE45" {
			c.ReviveOffer()
			e.Player.EnableTutorialEvent(45)
		}
		return ""
	}
	path := "script.feature." + tutorial
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithRespawnRestoreHP(0.2),
		gameservertest.WithScripts([]script.Listing{{Path: path}}, script.Catalog{
			path: func() script.Script { return script.Script{QuestID: -1, Hooks: script.Hooks{OnEvent: hook}} },
		}),
	)
	objID := srv.SoleObjectID(t)
	insertJournal(t, srv.DB, journalRow{objID, tutorial, questlog.KeyState, val("STARTED")})
	enterWorld(t, srv)
	heard.take()

	onCharacter(t, srv, objID, func(c *player.Character) { c.TakeDamage(1_000_000, c) })
	srv.ReadQueued(t, srv.Client)
	if got := heard.take(); !slices.Equal(got, []string{"CE45", "CE45", "CE30"}) {
		t.Fatalf("lethal hit: heard %q, want [CE45 CE45 CE30]", got)
	}

	var revived bool
	onCharacter(t, srv, objID, func(c *player.Character) {
		probe.Store(c)
		revived = c.Revive()
	})
	frames := srv.ReadQueued(t, srv.Client)
	if !revived {
		t.Fatal("Revive did not revive the dead player")
	}
	if got := heard.take(); !slices.Equal(got, []string{"CE45"}) {
		t.Fatalf("revive at 20%%: heard %q, want [CE45]", got)
	}
	status := slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeStatusUpdate })
	mark := slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeTutorialEnableClientEvent })
	revive := slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeRevive })
	if status < 0 || mark < 0 || revive < 0 || status >= mark || mark >= revive {
		t.Fatalf("revive frames %x: StatusUpdate at %d, tutorial answer at %d, Revive at %d; want them in that order", frames, status, mark, revive)
	}
}
