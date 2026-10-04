package wedding

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/dbtest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/wedding"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

// weddingManagerID is the shipped wedding manager.
const weddingManagerID = 50001

// weddingPages are the shipped wedding manager pages.
var weddingPages = []string{
	"start.htm", "start2.htm", "waitforpartner.htm", "notfound.htm", "error_wrongtarget.htm",
	"error_sex.htm", "error_friendlist.htm", "error_alreadymarried.htm", "error_noformal.htm", "error_adena.htm",
}

// chapel is a booted server with Alice and Bobby in the world, both
// holding adena, beside a wedding manager.
type chapel struct {
	srv     *gameservertest.Server
	alice   *testsupport.ScriptedClient
	aliceID int32
	bobby   *testsupport.ScriptedClient
	bobbyID int32
	manager *npc.Folk
	at      location.Location
}

// chapelOptions set up a chapel.
type chapelOptions struct {
	cfg          wedding.Config
	bobbySex     player.Sex
	aliceAdena   int32
	bobbyAdena   int32
	friends      bool
	formalOnBoth bool
}

// defaultChapel is a man and a woman, friends, each holding twice the
// fixture price, with formal wear not required.
func defaultChapel() chapelOptions {
	return chapelOptions{
		cfg:        wedding.Config{Price: 1_000_000},
		bobbySex:   player.SexFemale,
		aliceAdena: 2_000_000,
		bobbyAdena: 2_000_000,
		friends:    true,
	}
}

// bootChapel boots the server with the shipped wedding pages and opts.
func bootChapel(t *testing.T, opts chapelOptions) *chapel {
	t.Helper()
	pages := map[string]string{}
	for _, name := range weddingPages {
		data, err := os.ReadFile(datapack.Path(t, "data", "html", "mods", "wedding", name))
		if err != nil {
			t.Fatal(err)
		}
		pages["mods/wedding/"+name] = string(data)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Alice", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(pages),
		gameservertest.WithWeddingConfig(opts.cfg),
		gameservertest.WithReuseDelays(0, 0),
	)
	c := &chapel{srv: srv, alice: srv.Client, aliceID: srv.SoleObjectID(t)}
	c.bobbyID = srv.SeedCharacterFor(t, "player2", "Bobby", 20, 0).ID
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET sex = ? WHERE obj_Id = ?", int(opts.bobbySex), c.bobbyID); err != nil {
		t.Fatal(err)
	}
	if opts.aliceAdena > 0 {
		srv.GiveItem(t, c.aliceID, item.AdenaID, opts.aliceAdena)
	}
	if opts.bobbyAdena > 0 {
		srv.GiveItem(t, c.bobbyID, item.AdenaID, opts.bobbyAdena)
	}
	var dresses [2]int32
	if opts.formalOnBoth {
		dresses[0] = srv.GiveItem(t, c.aliceID, gameservertest.FormalWearID, 1)
		dresses[1] = srv.GiveItem(t, c.bobbyID, gameservertest.FormalWearID, 1)
	}
	c.bobby = srv.DialClient(t, "player2", 1)
	enter(t, srv, c.alice)
	enter(t, srv, c.bobby)
	if opts.formalOnBoth {
		c.alice.Send(encodeUseItem(dresses[0]))
		c.bobby.Send(encodeUseItem(dresses[1]))
	}
	if opts.friends {
		srv.Relations.AddFriend(c.aliceID, c.bobbyID)
	}
	x, y, z := srv.PlayerPosition(t, c.aliceID)
	c.at = location.Location{X: x, Y: y, Z: z}
	c.manager = srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("WeddingManagerNpc", weddingManagerID), location.Location{X: x + 50, Y: y, Z: z})
	drainFrames(t, c.alice)
	drainFrames(t, c.bobby)
	return c
}

// enter takes c's character into the world and drains the burst.
func enter(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient) {
	t.Helper()
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGameStart)
	w.WriteInt32(0)
	w.WriteUint16(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	c.Send(w.Bytes())
	if reply := c.Read(); reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("opcode = %#x, want SSQInfo", reply[0])
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected", reply[0])
	}
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	srv.Settle(t)
	drainFrames(t, c)
}

// character is the online character objID.
func (c *chapel) character(t *testing.T, objID int32) *player.Character {
	t.Helper()
	p, ok := c.srv.State.Player(objID)
	if !ok {
		t.Fatalf("player %d is not online", objID)
	}
	ch, ok := network.OnlineCharacter(p)
	if !ok {
		t.Fatalf("player %d is not a character", objID)
	}
	return ch
}

// talk has client select the manager, then talk to it, and returns the
// talk's frames.
func (c *chapel) talk(t *testing.T, client *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	client.Send(encodeAction(c.manager.ObjectID(), c.at))
	drainFrames(t, client)
	client.Send(encodeAction(c.manager.ObjectID(), c.at))
	return drainFrames(t, client)
}

