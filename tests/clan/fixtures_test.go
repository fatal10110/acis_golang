package clan

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/dbtest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

// masterID is the village master the clan commands are sent to.
const masterID = 30026

// clanWorld is a booted server with the founder in the world beside a
// village master whose page links the clan commands, and a second player
// in the world too.
type clanWorld struct {
	srv      *gameservertest.Server
	leader   *testsupport.ScriptedClient
	leaderID int32
	member   *testsupport.ScriptedClient
	memberID int32
	at       location.Location
	master   *npc.Folk
}

// clanPages stands the clan dialog in for the village master's own chat
// page: the four datapack pages whose links name the clan commands, as the
// clan dialog serves them, plus the pages a leader nomination answers with.
func clanPages(t *testing.T) map[string]string {
	t.Helper()
	read := func(name string) string {
		data, err := os.ReadFile(datapack.Path(t, "data", "html", "script", "feature", "Clan", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return string(data)
	}
	pages := map[string]string{
		"villagemaster/" + strconv.Itoa(masterID) + ".htm": read("9000-02.htm") + read("9000-03.htm") + read("9000-07.htm") + read("9000-08.htm"),
	}
	for _, name := range []string{"9000-07-success.htm", "9000-07-in-progress.htm", "9000-08-success.htm", "9000-08-no.htm"} {
		pages[filepath.Join("script", "feature", "Clan", name)] = read(name)
	}
	return pages
}

// bootClanWorld boots the founder (level leaderLevel, leaderSP SP, carrying
// leaderAdena adena) and the recruit (level 1), both in the world, and the
// village master.
func bootClanWorld(t *testing.T, leaderLevel, leaderSP, leaderAdena int, extra ...gameservertest.Option) *clanWorld {
	t.Helper()
	return bootClanWorldCarrying(t, leaderLevel, leaderSP, map[int32]int32{item.AdenaID: int32(leaderAdena)}, extra...)
}

// bootClanWorldCarrying is bootClanWorld with the founder carrying items,
// count by template id, as it enters the world.
func bootClanWorldCarrying(t *testing.T, leaderLevel, leaderSP int, items map[int32]int32, extra ...gameservertest.Option) *clanWorld {
	t.Helper()
	opts := append([]gameservertest.Option{
		gameservertest.WithCharacter("Founder", leaderLevel, leaderSP),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(clanPages(t)),
		gameservertest.WithReuseDelays(0, 0),
	}, extra...)
	srv := gameservertest.Boot(t, opts...)
	w := &clanWorld{srv: srv, leader: srv.Client, leaderID: srv.SoleObjectID(t)}
	w.memberID = srv.SeedCharacterFor(t, "player2", "Recruit", 1, 0).ID
	w.member = srv.DialClient(t, "player2", 1)
	for id, count := range items {
		if count > 0 {
			srv.GiveItem(t, w.leaderID, id, count)
		}
	}
	startInWorld(t, w.leader)
	startInWorld(t, w.member)
	drainFrames(t, w.leader)
	x, y, z := srv.PlayerPosition(t, w.leaderID)
	w.at = location.Location{X: x, Y: y, Z: z}
	w.master = srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("VillageMaster", masterID), location.Location{X: x + 30, Y: y, Z: z})
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	return w
}

// talkToMaster has the founder select and talk to the master, opening the
// clan page.
func (w *clanWorld) talkToMaster(t *testing.T) {
	t.Helper()
	w.leader.Send(encodeAction(w.master.ObjectID(), w.at))
	drainFrames(t, w.leader)
	w.leader.Send(encodeAction(w.master.ObjectID(), w.at))
	if _, ok := firstOpcode(drainFrames(t, w.leader), serverpackets.OpcodeNpcHtmlMessage); !ok {
		t.Fatal("talking to the village master opened no page")
	}
}

// masterCommand sends the founder's clan command to the master and
// returns the founder's answer.
func (w *clanWorld) masterCommand(t *testing.T, command string) [][]byte {
	t.Helper()
	w.leader.Send(encodeBypass("npc_" + strconv.Itoa(int(w.master.ObjectID())) + "_" + command))
	return drainFrames(t, w.leader)
}

// found has the founder found the clan named name, and returns its id.
func (w *clanWorld) found(t *testing.T, name string) int32 {
	t.Helper()
	w.talkToMaster(t)
	frames := w.masterCommand(t, "create_clan "+name)
	list, ok := firstOpcode(frames, serverpackets.OpcodePledgeShowMemberListAll)
	if !ok {
		t.Fatalf("create_clan %s answer = %x, want PledgeShowMemberListAll", name, opcodes(frames))
	}
	drainFrames(t, w.member)
	r := wire.NewReader(list[1:])
	r.ReadInt32()
	return r.ReadInt32()
}

// recruit has the founder invite the recruit and the recruit accept.
func (w *clanWorld) recruit(t *testing.T) {
	t.Helper()
	w.leader.Send(encodeRequestJoinPledge(w.memberID, 0))
	drainFrames(t, w.member)
	w.member.Send(encodeRequestAnswerJoinPledge(1))
	if _, ok := firstOpcode(drainFrames(t, w.member), serverpackets.OpcodeJoinPledge); !ok {
		t.Fatal("recruit did not join")
	}
	drainFrames(t, w.leader)
}

// leaveWorld logs c out and waits for its saves.
func (w *clanWorld) leaveWorld(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeLogout).Bytes())
	c.AwaitClose(5 * time.Second)
	w.srv.FlushPersistence(t)
}

