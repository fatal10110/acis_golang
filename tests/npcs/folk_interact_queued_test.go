package npcs

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// longCastSkillID is a five-second self cast to be mid-cast with.
const longCastSkillID = 3

// talkOrder is the full answer to a talk in reach.
var talkOrder = []byte{
	serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodeSocialAction,
	serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed,
}

func encodeRequestChangeWaitType(stand bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestChangeWaitType)
	w.WriteInt32(wire.BoolInt32(stand))
	return w.Bytes()
}

func encodeRequestMagicSkillUse(skillID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestMagicSkillUse)
	w.WriteInt32(skillID)
	w.WriteInt32(0)
	w.WriteUint8(0)
	return w.Bytes()
}

// readImmediate collects the frames already on their way to c, letting at
// most a millisecond pass on the driven clock: nothing a swing, cast or
// posture change in flight schedules comes due meanwhile.
func readImmediate(c *testsupport.ScriptedClient) [][]byte {
	var frames [][]byte
	for frame := c.ReadWithTimeout(time.Millisecond); frame != nil; frame = c.ReadWithTimeout(time.Millisecond) {
		frames = append(frames, frame)
	}
	return frames
}

// readUntilOpcode reads frames up to and including the first want.
func readUntilOpcode(t *testing.T, c *testsupport.ScriptedClient, want byte, what string) [][]byte {
	t.Helper()
	frames := make([][]byte, 0, 4)
	for i := 0; i < 100; i++ {
		frame := c.ReadWithTimeout(5 * time.Second)
		if frame == nil {
			t.Fatalf("%s never arrived (read %x)", what, opcodes(frames))
		}
		frames = append(frames, frame)
		if frame[0] == want {
			return frames
		}
	}
	t.Fatalf("%s not found within 100 frames", what)
	return nil
}

// isTalkFrame reports whether frame is where the player's talk with f
// shows: the player's MoveToPawn toward it, f's talk animation, or a chat
// window.
func isTalkFrame(frame []byte, playerID int32, f *npc.Folk) bool {
	switch frame[0] {
	case serverpackets.OpcodeNpcHtmlMessage:
		return true
	case serverpackets.OpcodeSocialAction:
		return wire.NewReader(frame[1:]).ReadInt32() == f.ObjectID()
	case serverpackets.OpcodeMoveToPawn:
		r := wire.NewReader(frame[1:])
		return r.ReadInt32() == playerID && r.ReadInt32() == f.ObjectID()
	}
	return false
}

// clickFolkQueued clicks the already-selected f and checks the click is
// answered with ActionFailed alone while the action in flight holds it.
func (w *folkWorld) clickFolkQueued(t *testing.T, f *npc.Folk, what string) {
	t.Helper()
	w.c.Send(encodeAction(f.ObjectID(), w.at, false))
	frames := readImmediate(w.c)
	if _, ok := firstOpcode(frames, serverpackets.OpcodeActionFailed); !ok {
		t.Fatalf("folk click %s = opcodes %x, want ActionFailed", what, opcodes(frames))
	}
	for _, frame := range frames {
		if isTalkFrame(frame, w.player, f) {
			t.Fatalf("folk click %s talked at once: opcodes %x", what, opcodes(frames))
		}
	}
}

// assertQueuedTalk checks frames, read from the moment the action holding
// the talk ended, end with the full talk answer and hold no talk frame
// before it.
func assertQueuedTalk(t *testing.T, frames [][]byte, what string) {
	t.Helper()
	order := interactOrder(frames)
	if len(order) < len(talkOrder) || string(order[len(order)-len(talkOrder):]) != string(talkOrder) {
		t.Fatalf("%s talk order = %x, want it to end %x (all %x)", what, order, talkOrder, opcodes(frames))
	}
	var htmls int
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeNpcHtmlMessage {
			htmls++
		}
	}
	if htmls != 1 {
		t.Fatalf("%s opened %d chat windows, want 1", what, htmls)
	}
}

