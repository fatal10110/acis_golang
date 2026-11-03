package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// Reference for the tests below (#3884): a plain click on a selected
// monster and an attack an effect forces on the player both reach
// PlayableAI.tryToAttack (PlayableAI.java:236-241), whose only up-front gate
// is denyAiAction (Creature.java:636-639, Player.java:627-630): stunned,
// immobile until attacked, asleep, paralyzed, teleporting, dead, afraid, or
// in store mode, answered ActionFailed with no intention change. Confusion
// is out of control (Creature.isOutOfControl) but not a denial, so only the
// forced click (AttackRequest) refuses a confused player.

// selectedMonster spawns a monster beside the player and selects it with a
// plain click.
func (w *folkWorld) selectedMonster(t *testing.T) *npc.Hostile {
	t.Helper()
	h := w.srv.SpawnHostileNPCAt(t, location.Location{X: w.at.X + 200, Y: w.at.Y, Z: w.at.Z})
	drainUntilQuiet(t, w.c)
	w.c.Send(encodeAction(h.ObjectID(), w.at, false))
	drainUntilQuiet(t, w.c)
	if got := w.playerTarget(t); got == nil || got.ObjectID() != h.ObjectID() {
		t.Fatalf("target after the selecting click = %v, want the monster", got)
	}
	return h
}

// attackedMonster reports whether frames hold the player's attack on h:
// the walk to it or a swing at it.
func (w *folkWorld) attackedMonster(frames [][]byte, h *npc.Hostile) bool {
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeMoveToPawn && frame[0] != serverpackets.OpcodeAttack {
			continue
		}
		r := wire.NewReader(frame[1:])
		if r.ReadInt32() == w.player && r.ReadInt32() == h.ObjectID() {
			return true
		}
	}
	return false
}

// expectRefusedPlainClick plain-clicks the selected monster h and checks it
// is answered ActionFailed alone, with the player held in place.
func (w *folkWorld) expectRefusedPlainClick(t *testing.T, h *npc.Hostile, what string) {
	t.Helper()
	w.c.Send(encodeAction(h.ObjectID(), w.at, false))
	w.srv.Settle(t)
	if got := opcodes(drainFrames(t, w.c)); string(got) != string([]byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("%s = opcodes %x, want ActionFailed alone", what, got)
	}
	if x, y, _ := w.srv.PlayerPosition(t, w.player); x != w.at.X || y != w.at.Y {
		t.Fatalf("%s moved the player to (%d,%d), want it held at (%d,%d)", what, x, y, w.at.X, w.at.Y)
	}
}

func TestConfusedPlainClickAttacksSelectedMonster(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil)
	h := w.selectedMonster(t)
	w.afflict(t, "Confusion")

	w.c.Send(encodeAction(h.ObjectID(), w.at, false))
	w.srv.Settle(t)
	if frames := drainFrames(t, w.c); !w.attackedMonster(frames, h) {
		t.Fatalf("confused plain click on the selected monster = opcodes %x, want the attack walk or swing", opcodes(frames))
	}
}

func TestDeniedPlainClickOnSelectedMonsterRefused(t *testing.T) {
	t.Parallel()

	t.Run("stunned", func(t *testing.T) {
		t.Parallel()
		w := bootFolkWorld(t, nil)
		h := w.selectedMonster(t)
		w.afflict(t, "Stun")
		w.expectRefusedPlainClick(t, h, "stunned plain click on the selected monster")
	})

	t.Run("store mode", func(t *testing.T) {
		t.Parallel()
		w := bootFolkWorld(t, nil)
		h := w.selectedMonster(t)
		w.srv.SetPlayerOperateType(t, w.player, privatestore.OperateSell)
		w.expectRefusedPlainClick(t, h, "store-mode plain click on the selected monster")
	})
}

// TestConfusedPlayerAttacksOnTargetMe lands TargetMe from the monster the
// confused player already targets (EffectTargetMe.onStart: tryToAttack on
// the effector): the player attacks it.
func TestConfusedPlayerAttacksOnTargetMe(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil)
	h := w.selectedMonster(t)
	w.afflict(t, "Confusion")

	w.onPlayer(t, func(pc *player.Character) {
		e, err := effect.New(effect.Skill{ID: 28, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: "TargetMe", Time: 1})
		if err != nil {
			t.Errorf("effect.New(TargetMe): %v", err)
			return
		}
		e.Effector, e.Effected = h, pc
		pc.EffectList().Add(e)
	})
	w.srv.Settle(t)
	if frames := drainFrames(t, w.c); !w.attackedMonster(frames, h) {
		t.Fatalf("TargetMe on a confused player = opcodes %x, want the attack walk or swing on the effector", opcodes(frames))
	}
}
