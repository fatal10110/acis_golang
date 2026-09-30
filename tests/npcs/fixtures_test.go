package npcs

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/dbtest"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

// playerLevel is the fixture character's level.
const playerLevel = 20

// folkWorld is a booted server with its one character in the world.
type folkWorld struct {
	srv    *gameservertest.Server
	c      *testsupport.ScriptedClient
	player int32
	at     location.Location
}

// bootFolkWorld boots the stack with pages in the HTML cache, enters the
// world with the plain fixture character and drains the enter burst.
func bootFolkWorld(t *testing.T, pages map[string]string, extra ...gameservertest.Option) *folkWorld {
	t.Helper()
	return bootFolkWorldAs(t, gameservertest.WithCharacter("Talker", playerLevel, 0), pages, extra...)
}

// bootFolkWorldAs is bootFolkWorld with character seeding the account's
// one character.
func bootFolkWorldAs(t *testing.T, character gameservertest.Option, pages map[string]string, extra ...gameservertest.Option) *folkWorld {
	t.Helper()
	opts := append([]gameservertest.Option{character, gameservertest.WithWantChars(1), gameservertest.WithHTMLPages(pages)}, extra...)
	srv := gameservertest.Boot(t, opts...)
	w := &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}
	startInWorld(t, w.c)
	x, y, z := srv.PlayerPosition(t, w.player)
	w.at = location.Location{X: x, Y: y, Z: z}
	return w
}

// withKarmaCharacter seeds the account's character carrying karma.
func withKarmaCharacter(karma int) gameservertest.Option {
	return gameservertest.WithSeed(func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		ch, err := player.NewCharacter(4242, gameservertest.ClassTemplate(), "player1", "Karma", playerLevel, 0, 0, player.SexMale)
		if err != nil {
			panic(err)
		}
		ch.KarmaPoints = karma
		ctx := context.Background()
		if err := chars.Create(ctx, ch); err != nil {
			panic(err)
		}
		if err := chars.Save(ctx, ch.SaveState()); err != nil {
			panic(err)
		}
	})
}

// folkTemplate is the fixture civilian template: level 70, 2444 base HP and
// CON 43 (a 1.58 CON bonus).
func folkTemplate(kind string, npcID int) *npc.Template {
	return gameservertest.FolkTemplate(kind, npcID)
}

// spawnFolk places a civilian NPC dx units east of the player and drains
// the NpcInfo it is shown with.
func (w *folkWorld) spawnFolk(t *testing.T, tmpl *npc.Template, dx int) *npc.Folk {
	t.Helper()
	f := w.srv.SpawnFolkNPCAt(t, tmpl, location.Location{X: w.at.X + dx, Y: w.at.Y, Z: w.at.Z})
	drainUntilQuiet(t, w.c)
	return f
}

// selectFolk clicks f once and drains the selection.
func (w *folkWorld) selectFolk(t *testing.T, f *npc.Folk) [][]byte {
	t.Helper()
	w.c.Send(encodeAction(f.ObjectID(), w.at, false))
	return drainFrames(t, w.c)
}

// talk clicks the already-selected f again and returns the answer.
func (w *folkWorld) talk(t *testing.T, f *npc.Folk, shift bool) [][]byte {
	t.Helper()
	w.c.Send(encodeAction(f.ObjectID(), w.at, shift))
	return drainFrames(t, w.c)
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

func encodeAction(objectID int32, at location.Location, shift bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAction)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	w.WriteUint8(wire.BoolByte(shift))
	return w.Bytes()
}

func startInWorld(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	c.Send(encodeRequestGameStart(0))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("opcode = %#x, want SSQInfo (%#x)", reply[0], serverpackets.OpcodeSSQInfo)
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected (%#x)", reply[0], serverpackets.OpcodeCharSelected)
	}
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	drainUntilQuiet(t, c)
}

func drainUntilQuiet(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	drainFrames(t, c)
}

// drainFrames collects every frame the client receives until the server
// goes quiet, in arrival order.
func drainFrames(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	frames := make([][]byte, 0, 8)
	for i := 0; i < 200; i++ {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			return frames
		}
		frames = append(frames, frame)
	}
	t.Fatal("client kept receiving frames after 200 drains")
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

// interactOrder keeps the frames an interact answers with, in order.
func interactOrder(frames [][]byte) []byte {
	var order []byte
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeActionFailed, serverpackets.OpcodeMoveToPawn, serverpackets.OpcodeSocialAction,
			serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeStopMove, serverpackets.OpcodeMoveToLocation:
			order = append(order, f[0])
		}
	}
	return order
}

// htmlMessage decodes an NpcHtmlMessage frame.
func htmlMessage(t *testing.T, frame []byte) (objectID int32, html string, itemID int32) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeNpcHtmlMessage {
		t.Fatalf("opcode = %#x, want NpcHtmlMessage", frame[0])
	}
	r := wire.NewReader(frame[1:])
	return r.ReadInt32(), r.ReadString(), r.ReadInt32()
}

// moveToPawn decodes a MoveToPawn frame.
func moveToPawn(t *testing.T, frame []byte) (mover, target, distance int32) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeMoveToPawn {
		t.Fatalf("opcode = %#x, want MoveToPawn", frame[0])
	}
	r := wire.NewReader(frame[1:])
	return r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
}

func encodeAttackRequest(objectID int32, at location.Location) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAttackRequest)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	w.WriteUint8(0)
	return w.Bytes()
}

// withShop sets KarmaPlayerCanShop, keeping the other karma gates at their
// defaults.
func withShop(allowed bool) gameservertest.Option {
	return gameservertest.WithKarmaServiceGates(allowed, false, true)
}
