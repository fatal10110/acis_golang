package admin

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Every seeded character enters the world at the class template's single
// spawn point.
const (
	spawnX = 10
	spawnY = 20
	spawnZ = 30
)

// Access levels as the shipped accessLevels.xml defines them.
const (
	userLevel   = 0
	adminLevel  = 7
	masterLevel = 8
)

// shippedAdminData loads the datapack's accessLevels.xml and
// adminCommands.xml, the tables the access checks are pinned against.
func shippedAdminData(t *testing.T) *admin.Data {
	t.Helper()
	data, err := gamexml.LoadAdminData(datapack.Path(t, "data", "xml"))
	if err != nil {
		t.Fatalf("load admin data: %v", err)
	}
	return data
}

// shippedAdminPages returns the datapack admin panel pages names.
func shippedAdminPages(t *testing.T, names ...string) map[string]string {
	t.Helper()
	pages := make(map[string]string, len(names))
	for _, name := range names {
		raw, err := os.ReadFile(datapack.Path(t, "data", "html", "admin", name))
		if err != nil {
			t.Fatalf("read admin page %s: %v", name, err)
		}
		pages["admin/"+name] = string(raw)
	}
	return pages
}

// bootAdmin boots one character, "Admin", at the given access level with
// the shipped admin tables and panel pages, and returns the server and the
// character's object id. The character is not in the world yet.
func bootAdmin(t *testing.T, level int, opts ...gameservertest.Option) (*gameservertest.Server, int32) {
	t.Helper()
	opts = append([]gameservertest.Option{
		gameservertest.WithCharacter("Admin", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAdmin(shippedAdminData(t)),
		gameservertest.WithHTMLPages(shippedAdminPages(t, "main_menu.htm", "game_menu.htm", "teleports.htm")),
		// Admin links are clicked back to back; the bypass reuse delay
		// would drop all but the first.
		gameservertest.WithReuseDelays(3*time.Second, 0),
	}, opts...)
	srv := gameservertest.Boot(t, opts...)
	objID := srv.SoleObjectID(t)
	setAccessLevel(t, srv, objID, level)
	return srv, objID
}

func setAccessLevel(t *testing.T, srv *gameservertest.Server, objID int32, level int) {
	t.Helper()
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET accesslevel = ? WHERE obj_Id = ?", level, objID); err != nil {
		t.Fatalf("set access level: %v", err)
	}
}

// addPlayer seeds a second account's character at the given access level,
// dials it and enters the world, returning its client and object id.
func addPlayer(t *testing.T, srv *gameservertest.Server, account, name string, level int) (*testsupport.ScriptedClient, int32) {
	t.Helper()
	ch := srv.SeedCharacterFor(t, account, name, 1, 0)
	setAccessLevel(t, srv, ch.ID, level)
	c := srv.DialClient(t, account, 1)
	enterWorld(t, c)
	return c, ch.ID
}

// enterWorld selects slot 0 and enters the world, reading until the stream
// is quiet.
func enterWorld(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGameStart)
	w.WriteInt32(0)
	w.WriteUint16(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	c.Send(w.Bytes())
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	drain(t, c)
}

// drain reads until the client has received nothing for 300ms.
func drain(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	for range 200 {
		if c.ReadWithTimeout(300*time.Millisecond) == nil {
			return
		}
	}
	t.Fatal("client kept receiving frames after 200 drains")
}

func encodeBuildCmd(command string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeSendBypassBuildCmd)
	w.WriteString(command)
	return w.Bytes()
}

func encodeBypass(command string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestBypassToServer)
	w.WriteString(command)
	return w.Bytes()
}

func encodeGmList() []byte {
	return wire.NewPacketWriter(clientpackets.OpcodeRequestGmList).Bytes()
}

func encodeAction(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAction)
	w.WriteInt32(objectID)
	w.WriteInt32(spawnX)
	w.WriteInt32(spawnY)
	w.WriteInt32(spawnZ)
	w.WriteUint8(0)
	return w.Bytes()
}

func encodeMove(x, y, z int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeMoveBackwardToLocation)
	w.WriteInt32(x)
	w.WriteInt32(y)
	w.WriteInt32(z)
	w.WriteInt32(spawnX)
	w.WriteInt32(spawnY)
	w.WriteInt32(spawnZ)
	w.WriteInt32(1)
	return w.Bytes()
}

func encodeManorBarrier() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestManorList)
	return w.Bytes()
}

// exchange sends payload and returns every frame it drew, read up to the
// manor-list barrier sent after it.
func exchange(t *testing.T, c *testsupport.ScriptedClient, payload []byte) [][]byte {
	t.Helper()
	return testsupport.SyncBarrierFrames(t, c, func() {
		c.Send(payload)
		c.Send(encodeManorBarrier())
	}, serverpackets.OpcodeExtended)
}

// systemText returns a one-text-parameter SystemMessage frame's id and text.
func systemText(t *testing.T, frame []byte) (int, string) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("opcode = %#x, want SystemMessage", frame[0])
	}
	r := wire.NewReader(frame[1:])
	id := int(r.ReadInt32())
	if n := r.ReadInt32(); n != 1 {
		t.Fatalf("system message %d params = %d, want 1", id, n)
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamText {
		t.Fatalf("system message %d param type = %d, want text", id, typ)
	}
	return id, r.ReadString()
}

// assertTexts requires frames to be exactly the plain-text messages want.
func assertTexts(t *testing.T, frames [][]byte, want ...string) {
	t.Helper()
	if len(frames) != len(want) {
		t.Fatalf("frames = %x, want %d text message(s) %q", testsupport.FrameOpcodes(frames), len(want), want)
	}
	for i, frame := range frames {
		id, text := systemText(t, frame)
		if id != serverpackets.SystemMessageS1 || text != want[i] {
			t.Fatalf("message %d = %d %q, want S1 %q", i, id, text, want[i])
		}
	}
}

// assertStatic requires frame to be the parameterless system message id.
func assertStatic(t *testing.T, frame []byte, id int) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("opcode = %#x, want SystemMessage %d", frame[0], id)
	}
	r := wire.NewReader(frame[1:])
	if got, n := r.ReadInt32(), r.ReadInt32(); int(got) != id || n != 0 {
		t.Fatalf("system message = %d with %d params, want %d with none", got, n, id)
	}
}

// htmlBody returns an NpcHtmlMessage frame's page.
func htmlBody(t *testing.T, frame []byte) string {
	t.Helper()
	if frame[0] != serverpackets.OpcodeNpcHtmlMessage {
		t.Fatalf("opcode = %#x, want NpcHtmlMessage", frame[0])
	}
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != 0 {
		t.Fatalf("NpcHtmlMessage object id = %d, want 0", id)
	}
	return r.ReadString()
}

// assertPage requires frames to be the one admin page whose text contains
// marker.
func assertPage(t *testing.T, frames [][]byte, marker string) {
	t.Helper()
	if len(frames) != 1 {
		t.Fatalf("frames = %x, want one NpcHtmlMessage", testsupport.FrameOpcodes(frames))
	}
	if body := htmlBody(t, frames[0]); !strings.Contains(body, marker) {
		t.Fatalf("page = %q, want one containing %q", body, marker)
	}
}

// teleportTo returns a TeleportToLocation frame's object id and point.
func teleportTo(t *testing.T, frame []byte) (int32, [3]int32) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeTeleportToLocation {
		t.Fatalf("opcode = %#x, want TeleportToLocation", frame[0])
	}
	r := wire.NewReader(frame[1:])
	id := r.ReadInt32()
	return id, [3]int32{r.ReadInt32(), r.ReadInt32(), r.ReadInt32()}
}
