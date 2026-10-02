package petition

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Access levels as the shipped accessLevels.xml defines them: 7 plays as a
// game master.
const (
	userLevel  = 0
	adminLevel = 7
)

// Chat channels, as the client numbers them.
const (
	sayPetitionPlayer int32 = 6
	sayPetitionGM     int32 = 7
	sayHeroVoice      int32 = 17
)

// Every seeded character enters the world at the class template's single
// spawn point.
const (
	spawnX = 10
	spawnY = 20
	spawnZ = 30
)

// rig is a booted server with a game master, "Admin", and a player,
// "Player", dialed but not yet in the world.
type rig struct {
	srv      *gameservertest.Server
	gm       *testsupport.ScriptedClient
	gmID     int32
	player   *testsupport.ScriptedClient
	playerID int32
}

func boot(t *testing.T, opts ...gameservertest.Option) *rig {
	t.Helper()
	admin, err := gamexml.LoadAdminData(datapack.Path(t, "data", "xml"))
	if err != nil {
		t.Fatalf("load admin data: %v", err)
	}
	pages := map[string]string{}
	for _, name := range []string{"petitions.htm", "petition.htm"} {
		raw, err := os.ReadFile(datapack.Path(t, "data", "html", "admin", name))
		if err != nil {
			t.Fatalf("read admin page %s: %v", name, err)
		}
		pages["admin/"+name] = string(raw)
	}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Admin", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAdmin(admin),
		gameservertest.WithHTMLPages(pages),
		gameservertest.WithReuseDelays(0, 0),
	}, opts...)...)
	r := &rig{srv: srv, gm: srv.Client, gmID: srv.SoleObjectID(t)}
	r.setAccessLevel(t, r.gmID, adminLevel)
	r.playerID = srv.SeedCharacterFor(t, "player2", "Player", 1, 0).ID
	r.player = srv.DialClient(t, "player2", 1)
	return r
}

func (r *rig) setAccessLevel(t *testing.T, objID int32, level int) {
	t.Helper()
	if _, err := r.srv.DB.ExecContext(context.Background(), "UPDATE characters SET accesslevel = ? WHERE obj_Id = ?", level, objID); err != nil {
		t.Fatalf("set access level: %v", err)
	}
}

// enterAll brings the game master and the player into the world.
func (r *rig) enterAll(t *testing.T) {
	t.Helper()
	enterWorld(t, r.gm)
	enterWorld(t, r.player)
	drain(t, r.gm)
	drain(t, r.player)
}

// addPlayer seeds, dials and enters a third character on its own account.
func (r *rig) addPlayer(t *testing.T, account, name string) (*testsupport.ScriptedClient, int32) {
	t.Helper()
	id := r.srv.SeedCharacterFor(t, account, name, 1, 0).ID
	c := r.srv.DialClient(t, account, 1)
	enterWorld(t, c)
	drain(t, r.gm)
	drain(t, r.player)
	return c, id
}

// submit sends the player's petition and returns its id, draining the
// answers.
func (r *rig) submit(t *testing.T) int32 {
	t.Helper()
	frames := exchange(t, r.player, encodePetition("help", 3))
	id, params := systemMessage(t, frames[0])
	if id != serverpackets.SystemMessagePetitionAcceptedRecentNoS1 {
		t.Fatalf("submit answer = %d, want PETITION_ACCEPTED_RECENT_NO_S1", id)
	}
	collect(t, r.gm)
	return params[0].(int32)
}

// enterWorld selects slot 0 and enters the world, returning every frame
// the server sent until it went quiet.
func enterWorld(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGameStart)
	w.WriteInt32(0)
	w.WriteUint16(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	c.Send(w.Bytes())
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	return drain(t, c)
}

// restart takes the client's character back to character selection and
// waits until it has left the world.
func (r *rig) restart(t *testing.T, c *testsupport.ScriptedClient, objID int32) {
	t.Helper()
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestRestart).Bytes())
	if frame := c.Read(); frame[0] != serverpackets.OpcodeRestartResponse {
		t.Fatalf("restart answer opcode = %#x, want RestartResponse", frame[0])
	}
	r.srv.AdvanceUntil(t, "player leaves the world", func() bool {
		_, ok := r.srv.State.Player(objID)
		return !ok
	})
	drain(t, c)
}

// drain reads until the client has received nothing for 300ms and returns
// what it read.
func drain(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	var frames [][]byte
	for range 400 {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			return frames
		}
		frames = append(frames, frame)
	}
	t.Fatal("client kept receiving frames after 400 reads")
	return nil
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

