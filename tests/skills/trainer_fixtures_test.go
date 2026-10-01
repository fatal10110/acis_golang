package skills

import (
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// trainerNpcID is the fixture trainer's template id; its no-skills page is
// trainer/<trainerNpcID>-noskills.htm.
const trainerNpcID = 30010

// spawnTrainer places a civilian NPC of kind teaching the given
// professions dx units east of the player and drains its NpcInfo.
func spawnTrainer(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, objID int32, kind string, dx int, teachTo ...int) *npc.Folk {
	t.Helper()
	tmpl := gameservertest.FolkTemplate(kind, trainerNpcID)
	tmpl.TeachTo = teachTo
	x, y, z := srv.PlayerPosition(t, objID)
	f := srv.SpawnFolkNPCAt(t, tmpl, location.Location{X: x + dx, Y: y, Z: z})
	drainUntilQuiet(t, c)
	return f
}

// selectTrainer spawns a trainer of the given professions next to the
// player, selects it with one click and drains the selection: it becomes
// the NPC the player's learn and enchant requests are made at.
func selectTrainer(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, objID int32, teachTo ...int) *npc.Folk {
	t.Helper()
	f := spawnTrainer(t, srv, c, objID, "Trainer", 50, teachTo...)
	selectNpc(t, srv, c, objID, f)
	return f
}

// selectNpc clicks f once and drains the selection.
func selectNpc(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, objID int32, f *npc.Folk) {
	t.Helper()
	x, y, z := srv.PlayerPosition(t, objID)
	c.Send(encodeAction(f.ObjectID(), int32(x), int32(y), int32(z), false))
	drainUntilQuiet(t, c)
}

// anyNpcLinkPage admits every npc_ bypass command: its one link takes the
// whole command after npc_ from an edit box.
const anyNpcLinkPage = `<html><body><edit var="c"><a action="bypass -h npc_$c">Go</a></body></html>`

func encodeLinkHTML(link string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestLinkHtml)
	w.WriteString(link)
	return w.Bytes()
}

func encodeBypass(command string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestBypassToServer)
	w.WriteString(command)
	return w.Bytes()
}

// trainerBypass opens the page admitting every npc_ command, then sends
// npc_<f>_<command> and returns every frame it is answered with.
func trainerBypass(t *testing.T, c *testsupport.ScriptedClient, f *npc.Folk, command string) [][]byte {
	t.Helper()
	c.Send(encodeLinkHTML("test/any.htm"))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeNpcHtmlMessage, "any-command page")
	drainUntilQuiet(t, c)
	c.Send(encodeBypass("npc_" + strconv.Itoa(int(f.ObjectID())) + "_" + command))
	return readUntilQuiet(t, c)
}

// readUntilQuiet collects every frame the client receives until the server
// stays quiet for a full read timeout, in arrival order.
func readUntilQuiet(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	var frames [][]byte
	for range 100 {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			return frames
		}
		frames = append(frames, frame)
	}
	t.Fatal("client kept receiving frames after 100 drains")
	return nil
}

func frameOpcodes(frames [][]byte) []byte {
	out := make([]byte, 0, len(frames))
	for _, f := range frames {
		out = append(out, f[0])
	}
	return out
}

// assertNumberSystemMessage asserts frame is a SystemMessage carrying one
// number parameter.
func assertNumberSystemMessage(t *testing.T, frame []byte, messageID int, number int32) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "SystemMessage")
	r := wire.NewReader(frame[1:])
	if id, params, typ, got := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id != int32(messageID) || params != 1 || typ != serverpackets.SystemMessageParamNumber || got != number {
		t.Fatalf("SystemMessage = %d/%d/%d/%d, want %d/1/number/%d", id, params, typ, got, messageID, number)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SystemMessage: %v", err)
	}
}

// assertNpcHTML asserts frame is an NpcHtmlMessage from objectID carrying
// html.
func assertNpcHTML(t *testing.T, frame []byte, objectID int32, html string) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeNpcHtmlMessage, "NpcHtmlMessage")
	r := wire.NewReader(frame[1:])
	if id, got, item := r.ReadInt32(), r.ReadString(), r.ReadInt32(); id != objectID || got != html || item != 0 {
		t.Fatalf("NpcHtmlMessage = object %d item %d %q, want %d/0 %q", id, item, got, objectID, html)
	}
}

// assertEmptyListClose asserts the frames that close an empty learn list:
// the message naming why it is empty, AcquireSkillDone, then ActionFailed.
func assertEmptyListClose(t *testing.T, c *testsupport.ScriptedClient, messageID int) {
	t.Helper()
	assertStaticSystemMessage(t, c.Read(), messageID)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeAcquireSkillDone, "AcquireSkillDone")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "ActionFailed")
}
