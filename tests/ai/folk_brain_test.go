package ai

import (
	"encoding/binary"
	"fmt"
	"slices"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// opcodesOf returns, in order, the opcodes among want of the frames whose
// leading object id is id.
func opcodesOf(frames [][]byte, id int32, want ...byte) []byte {
	var out []byte
	for _, fr := range frames {
		if len(fr) >= 5 && slices.Contains(want, fr[0]) && int32(binary.LittleEndian.Uint32(fr[1:5])) == id {
			out = append(out, fr[0])
		}
	}
	return out
}

// TestFolkFirstAttackReactsBeforeTheHitsHPChange runs a civilian border
// patrol under a behavior shaped like its reference Guard, whose attacked
// hook asks it to attack the attacker. The hook runs on the attacker's
// queue before the hit's HP change, and the patrol hates no one yet, so its
// desire selection runs right there, in place: the player watching it sees
// its swing before its StatusUpdate for the hit, on either executor.
func TestFolkFirstAttackReactsBeforeTheHitsHPChange(t *testing.T) {
	t.Parallel()
	for _, exec := range []struct {
		name string
		opts []gameservertest.Option
	}{
		{"inline", nil},
		{"pool", []gameservertest.Option{gameservertest.WithRealPool()}},
	} {
		t.Run(exec.name, func(t *testing.T) {
			t.Parallel()
			guard := func() script.Script {
				return script.Script{Behavior: true, NPCs: []int32{folkPatrolID}, Hooks: script.Hooks{
					OnAttacked: func(_ *script.Script, e script.Attacked) {
						e.NPC.AddAttackDesire(e.Attacker, 2000)
					},
				}}
			}
			srv, at := bootFolkDesires(t, script.Catalog{"ai.FolkGuardInPlace": guard}, exec.opts...)
			player := characterOf(t, srv)
			f := srv.SpawnFolkNPCAt(t, folkFighterTemplate(folkPatrolID), location.Location{X: at.X + 40, Y: at.Y, Z: at.Z})
			id := f.ObjectID()
			srv.ReadQueued(t, srv.Client)
			// Select the patrol, so its HP changes reach the player.
			srv.Client.Send(encodeFolkAction(id, at))
			srv.ReadQueued(t, srv.Client)

			// A hit large enough to move the patrol's health bar.
			hitFolk(t, srv, f, player, 30)
			got := opcodesOf(srv.ReadQueued(t, srv.Client), id, serverpackets.OpcodeAttack, serverpackets.OpcodeStatusUpdate)
			if want := []byte{serverpackets.OpcodeAttack, serverpackets.OpcodeStatusUpdate}; !slices.Equal(got, want) {
				t.Fatalf("patrol frames = % x, want % x: the swing before the hit's HP update", got, want)
			}
			if got := f.CurrentIntention(); got != actorai.IntentionAttack {
				t.Fatalf("intention = %v, want attack", got)
			}
		})
	}
}

// TestFolkNestedAttackFromTheNoDesireHook runs a civilian NPC whose
// no-desire hook asks it to attack the character. The idle opens the hook
// with the AI's brain released, so the attack's desire selection runs in
// place, nested inside the tick's pass, and the NPC swings within the
// same tick; the tick's pass then goes on, and the hook ran once.
func TestFolkNestedAttackFromTheNoDesireHook(t *testing.T) {
	t.Parallel()
	log := &folkLog{}
	var target script.Creature
	sentry := func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{folkOfferingID}, Hooks: script.Hooks{
			OnNoDesire: func(_ *script.Script, e script.NoDesire) {
				if log.add("NO_DESIRE") == 1 {
					e.NPC.AddAttackDesire(target, 2000)
				}
			},
		}}
	}
	srv, at := bootFolkDesires(t, script.Catalog{"ai.FolkSentry": sentry})
	player := characterOf(t, srv)
	target = script.PlayerOf(player)
	f := srv.SpawnFolkNPCAt(t, folkFighterTemplate(folkOfferingID), location.Location{X: at.X + 40, Y: at.Y, Z: at.Z})
	id := f.ObjectID()
	srv.ReadQueued(t, srv.Client)

	cycle(t, srv)
	swings := framesOf(cycle(t, srv), serverpackets.OpcodeAttack, id)
	if got := log.take(); !slices.Equal(got, []string{"NO_DESIRE"}) {
		t.Fatalf("hooks on the second cycle = %q, want the idle's no-desire once", got)
	}
	if len(swings) != 1 || attackTarget(swings[0]) != player.ObjectID() {
		t.Fatalf("swings on the idle's cycle = %d, want one at the character", len(swings))
	}
	if got := f.CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("intention = %v, want attack", got)
	}
}