func encodeRequestGameStart(slot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGameStart)
	w.WriteInt32(slot)
	w.WriteUint16(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	return w.Bytes()
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

func encodeBypass(command string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestBypassToServer)
	w.WriteString(command)
	return w.Bytes()
}

func encodeRequestJoinPledge(target, pledgeType int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestJoinPledge)
	w.WriteInt32(target)
	w.WriteInt32(pledgeType)
	return w.Bytes()
}

func encodeRequestAnswerJoinPledge(answer int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAnswerJoinPledge)
	w.WriteInt32(answer)
	return w.Bytes()
}

func encodeRequestOustPledgeMember(name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestOustPledgeMember)
	w.WriteString(name)
	return w.Bytes()
}

func encodeRequestPledgePower(rank, action, privs int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestPledgePower)
	w.WriteInt32(rank)
	w.WriteInt32(action)
	if action == 2 {
		w.WriteInt32(privs)
	}
	return w.Bytes()
}

func encodeExtended(sub uint16) *wire.Writer {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(sub)
	return w
}

func encodeRequestPledgeMemberName(sub uint16, name string) []byte {
	w := encodeExtended(sub)
	w.WriteInt32(0)
	w.WriteString(name)
	return w.Bytes()
}

func encodeRequestPledgeSetMemberPowerGrade(name string, grade int32) []byte {
	w := encodeExtended(clientpackets.OpcodeRequestPledgeSetGrade)
	w.WriteString(name)
	w.WriteInt32(grade)
	return w.Bytes()
}

func startInWorld(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	c.Send(encodeRequestGameStart(0))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("opcode = %#x, want SSQInfo", reply[0])
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected", reply[0])
	}
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	return drainFrames(t, c)
}

// drainFrames collects every frame c receives until the server goes quiet.
func drainFrames(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	frames := make([][]byte, 0, 8)
	for range 400 {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			return frames
		}
		frames = append(frames, frame)
	}
	t.Fatal("client kept receiving frames after 400 drains")
	return nil
}

func opcodes(frames [][]byte) []byte {
	out := make([]byte, 0, len(frames))
	for _, f := range frames {
		out = append(out, f[0])
	}
	return out
}

func firstOpcode(frames [][]byte, opcode byte) ([]byte, bool) {
	for _, f := range frames {
		if len(f) > 0 && f[0] == opcode {
			return f, true
		}
	}
	return nil, false
}

// sysMsg decodes a SystemMessage frame: its id and its text or number
// parameters, numbers written in decimal.
func sysMsg(t *testing.T, frame []byte) (int, []string) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("opcode = %#x, want SystemMessage", frame[0])
	}
	r := wire.NewReader(frame[1:])
	id := int(r.ReadInt32())
	n := int(r.ReadInt32())
	var params []string
	for range n {
		if r.ReadInt32() == serverpackets.SystemMessageParamText {
			params = append(params, r.ReadString())
		} else {
			params = append(params, strconv.Itoa(int(r.ReadInt32())))
		}
	}
	return id, params
}

// messages returns the ids of the system messages among frames, in order.
func messages(t *testing.T, frames [][]byte) []int {
	t.Helper()
	var ids []int
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage {
			id, _ := sysMsg(t, f)
			ids = append(ids, id)
		}
	}
	return ids
}

// only keeps the frames whose opcode is in keep, in order.
func only(frames [][]byte, keep ...byte) []byte {
	var out []byte
	for _, f := range frames {
		for _, k := range keep {
			if f[0] == k {
				out = append(out, f[0])
				break
			}
		}
	}
	return out
}

// queryInt reads one integer column.
func queryInt(t *testing.T, w *clanWorld, query string, args ...any) int64 {
	t.Helper()
	var v int64
	if err := w.srv.DB.QueryRowContext(context.Background(), query, args...).Scan(&v); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return v
}

func itoa(id int32) string { return strconv.Itoa(int(id)) }
