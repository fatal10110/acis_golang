package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// afflict puts the named effect (Stun, Confusion, ...) on the player.
func (w *folkWorld) afflict(t *testing.T, name string) {
	t.Helper()
	w.onPlayer(t, func(pc *player.Character) {
		e, err := effect.New(effect.Skill{ID: 2, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: name, Time: 30})
		if err != nil {
			t.Errorf("effect.New(%s): %v", name, err)
			return
		}
		e.Effector, e.Effected = pc, pc
		pc.EffectList().Add(e)
	})
	drainUntilQuiet(t, w.c)
}

// playerTarget reads the player's selected object.
func (w *folkWorld) playerTarget(t *testing.T) world.Tracked {
	t.Helper()
	var target world.Tracked
	w.onPlayer(t, func(pc *player.Character) { target = pc.Target() })
	return target
}

// expectRefusedAttackRequest sends an AttackRequest on objectID and checks
// it is answered ActionFailed alone: no selection, walk, swing, chat page or
// town map.
func (w *folkWorld) expectRefusedAttackRequest(t *testing.T, objectID int32, what string) {
	t.Helper()
	w.c.Send(encodeAttackRequest(objectID, w.at))
	w.srv.Settle(t)
	if got := opcodes(drainFrames(t, w.c)); string(got) != string([]byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("%s = opcodes %x, want ActionFailed alone", what, got)
	}
	if x, y, _ := w.srv.PlayerPosition(t, w.player); x != w.at.X || y != w.at.Y {
		t.Fatalf("%s moved the player to (%d,%d), want it held at (%d,%d)", what, x, y, w.at.X, w.at.Y)
	}
}

// TestOutOfControlAttackRequestRefusedUpFront pins AttackRequest.runImpl
// refusing a player out of control (Creature.isOutOfControl) with
// ActionFailed before it resolves the object: the request selects nothing
// new, and a confused player (out of control, but not denied AI actions)
// neither walks to nor interacts with what it has selected.
func TestOutOfControlAttackRequestRefusedUpFront(t *testing.T) {
	t.Parallel()

	t.Run("stunned on an unselected monster", func(t *testing.T) {
		t.Parallel()
		w := bootFolkWorld(t, nil)
		h := w.srv.SpawnHostileNPCAt(t, location.Location{X: w.at.X + 60, Y: w.at.Y, Z: w.at.Z})
		drainUntilQuiet(t, w.c)
		w.afflict(t, "Stun")
		w.expectRefusedAttackRequest(t, h.ObjectID(), "stunned attack request on an unselected monster")
		if got := w.playerTarget(t); got != nil {
			t.Fatalf("target after the refused request = %d, want none", got.ObjectID())
		}
	})

	t.Run("confused on a selected monster corpse", func(t *testing.T) {
		t.Parallel()
		w := bootFolkWorld(t, nil)
		h := w.srv.SpawnHostileNPCAt(t, location.Location{X: w.at.X + 200, Y: w.at.Y, Z: w.at.Z})
		drainUntilQuiet(t, w.c)
		if !h.Kill(nil) {
			t.Fatal("monster did not die")
		}
		drainUntilQuiet(t, w.c)
		w.c.Send(encodeAction(h.ObjectID(), w.at, false))
		drainUntilQuiet(t, w.c)
		w.afflict(t, "Confusion")
		w.expectRefusedAttackRequest(t, h.ObjectID(), "confused forced click on a selected monster corpse")
		if got := w.playerTarget(t); got == nil || got.ObjectID() != h.ObjectID() {
			t.Fatalf("target after the refused request = %v, want the corpse kept", got)
		}
	})

	t.Run("confused on a selected static object", func(t *testing.T) {
		t.Parallel()
		w := bootFolkWorld(t, nil)
		at := location.Location{X: w.at.X + 50, Y: w.at.Y, Z: w.at.Z}
		obj, err := staticobject.NewObject(w.srv.NewObjectID(), &staticobject.Template{
			ID: 24180018, Location: at, Type: staticobject.MapType, Texture: "testmap", MapX: 17, MapY: 21,
		})
		if err != nil {
			t.Fatal(err)
		}
		w.srv.State.Spawn(obj, at.X, at.Y, at.Z, 0)
		drainUntilQuiet(t, w.c)
		w.c.Send(encodeAction(obj.ObjectID(), w.at, false))
		drainUntilQuiet(t, w.c)
		w.afflict(t, "Confusion")
		w.expectRefusedAttackRequest(t, obj.ObjectID(), "confused forced click on a selected town map")
	})
}
