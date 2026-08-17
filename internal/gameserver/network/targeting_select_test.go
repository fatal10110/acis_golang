package network

import (
	"context"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func TestSelectAndClearLiveTargetSendsTargetPackets(t *testing.T) {
	state := world.New()
	attackerFrames := &frameCapture{}
	observerFrames := &frameCapture{}
	attacker := newTestLivePlayer(t, 1, attackerFrames)
	observer := newTestLivePlayer(t, 2, observerFrames)
	target := newTestHostileNPC(t, 3)

	state.Spawn(attacker, 0, 0, 0, 0)
	state.Spawn(observer, 100, 0, 0, 0)
	state.Spawn(target, 150, 0, 0, 0)
	attackerFrames.frames = nil
	observerFrames.frames = nil

	gcl := &GameClientLink{world: state, log: zerolog.Nop()}
	if !gcl.selectLiveTarget(attacker, target) {
		t.Fatal("selectLiveTarget returned false")
	}
	if got := frameOpcodes(attackerFrames.frames); string(got) != string([]byte{serverpackets.OpcodeValidateLocation, serverpackets.OpcodeMyTargetSelected, serverpackets.OpcodeStatusUpdate}) {
		t.Fatalf("attacker select opcodes = %x, want ValidateLocation, MyTargetSelected, StatusUpdate", got)
	}
	if got := frameOpcodes(observerFrames.frames); string(got) != string([]byte{serverpackets.OpcodeTargetSelected}) {
		t.Fatalf("observer select opcodes = %x, want TargetSelected", got)
	}

	attackerFrames.frames = nil
	observerFrames.frames = nil
	gcl.clearLiveTarget(attacker)
	if got := frameOpcodes(attackerFrames.frames); string(got) != string([]byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("attacker clear opcodes = %x, want ActionFailed", got)
	}
	if got := frameOpcodes(observerFrames.frames); string(got) != string([]byte{serverpackets.OpcodeTargetUnselected}) {
		t.Fatalf("observer clear opcodes = %x, want TargetUnselected", got)
	}
}

// TestSelectLiveTargetOmitsValidateLocationForStaticObject is the
// regression test for a PR #1378 review comment: selectLiveTarget's
// ValidateLocation gate originally duck-typed on Position() alone, which
// staticobject.Chair satisfies (model/staticobject/chair.go:14-22) despite
// having no Heading() — so a first click on a chair sent a spurious
// ValidateLocation with a meaningless zero heading. The reference's
// ValidateLocation leg sits strictly inside Player.setTarget's
// `else if (newTarget instanceof Creature)` branch (Player.java:2474-2475);
// the preceding StaticObject branch (Player.java:2465-2470) sends only
// MyTargetSelected + StaticObjectInfo, never ValidateLocation. The gate now
// requires both Position() and Heading(), which excludes Chair.
func TestSelectLiveTargetOmitsValidateLocationForStaticObject(t *testing.T) {
	state := world.New()
	frames := &frameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	chair, err := staticobject.NewObject(2, &staticobject.Template{
		ID:       777,
		Location: location.Location{X: 100, Y: 0, Z: 0},
		Type:     1,
	})
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}

	state.Spawn(live, 0, 0, 0, 0)
	state.Spawn(chair, 100, 0, 0, 0)
	frames.frames = nil

	gcl := &GameClientLink{world: state, log: zerolog.Nop()}
	if !gcl.selectLiveTarget(live, chair) {
		t.Fatal("selectLiveTarget returned false")
	}
	if got := frameOpcodes(frames.frames); string(got) != string([]byte{serverpackets.OpcodeMyTargetSelected}) {
		t.Fatalf("chair select opcodes = %x, want MyTargetSelected only (no ValidateLocation)", got)
	}
}