// TestFolkChaseOutOfTerritorySweepsStaleAggro runs a civilian border patrol
// that chases a character standing outside its territory. The chase ends
// outside, so its arrival runs the out-of-territory hook, once: a later
// arrival still outside runs none, and only one after an arrival back
// inside runs it again. The arrival also arms the stale-aggro sweep: 100 ms
// later, then every 10 s, while the patrol is outside, an aggro list entry
// whose last damage is 90 s old loses its hate and the patrol's attack
// desire on it. No AI tick runs meanwhile, so nothing else touches the hate.
func TestFolkChaseOutOfTerritorySweepsStaleAggro(t *testing.T) {
	t.Parallel()
	log := &folkLog{}
	patrol := func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{folkPatrolID}, Hooks: script.Hooks{
			OnOutOfTerritory: func(_ *script.Script, e script.OutOfTerritory) {
				log.add("OUT_OF_TERRITORY")
			},
			OnMoveToFinished: func(_ *script.Script, e script.MoveToFinished) {
				log.add("MOVE_TO_FINISHED at=%d,%d,%d", e.X, e.Y, e.Z)
			},
		}}
	}
	srv, at := bootFolkDesires(t, script.Catalog{"ai.FolkPatrolTerritory": patrol})
	player := characterOf(t, srv)
	home := location.Location{X: at.X + 500, Y: at.Y, Z: at.Z}
	f := srv.SpawnFolkNPCAt(t, folkFighterTemplate(folkPatrolID), home)
	id := f.ObjectID()
	srv.ReadQueued(t, srv.Client)
	onPlayer := &actorai.Desire{Kind: actorai.IntentionAttack, FinalTarget: player}

	f.AddAttackDesire(player, 2000)
	if chase := framesOf(srv.ReadQueued(t, srv.Client), serverpackets.OpcodeMoveToLocation, id); len(chase) != 1 {
		t.Fatalf("chase frames = %d, want one walk toward the character", len(chase))
	}
	walkUntilArrived(t, srv, f)
	if f.InTerritory() {
		t.Fatal("the chase ended inside the patrol's territory; want outside")
	}
	if got, want := log.take(), []string{"OUT_OF_TERRITORY"}; !slices.Equal(got, want) {
		t.Fatalf("hooks on the chase's arrival = %q, want %q", got, want)
	}

	// Short of 90 s since the attack desire stamped the entry, the sweeps
	// keep it.
	srv.Advance(t, 80*time.Second)
	if c, ok := f.MostHated(); !ok || c.ObjectID() != player.ObjectID() {
		t.Fatalf("most hated after 80 s = %v, %v; want the character still", c, ok)
	}
	if !f.Desires().Has(onPlayer) {
		t.Fatal("the attack desire on the character is gone after 80 s; want it kept")
	}
	// The sweep 90.1 s after the arrival finds the entry stale.
	srv.Advance(t, 10200*time.Millisecond)
	if c, ok := f.MostHated(); ok {
		t.Fatalf("most hated after the stale sweep = %v; want none", c)
	}
	if f.Desires().Has(onPlayer) {
		t.Fatal("the attack desire on the character survived the stale sweep")
	}

	walk := func(to location.Location, want ...string) {
		t.Helper()
		if !f.AddMoveToDesire(to, 1e6) {
			t.Fatalf("AddMoveToDesire(%+v) = false, want the walk queued", to)
		}
		// The first walk waits out the patrol's first AI tick, then the
		// attack its chase latched, which the selection takes up once more
		// although its desire left the queue, and that swing's end.
		for i := 0; ; i++ {
			if dest, moving := f.MovingTo(); moving && dest == to {
				break
			}
			if i == 5 {
				t.Fatalf("intention = %v after five cycles, want the walk to %+v", f.CurrentIntention(), to)
			}
			cycle(t, srv)
			srv.Advance(t, time.Second)
		}
		walkUntilArrived(t, srv, f)
		if got := log.take(); !slices.Equal(got, want) {
			t.Fatalf("hooks on arriving at %+v = %q, want %q", to, got, want)
		}
	}
	finished := func(l location.Location) string {
		return fmt.Sprintf("MOVE_TO_FINISHED at=%d,%d,%d", l.X, l.Y, l.Z)
	}
	out := location.Location{X: home.X, Y: home.Y + 300, Z: home.Z}
	walk(out, finished(out))
	walk(home, finished(home))
	walk(out, finished(out), "OUT_OF_TERRITORY")
}