// TestFolkTalkWaitsForStandUp pins a talk clicked while the player stands
// up (PlayerAI.onEvtStoodUp running the next intention): the click is
// answered ActionFailed alone, and the talk runs once the player stands.
func TestFolkTalkWaitsForStandUp(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, merchantPages())
	f := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 50)
	w.selectFolk(t, f)

	w.c.Send(encodeRequestChangeWaitType(false))
	readUntilOpcode(t, w.c, serverpackets.OpcodeChangeWaitType, "sit")
	w.srv.SettlePosture(t, w.player)
	drainUntilQuiet(t, w.c)
	w.c.Send(encodeRequestChangeWaitType(true))
	readUntilOpcode(t, w.c, serverpackets.OpcodeChangeWaitType, "stand")
	w.clickFolkQueued(t, f, "during stand-up")

	w.srv.SettlePosture(t, w.player)
	frames := readUntilOpcode(t, w.c, serverpackets.OpcodeNpcHtmlMessage, "chat window after stand-up")
	frames = append(frames, drainFrames(t, w.c)...)
	assertQueuedTalk(t, frames, "stand-up")
	frame, _ := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
	if objectID, html, _ := htmlMessage(t, frame); objectID != f.ObjectID() || html != wantChatPage(merchantPage, f) {
		t.Fatalf("NpcHtmlMessage = object %d html %q, want %d %q", objectID, html, f.ObjectID(), wantChatPage(merchantPage, f))
	}
}

// TestFolkTalkMidSwingRunsAtSwingEnd pins PlayableAI.tryToInteract for a
// swing in flight: the click on the selected NPC is answered ActionFailed
// and kept as the next intention, and the swing's end runs the talk in
// place of the attack, which does not swing again.
func TestFolkTalkMidSwingRunsAtSwingEnd(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, merchantPages())
	if !w.srv.DrivesClock() {
		t.Skip("holding a swing open needs the driven clock")
	}
	f := w.spawnFolk(t, folkTemplate("Merchant", merchantID), -50)
	hostile := w.srv.SpawnHostileNPCAt(t, location.Location{X: w.at.X + 30, Y: w.at.Y, Z: w.at.Z})
	drainUntilQuiet(t, w.c)

	w.c.Send(encodeAction(hostile.ObjectID(), w.at, false))
	drainUntilQuiet(t, w.c)
	w.c.Send(encodeAction(hostile.ObjectID(), w.at, false))
	readUntilOwnSwing(t, w)

	// Selecting the NPC leaves the swing and the attack running.
	w.c.Send(encodeAction(f.ObjectID(), w.at, false))
	readImmediate(w.c)
	w.clickFolkQueued(t, f, "mid-swing")

	frames := readUntilOpcode(t, w.c, serverpackets.OpcodeNpcHtmlMessage, "chat window after the swing")
	for end := w.c.Now().Add(3 * time.Second); w.c.Now().Before(end); {
		frame := w.c.ReadWithTimeout(end.Sub(w.c.Now()))
		if frame == nil {
			break
		}
		frames = append(frames, frame)
	}
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeAttack && wire.NewReader(frame[1:]).ReadInt32() == w.player {
			t.Fatalf("the player swung again instead of talking: %x", opcodes(frames))
		}
	}
	assertQueuedTalk(t, frames, "swing end")
}

// readUntilOwnSwing reads until the player's own Attack broadcast.
func readUntilOwnSwing(t *testing.T, w *folkWorld) {
	t.Helper()
	for i := 0; i < 100; i++ {
		frame := w.c.ReadWithTimeout(5 * time.Second)
		if frame == nil {
			t.Fatal("the player never swung")
		}
		if frame[0] == serverpackets.OpcodeAttack && wire.NewReader(frame[1:]).ReadInt32() == w.player {
			return
		}
	}
	t.Fatal("the player's swing not found within 100 frames")
}

