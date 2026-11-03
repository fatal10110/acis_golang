package quest

import (
	"maps"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// TestScriptAdenaRefusedWhileDetaching charges adena through a handle
// whose player went back to character select: every charge reports false,
// even one the player could afford, one it could not and one of nothing,
// no line reaches the client, and the adena is still held on reselect, so
// a payment cannot land in the window where the leaving inventory is no
// longer saved.
func TestScriptAdenaRefusedWhileDetaching(t *testing.T) {
	t.Parallel()
	srv, objID := bootHelpers(t)
	var left *script.Player
	run(t, srv, objID, func(s *script.Script, p *script.Player) {
		s.RewardItems(p, item.AdenaID, 1000)
		left = p
	})
	srv.ReadQueued(t, srv.Client)
	restart(t, srv)
	srv.FlushItems(t)

	for _, count := range []int32{100, 1000, 5000, 0} {
		if left.ReduceAdena(count) {
			t.Errorf("a detached player paid %d adena", count)
		}
	}
	srv.FlushItems(t)
	srv.Client.ExpectNoFrame()
	if got := itemRows(t, srv, objID); !maps.Equal(got, map[int32]int{item.AdenaID: 1000}) {
		t.Fatalf("item rows after the detached charges = %v", got)
	}
	enterWorld(t, srv)
	if got := srv.PlayerItemCount(t, objID, item.AdenaID); got != 1000 {
		t.Fatalf("reselected with %d adena, want 1000", got)
	}
}