// TestSelectLiveTargetSendsValidateLocationForSummon is the regression test
// for a follow-up to the Chair fix above: gating ValidateLocation on
// AttackableBy(skilltarget.Creature) (targetColor's Creature discriminator)
// under-matches, since only npc.Hostile and player.Character implement it —
// summon.Actor doesn't, so that gate would silently drop ValidateLocation
// for a pet/servitor target too, even though Summon is a Creature in the
// reference (no override of setTarget in the Summon hierarchy) and belongs
// in the same branch as Hostile/Player targets (Player.java:2474-2493). The
// gate now excludes only staticobject.Chair, so a summon target (which has
// Position()/Heading() via its embedded world.Presence, same as Hostile and
// livePlayer) still gets ValidateLocation.
func TestSelectLiveTargetSendsValidateLocationForSummon(t *testing.T) {
	state := world.New()
	frames := &frameCapture{}
	attacker := newTestLivePlayer(t, 1, frames)
	pet := summon.NewPet(summon.PetConfig{ObjectID: 501, Level: 44, Stats: summon.CombatStats{MaxHP: 500, MaxMP: 200}})

	state.Spawn(attacker, 0, 0, 0, 0)
	state.Spawn(pet, 100, 0, 0, 0)
	frames.frames = nil

	gcl := &GameClientLink{world: state, log: zerolog.Nop()}
	if !gcl.selectLiveTarget(attacker, pet) {
		t.Fatal("selectLiveTarget returned false")
	}
	if got := frameOpcodes(frames.frames); len(got) == 0 || got[0] != serverpackets.OpcodeValidateLocation {
		t.Fatalf("summon select opcodes = %x, want leading ValidateLocation", got)
	}
}

func TestGameClientLinkActionSitsOnSelectedChairStaticObject(t *testing.T) {
	state := world.New()
	frames := &frameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	chair, err := staticobject.NewObject(2, &staticobject.Template{
		ID:       777,
		Location: location.Location{X: 100, Y: 0, Z: 0},
		Type:     1,
	})
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}

	state.Spawn(live, 0, 0, 0, 0)
	state.Spawn(chair, 100, 0, 0, 0)
	frames.frames = nil
	live.SetTargetTracked(chair)

	gcl := &GameClientLink{world: state, log: zerolog.Nop()}
	gcl.handleTargetAction(context.Background(), live, chair.ObjectID(), true, false)

	if got := frameOpcodes(frames.frames); string(got) != string([]byte{serverpackets.OpcodeChangeWaitType, serverpackets.OpcodeActionFailed, serverpackets.OpcodeChairSit}) {
		t.Fatalf("chair action opcodes = %x, want ChangeWaitType, ActionFailed, ChairSit", got)
	}
	if live.Standing() {
		t.Fatal("live player remained standing after chair action")
	}
	if !chair.Busy() {
		t.Fatal("chair was not marked busy")
	}

	r := wire.NewReader(frames.frames[2][1:])
	if got := r.ReadInt32(); got != live.ObjectID() {
		t.Fatalf("ChairSit player id = %d, want %d", got, live.ObjectID())
	}
	if got := r.ReadInt32(); got != int32(chair.StaticObjectID()) {
		t.Fatalf("ChairSit static id = %d, want %d", got, chair.StaticObjectID())
	}

	frames.frames = nil
	gcl.changeLiveWaitType(live, true)
	if chair.Busy() {
		t.Fatal("chair stayed busy after standing")
	}
	if !live.Standing() {
		t.Fatal("live player did not stand after stand request")
	}
	if got := frameOpcodes(frames.frames); string(got) != string([]byte{serverpackets.OpcodeChangeWaitType}) {
		t.Fatalf("stand opcodes = %x, want ChangeWaitType", got)
	}
}

func TestGameClientLinkResolveTargetFallsBackToPlayerRegistry(t *testing.T) {
	state := world.New()
	targetFrames := &frameCapture{}
	target := newTestLivePlayer(t, 42, targetFrames)
	state.AddPlayer(target)

	gcl := &GameClientLink{world: state, log: zerolog.Nop()}
	if got := gcl.resolveTarget(target.ObjectID()); got != target {
		t.Fatalf("resolveTarget(player) = %v, want player registry target", got)
	}
}