// TestFolkTalkMidCastRunsAtCastEnd is the same for a cast in flight:
// PlayerAI.onEvtFinishedCasting runs the queued talk once the cast
// launches, and only then does the player face the NPC and get its page.
func TestFolkTalkMidCastRunsAtCastEnd(t *testing.T) {
	t.Parallel()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{{
		ID: longCastSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "DUMMY", StaticHitTime: true, HitTime: 5000, StaticReuse: true, ReuseDelay: 0,
	}}), gamesql.NewCharacterSkillStore(db))
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Talker", playerLevel, 0), gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(merchantPages()), gameservertest.WithSkills(skills))
	if !srv.DrivesClock() {
		t.Skip("holding a cast open needs the driven clock")
	}
	w := &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), w.player, 0, longCastSkillID, 1); err != nil {
		t.Fatalf("seed known skill: %v", err)
	}
	startInWorld(t, w.srv, w.c)
	x, y, z := srv.PlayerPosition(t, w.player)
	w.at = location.Location{X: x, Y: y, Z: z}
	f := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 50)
	w.selectFolk(t, f)

	w.c.Send(encodeRequestMagicSkillUse(longCastSkillID))
	readUntilOpcode(t, w.c, serverpackets.OpcodeMagicSkillUse, "long cast MagicSkillUse")
	readImmediate(w.c)
	if !srv.PlayerCastingNow(t, w.player) {
		t.Fatal("long cast not in flight")
	}
	w.clickFolkQueued(t, f, "mid-cast")

	frames := readUntilOpcode(t, w.c, serverpackets.OpcodeNpcHtmlMessage, "chat window after the cast")
	frames = append(frames, drainFrames(t, w.c)...)
	var order []byte
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeMagicSkillLaunched {
			order = append(order, frame[0])
		}
		order = append(order, interactOrder([][]byte{frame})...)
	}
	want := append([]byte{serverpackets.OpcodeMagicSkillLaunched}, talkOrder...)
	if string(order) != string(want) {
		t.Fatalf("cast end order = %x, want %x (all %x)", order, want, opcodes(frames))
	}
}

// TestFolkTalkOnBlockedArrival pins PlayerAI.onEvtArrivedBlocked for a
// talk: an approach walk stopped by geodata inside interaction distance
// releases the client, broadcasts StopMove, and the NPC talks.
func TestFolkTalkOnBlockedArrival(t *testing.T) {
	t.Parallel()
	geo := &gameservertest.GateGeo{}
	w := bootFolkWorld(t, merchantPages(), gameservertest.WithGeo(geo))
	f := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 130)
	w.selectFolk(t, f)

	frames := w.talk(t, f, false)
	if _, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage); ok {
		t.Fatalf("talk from 130 opened the page before the approach: %x", opcodes(frames))
	}
	if frame, ok := firstOpcode(frames, serverpackets.OpcodeMoveToPawn); !ok {
		t.Fatalf("talk from 130 did not approach: %x", opcodes(frames))
	} else if _, _, distance := moveToPawn(t, frame); distance != 100 {
		t.Fatalf("approach distance = %d, want 100", distance)
	}

	w.srv.TickPlayerBlocked(t, w.player, geo)
	frames = drainFrames(t, w.c)
	want := []byte{
		serverpackets.OpcodeActionFailed, serverpackets.OpcodeStopMove, serverpackets.OpcodeSocialAction,
		serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed,
	}
	if got := interactOrder(frames); string(got) != string(want) {
		t.Fatalf("blocked arrival order = %x, want %x (all %x)", got, want, opcodes(frames))
	}
	frame, _ := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
	if objectID, html, _ := htmlMessage(t, frame); objectID != f.ObjectID() || html != wantChatPage(merchantPage, f) {
		t.Fatalf("NpcHtmlMessage = object %d html %q, want %d %q", objectID, html, f.ObjectID(), wantChatPage(merchantPage, f))
	}
}