// collect returns every frame c was sent before a manor-list barrier: what
// another client's request sent it.
func collect(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	return testsupport.SyncBarrierFrames(t, c, func() { c.Send(encodeManorBarrier()) }, serverpackets.OpcodeExtended)
}

func encodeManorBarrier() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestManorList)
	return w.Bytes()
}

func encodePetition(content string, typ int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestPetition)
	w.WriteString(content)
	w.WriteInt32(typ)
	return w.Bytes()
}

func encodePetitionCancel() []byte {
	return wire.NewPacketWriter(clientpackets.OpcodeRequestPetitionCancel).Bytes()
}

func encodeVote(rate int32, feedback string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodePetitionVote)
	w.WriteInt32(1)
	w.WriteInt32(rate)
	w.WriteString(feedback)
	return w.Bytes()
}

func encodeSay2(typ int32, text string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeSay2)
	w.WriteString(text)
	w.WriteInt32(typ)
	return w.Bytes()
}

func encodeBypass(command string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestBypassToServer)
	w.WriteString(command)
	return w.Bytes()
}

func encodeBuildCmd(command string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeSendBypassBuildCmd)
	w.WriteString(command)
	return w.Bytes()
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

// systemMessage returns a SystemMessage frame's id and parameters: a
// string for a text parameter, an int32 for a number.
func systemMessage(t *testing.T, frame []byte) (int, []any) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("opcode = %#x, want SystemMessage", frame[0])
	}
	r := wire.NewReader(frame[1:])
	id := int(r.ReadInt32())
	n := int(r.ReadInt32())
	params := make([]any, 0, n)
	for range n {
		switch typ := r.ReadInt32(); typ {
		case serverpackets.SystemMessageParamText:
			params = append(params, r.ReadString())
		case serverpackets.SystemMessageParamNumber:
			params = append(params, r.ReadInt32())
		default:
			t.Fatalf("system message %d parameter type %d", id, typ)
		}
	}
	return id, params
}

// say is one CreatureSay frame.
type say struct {
	objectID int32
	channel  int32
	name     string
	text     string
}

func creatureSay(t *testing.T, frame []byte) say {
	t.Helper()
	if frame[0] != serverpackets.OpcodeCreatureSay {
		t.Fatalf("opcode = %#x, want CreatureSay", frame[0])
	}
	r := wire.NewReader(frame[1:])
	return say{r.ReadInt32(), r.ReadInt32(), r.ReadString(), r.ReadString()}
}

// message is a system message expected on the wire: its id and
// parameters.
type message struct {
	id     int
	params []any
}

func msg(id int, params ...any) message { return message{id, params} }

// assertMessages requires frames to be exactly the system messages want.
func assertMessages(t *testing.T, frames [][]byte, want ...message) {
	t.Helper()
	if len(frames) != len(want) {
		t.Fatalf("frames = %x, want %d system message(s) %v", testsupport.FrameOpcodes(frames), len(want), want)
	}
	for i, frame := range frames {
		id, params := systemMessage(t, frame)
		if id != want[i].id || len(params) != len(want[i].params) {
			t.Fatalf("message %d = %d %v, want %d %v", i, id, params, want[i].id, want[i].params)
		}
		for j := range params {
			if params[j] != want[i].params[j] {
				t.Fatalf("message %d = %d %v, want %d %v", i, id, params, want[i].id, want[i].params)
			}
		}
	}
}

// assertSays requires frames to be exactly the chat lines want.
func assertSays(t *testing.T, frames [][]byte, want ...say) {
	t.Helper()
	if len(frames) != len(want) {
		t.Fatalf("frames = %x, want %d CreatureSay(s) %+v", testsupport.FrameOpcodes(frames), len(want), want)
	}
	for i, frame := range frames {
		if got := creatureSay(t, frame); got != want[i] {
			t.Fatalf("line %d = %+v, want %+v", i, got, want[i])
		}
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

// assertPage requires frame to be an admin page containing every marker.
func assertPage(t *testing.T, frame []byte, markers ...string) string {
	t.Helper()
	body := htmlBody(t, frame)
	for _, marker := range markers {
		if !strings.Contains(body, marker) {
			t.Fatalf("page = %q, want one containing %q", body, marker)
		}
	}
	return body
}

func itoa(n int32) string { return strconv.Itoa(int(n)) }