// bypass sends the manager command from client and returns its answer.
func (c *chapel) bypass(t *testing.T, client *testsupport.ScriptedClient, command string) [][]byte {
	t.Helper()
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestBypassToServer)
	w.WriteString("npc_" + strconv.Itoa(int(c.manager.ObjectID())) + "_" + command)
	client.Send(w.Bytes())
	return drainFrames(t, client)
}

// answer sends client's answer to the marriage request dialog.
func answer(t *testing.T, client *testsupport.ScriptedClient, yes bool) {
	t.Helper()
	w := wire.NewPacketWriter(clientpackets.OpcodeDlgAnswer)
	w.WriteInt32(serverpackets.ConfirmDlgEngageRequest)
	w.WriteInt32(wire.BoolInt32(yes))
	w.WriteInt32(0)
	client.Send(w.Bytes())
}

// page is the shipped wedding page name as the page cache holds it, filled
// for the manager with the price and formal wear words.
func (c *chapel) page(t *testing.T, name, price, needOrNot string) string {
	t.Helper()
	data, err := os.ReadFile(datapack.Path(t, "data", "html", "mods", "wedding", name))
	if err != nil {
		t.Fatal(err)
	}
	// The page cache reads a page line by line, ending each line with \n.
	page := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasSuffix(page, "\n") {
		page += "\n"
	}
	page = strings.ReplaceAll(page, "%objectId%", strconv.Itoa(int(c.manager.ObjectID())))
	page = strings.ReplaceAll(page, "%adenaCost%", price)
	return strings.ReplaceAll(page, "%needOrNot%", needOrNot)
}

// adena is the adena objID holds.
func (c *chapel) adena(t *testing.T, objID int32) int {
	t.Helper()
	return c.srv.PlayerInventory(t, objID).Adena()
}

func encodeAction(objectID int32, at location.Location) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAction)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	w.WriteUint8(0)
	return w.Bytes()
}

func encodeUseItem(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
	w.WriteInt32(objectID)
	w.WriteInt32(0)
	return w.Bytes()
}

// drainFrames collects every frame c receives until the server goes
// quiet, in arrival order.
func drainFrames(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	frames := make([][]byte, 0, 8)
	for range 200 {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			return frames
		}
		frames = append(frames, frame)
	}
	t.Fatal("client kept receiving frames after 200 drains")
	return nil
}

// only keeps the frames whose opcode is one of want, in order.
func only(frames [][]byte, want ...byte) [][]byte {
	var out [][]byte
	for _, f := range frames {
		for _, op := range want {
			if f[0] == op {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

func opcodes(frames [][]byte) []byte {
	out := make([]byte, 0, len(frames))
	for _, f := range frames {
		out = append(out, f[0])
	}
	return out
}

// htmlOf is the one NpcHtmlMessage among frames: its object id and page.
func htmlOf(t *testing.T, frames [][]byte) (int32, string) {
	t.Helper()
	pages := only(frames, serverpackets.OpcodeNpcHtmlMessage)
	if len(pages) != 1 {
		t.Fatalf("NpcHtmlMessage count = %d, want 1 (all %x)", len(pages), opcodes(frames))
	}
	r := wire.NewReader(pages[0][1:])
	return r.ReadInt32(), r.ReadString()
}

// assertPage checks frames carry exactly one page, from the manager, equal
// to want.
func (c *chapel) assertPage(t *testing.T, what string, frames [][]byte, want string) {
	t.Helper()
	objectID, got := htmlOf(t, frames)
	if objectID != c.manager.ObjectID() || got != want {
		t.Fatalf("%s: page from %d =\n%q\nwant from %d\n%q", what, objectID, got, c.manager.ObjectID(), want)
	}
}

// textOf decodes a plain-text SystemMessage.
func textOf(t *testing.T, frame []byte) string {
	t.Helper()
	if frame[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("opcode = %#x, want SystemMessage", frame[0])
	}
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != serverpackets.SystemMessageS1 {
		t.Fatalf("system message id = %d, want S1", id)
	}
	if n := r.ReadInt32(); n != 1 {
		t.Fatalf("system message params = %d, want 1", n)
	}
	r.ReadInt32()
	return r.ReadString()
}

// texts are the plain-text SystemMessages among frames, in order.
func texts(t *testing.T, frames [][]byte) []string {
	t.Helper()
	var out []string
	for _, f := range only(frames, serverpackets.OpcodeSystemMessage) {
		if wire.NewReader(f[1:]).ReadInt32() == serverpackets.SystemMessageS1 {
			out = append(out, textOf(t, f))
		}
	}
	return out
}

// systemMessageIDs are the ids of the SystemMessages among frames.
func systemMessageIDs(frames [][]byte) []int32 {
	var out []int32
	for _, f := range only(frames, serverpackets.OpcodeSystemMessage) {
		out = append(out, wire.NewReader(f[1:]).ReadInt32())
	}
	return out
}
