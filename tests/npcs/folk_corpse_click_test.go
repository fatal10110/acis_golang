package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// corpseTalkOrder is a talk with a civilian NPC's corpse: a dead NPC plays
// no talk animation (NpcAI.onRandomAnimation refuses on denyAiAction).
var corpseTalkOrder = []byte{
	serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn,
	serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed,
}

// mortalMerchantCorpse spawns a merchant that is not undying beside the
// player, kills it and selects its corpse.
func mortalMerchantCorpse(t *testing.T, w *folkWorld) *npc.Folk {
	t.Helper()
	tmpl := folkTemplate("Merchant", merchantID)
	tmpl.Undying = false
	f := w.spawnFolk(t, tmpl, 50)
	if !f.Kill(nil) {
		t.Fatal("merchant did not die")
	}
	drainUntilQuiet(t, w.c)
	w.selectFolk(t, f)
	return f
}

// assertCorpseTalk checks that frames hold no swing and end with the talk
// that opens corpse's chat page.
func assertCorpseTalk(t *testing.T, w *folkWorld, corpse *npc.Folk, frames [][]byte, what string) {
	t.Helper()
	for _, frame := range frames {
		if objectFrame(frame, serverpackets.OpcodeAttack, w.player) {
			t.Fatalf("%s swung at the corpse: opcodes %x", what, opcodes(frames))
		}
	}
	order := interactOrder(frames)
	if len(order) < len(corpseTalkOrder) || string(order[len(order)-len(corpseTalkOrder):]) != string(corpseTalkOrder) {
		t.Fatalf("%s talk order = %x, want it to end %x (all %x)", what, order, corpseTalkOrder, opcodes(frames))
	}
	frame, _ := firstOpcode(frames, serverpackets.OpcodeMoveToPawn)
	if mover, target, distance := moveToPawn(t, frame); mover != w.player || target != corpse.ObjectID() || distance != 150 {
		t.Fatalf("%s MoveToPawn = %d->%d at %d, want %d->%d at 150", what, mover, target, distance, w.player, corpse.ObjectID())
	}
	frame, _ = firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
	if objectID, html, _ := htmlMessage(t, frame); objectID != corpse.ObjectID() || html != wantChatPage(merchantPage, corpse) {
		t.Fatalf("%s NpcHtmlMessage = object %d html %q, want %d %q", what, objectID, html, corpse.ObjectID(), wantChatPage(merchantPage, corpse))
	}
}

// TestForcedClickOnFolkCorpseTalks pins Creature.onAction for a dead
// civilian NPC: a corpse is no longer attackable (Creature.isAttackableBy
// is !isDead), so a forced click on it runs the interact instead of the
// attack. The player swings at nothing and is shown facing the corpse, and
// the NPC's chat page opens.
func TestForcedClickOnFolkCorpseTalks(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, merchantPages())
	f := mortalMerchantCorpse(t, w)

	w.c.Send(encodeAttackRequest(f.ObjectID(), w.at))
	w.srv.Settle(t)
	assertCorpseTalk(t, w, f, drainFrames(t, w.c), "forced click")
}

// TestForcedClickOnFolkCorpseDuringStandUpTalks pins the same choice for a
// click queued during a stand-up (PlayerAI.onEvtStoodUp running the next
// intention): the forced click on the corpse is answered ActionFailed alone
// and queues the talk, not an attack, which runs once the player stands.
func TestForcedClickOnFolkCorpseDuringStandUpTalks(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, merchantPages())
	f := mortalMerchantCorpse(t, w)

	w.c.Send(encodeRequestChangeWaitType(false))
	readUntilOpcode(t, w.c, serverpackets.OpcodeChangeWaitType, "sit")
	w.srv.SettlePosture(t, w.player)
	drainUntilQuiet(t, w.c)
	w.c.Send(encodeRequestChangeWaitType(true))
	readUntilOpcode(t, w.c, serverpackets.OpcodeChangeWaitType, "stand")
	w.c.Send(encodeAttackRequest(f.ObjectID(), w.at))
	queued := w.srv.ReadQueued(t, w.c)
	if order := interactOrder(queued); string(order) != string([]byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("forced click during stand-up = opcodes %x, want ActionFailed alone", opcodes(queued))
	}

	w.srv.SettlePosture(t, w.player)
	frames := readUntilOpcode(t, w.c, serverpackets.OpcodeNpcHtmlMessage, "chat window after stand-up")
	frames = append(frames, drainFrames(t, w.c)...)
	assertCorpseTalk(t, w, f, frames, "forced click after stand-up")
}
