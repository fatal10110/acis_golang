package ai

import (
	"maps"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// TestHeartOfWardingWakeOutlivesTheEntrantsLogout: the first entrant
// logging out inside the boss wait does not cancel the wake. At the wait's
// end the Heart still wakes the small Antharas (one entrant) and clears its
// wait mark and count, so the next entrant starts a new wait. The wake timer
// is bound to no player: an owner-approved divergence (#5132).
func TestHeartOfWardingWakeOutlivesTheEntrantsLogout(t *testing.T) {
	w := bootHeart(t, nil, 1)
	n := heartNPC(t, w)
	onHeart := w.srv.ScriptOffQueue(t, w.player.ObjectID(), heartDirector)
	enterHeart(t, w)
	w.appear(t)
	if waiting, entrants := heartCounts(t, w); waiting != 1 || entrants != 1 {
		t.Fatalf("the Heart's wait mark and entrants = %d, %d, want 1, 1", waiting, entrants)
	}

	w.srv.Client.Send(wire.NewPacketWriter(clientpackets.OpcodeLogout).Bytes())
	w.srv.AwaitConnectionEnded(t, w.srv.Client)

	rowValues := func() map[string]int {
		out := map[string]int{}
		for _, size := range heartSizes {
			v, _ := w.srv.NpcSpawns.Spawns().DBValue(size.row)
			out[size.row] = v
		}
		return out
	}
	pass := func(d time.Duration) {
		w.elapsed.Add(int64(d))
		w.srv.Advance(t, d)
	}

	pass(30*time.Minute - time.Second)
	if got, want := rowValues(), map[string]int{"antaras_min": 0, "antaras_normal": 0, "antaras_max": 0}; !maps.Equal(got, want) {
		t.Fatalf("db values before the wait ends = %v, want %v", got, want)
	}
	pass(time.Second)
	if got, want := rowValues(), map[string]int{"antaras_min": 1, "antaras_normal": 0, "antaras_max": 0}; !maps.Equal(got, want) {
		t.Fatalf("db values once the wait ends = %v, want %v", got, want)
	}
	var waiting, entrants int32
	if !onHeart(func(*script.Script, *script.Player) {
		waiting, entrants = n.Int(script.SlotIntAI0), n.Int(script.SlotIntAI1)
	}) {
		t.Fatal("the Heart's memory read did not run")
	}
	if waiting != 0 || entrants != 0 {
		t.Fatalf("the Heart's wait mark and entrants after the wake = %d, %d, want 0, 0", waiting, entrants)
	}
}
