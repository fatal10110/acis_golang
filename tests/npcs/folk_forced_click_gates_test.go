package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// holyArtifactID is the template id the holy artifact fixture is built
// with; nothing reads it but the spawn.
const holyArtifactID = 35063

// TestConfusedForcedClickOnFolkCorpseRefused pins AttackRequest.runImpl
// refusing an out-of-control player before target.onAction: a confused
// player (Creature.isOutOfControl includes isConfused, denyAiAction does
// not) forcing a click on a selected civilian NPC's corpse gets
// ActionFailed alone. It neither walks to the corpse nor opens its chat
// page.
func TestConfusedForcedClickOnFolkCorpseRefused(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, merchantPages())
	f := mortalMerchantCorpse(t, w)
	w.onPlayer(t, func(pc *player.Character) {
		e, err := effect.New(
			effect.Skill{ID: 2, Level: 1, Debuff: true},
			modelskill.EffectTemplate{Name: "Confusion", Time: 30},
		)
		if err != nil {
			t.Errorf("effect.New(Confusion): %v", err)
			return
		}
		e.Effector, e.Effected = pc, pc
		pc.EffectList().Add(e)
	})
	drainUntilQuiet(t, w.c)

	w.c.Send(encodeAttackRequest(f.ObjectID(), w.at))
	w.srv.Settle(t)
	frames := drainFrames(t, w.c)
	if order := interactOrder(frames); string(order) != string([]byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("confused forced click on the corpse = opcodes %x, want ActionFailed alone", opcodes(frames))
	}
	if _, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage); ok {
		t.Fatalf("confused forced click opened the chat page: opcodes %x", opcodes(frames))
	}
	if x, y, _ := w.srv.PlayerPosition(t, w.player); x != w.at.X || y != w.at.Y {
		t.Fatalf("confused player moved to (%d,%d), want it held at (%d,%d)", x, y, w.at.X, w.at.Y)
	}
}

// TestForcedClickOnHolyArtifactWalksWithoutChat pins Creature.onAction for
// a castle holy artifact: HolyThing.isAttackableBy is false, so a forced
// click on the selected artifact is an interact, not an attack. The player
// swings at nothing and is shown facing it, and HolyThing.onInteract opens
// no chat page.
func TestForcedClickOnHolyArtifactWalksWithoutChat(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, merchantPages())
	f := w.spawnFolk(t, folkTemplate("HolyThing", holyArtifactID), 50)
	w.selectFolk(t, f)

	w.c.Send(encodeAttackRequest(f.ObjectID(), w.at))
	w.srv.Settle(t)
	frames := drainFrames(t, w.c)
	for _, frame := range frames {
		if objectFrame(frame, serverpackets.OpcodeAttack, w.player) {
			t.Fatalf("forced click swung at the artifact: opcodes %x", opcodes(frames))
		}
	}
	frame, ok := firstOpcode(frames, serverpackets.OpcodeMoveToPawn)
	if !ok {
		t.Fatalf("forced click on the artifact sent no MoveToPawn: opcodes %x", opcodes(frames))
	}
	if mover, target, distance := moveToPawn(t, frame); mover != w.player || target != f.ObjectID() || distance != 150 {
		t.Fatalf("MoveToPawn = %d->%d at %d, want %d->%d at 150", mover, target, distance, w.player, f.ObjectID())
	}
	if _, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage); ok {
		t.Fatalf("forced click on the artifact opened a chat page: opcodes %x", opcodes(frames))
	}
}
