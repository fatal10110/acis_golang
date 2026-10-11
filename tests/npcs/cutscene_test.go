package npcs

import (
	"bytes"
	"encoding/binary"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// cutsceneMonsterID is the hostile NPC template the camera fixture spawns.
const cutsceneMonsterID = 21311

// cutsceneWorld is two players in one region, the first running the
// director script's invocations.
type cutsceneWorld struct {
	srv           *gameservertest.Server
	first, second *testsupport.ScriptedClient
	firstID       int32
	at            location.Location
}

// bootCutscene boots a quest script for the first player to run handle
// calls through, then logs both players in.
func bootCutscene(t *testing.T) *cutsceneWorld {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Director", playerLevel, 0), gameservertest.WithWantChars(1),
		gameservertest.WithScripts([]script.Listing{{Path: "quest.Director"}}, script.Catalog{
			"quest.Director": func() script.Script { return script.Script{} },
		}),
	)
	startInWorld(t, srv, srv.Client)
	srv.SeedCharacterFor(t, "player2", "Viewer", playerLevel, 0)
	second := srv.DialClient(t, "player2", 1)
	startInWorld(t, srv, second)
	firstID := srv.SoleObjectID(t)
	x, y, z := srv.PlayerPosition(t, firstID)
	return &cutsceneWorld{srv: srv, first: srv.Client, second: second, firstID: firstID, at: location.Location{X: x, Y: y, Z: z}}
}

// direct runs fn as one invocation of the director script on the first
// player's queue.
func (w *cutsceneWorld) direct(t *testing.T, fn func(p *script.Player)) {
	t.Helper()
	if !w.srv.RunScript(t, w.firstID, "Director", func(_ *script.Script, p *script.Player) { fn(p) }) {
		t.Fatal("director invocation panicked")
	}
}

// cameraPayload is the SpecialCamera payload, opcode first, the field
// appendix gives for a shot centred on objectID.
func cameraPayload(objectID int32, s script.CameraShot) []byte {
	out := []byte{0xc7}
	for _, v := range []int32{objectID, s.Distance, s.Yaw, s.Pitch, s.Time, s.Duration, s.Turn, s.Rise, s.Widescreen, s.Unknown} {
		out = binary.LittleEndian.AppendUint32(out, uint32(v))
	}
	return out
}

// pick returns the frames among frames whose opcode is one of opcodes, in
// arrival order.
func pick(frames [][]byte, opcodes ...byte) [][]byte {
	var out [][]byte
	for _, f := range frames {
		if len(f) > 0 && slices.Contains(opcodes, f[0]) {
			out = append(out, f)
		}
	}
	return out
}

// TestSpecialCameraReachesKnownPlayersInCallOrder: a camera shot played
// through a script's NPC handle reaches every player that knows the NPC
// once, centred on that NPC, between the chat line and the animation the
// script sent around it in call order; a shot from an NPC no player knows
// reaches nobody. Each player's frame is its own copy, and nothing is left
// pending for either client.
func TestSpecialCameraReachesKnownPlayersInCallOrder(t *testing.T) {
	t.Parallel()
	w := bootCutscene(t)
	near := w.srv.SpawnHostileNPCTemplateAt(t, spawnMonster(cutsceneMonsterID, "Watcher"), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
	far := w.srv.SpawnHostileNPCTemplateAt(t, spawnMonster(cutsceneMonsterID, "Distant"), location.Location{X: w.at.X + 40_000, Y: w.at.Y, Z: w.at.Z})
	folk := w.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", 30868), location.Location{X: w.at.X, Y: w.at.Y + 40, Z: w.at.Z})
	w.srv.ReadQueued(t, w.first)
	w.srv.ReadQueued(t, w.second)

	opening := script.CameraShot{Distance: 1100, Yaw: 210, Pitch: -5, Time: 3000, Duration: 10000, Turn: -13, Widescreen: 1, Unknown: 1}
	closing := script.CameraShot{Distance: 1700, Yaw: 10, Time: 300, Duration: 250, Turn: 20, Rise: -20, Widescreen: 1, Unknown: 1}
	folkShot := script.CameraShot{Distance: 1500, Yaw: 88, Pitch: 89, Duration: 10000, Rise: 110, Widescreen: 1}
	w.direct(t, func(_ *script.Player) {
		n := script.NPCOf(near)
		n.Say("The ground shakes.")
		n.SpecialCamera(opening)
		n.SocialAction(1)
		n.SpecialCamera(closing)
		script.NPCOf(far).SpecialCamera(opening)
		script.NPCOf(folk).SpecialCamera(folkShot)
	})

	wantCameras := [][]byte{
		cameraPayload(near.ObjectID(), opening),
		cameraPayload(near.ObjectID(), closing),
		cameraPayload(folk.ObjectID(), folkShot),
	}
	cameras := map[string][][]byte{}
	for _, viewer := range []struct {
		name string
		c    *testsupport.ScriptedClient
	}{{"first", w.first}, {"second", w.second}} {
		frames := w.srv.ReadQueued(t, viewer.c)
		if failed := pick(frames, serverpackets.OpcodeActionFailed); len(failed) != 0 {
			t.Errorf("%s: %d ActionFailed after the shots, want none", viewer.name, len(failed))
		}
		got := pick(frames, serverpackets.OpcodeNpcSay, serverpackets.OpcodeSpecialCamera, serverpackets.OpcodeSocialAction)
		want := []byte{serverpackets.OpcodeNpcSay, serverpackets.OpcodeSpecialCamera, serverpackets.OpcodeSocialAction, serverpackets.OpcodeSpecialCamera, serverpackets.OpcodeSpecialCamera}
		if len(got) != len(want) {
			t.Fatalf("%s: %d frames %x, want %x", viewer.name, len(got), got, want)
		}
		for i, f := range got {
			if f[0] != want[i] {
				t.Fatalf("%s: frame %d opcode %#x, want %#x", viewer.name, i, f[0], want[i])
			}
		}
		cams := pick(got, serverpackets.OpcodeSpecialCamera)
		for i, f := range cams {
			if !bytes.Equal(f, wantCameras[i]) {
				t.Errorf("%s: SpecialCamera %d = % x, want % x", viewer.name, i, f, wantCameras[i])
			}
		}
		cameras[viewer.name] = cams
	}

	cameras["first"][0][len(cameras["first"][0])-1] ^= 0xff
	if !bytes.Equal(cameras["second"][0], wantCameras[0]) {
		t.Fatalf("second's SpecialCamera changed with first's: % x", cameras["second"][0])
	}
}

// TestPlaySceneReachesOnlyTheTargetPlayer: the scene played through a
// script's player handle is one ExPlayScene to that player alone, with no
// body and nothing left pending; a player standing beside it gets nothing.
func TestPlaySceneReachesOnlyTheTargetPlayer(t *testing.T) {
	t.Parallel()
	w := bootCutscene(t)
	w.srv.ReadQueued(t, w.first)
	w.srv.ReadQueued(t, w.second)

	w.direct(t, func(p *script.Player) { p.PlayScene() })

	scene := []byte{0xfe, 0x5b, 0x00}
	own := w.srv.ReadQueued(t, w.first)
	if got := pick(own, serverpackets.OpcodeExtended, serverpackets.OpcodeActionFailed); len(got) != 1 || !bytes.Equal(got[0], scene) {
		t.Fatalf("target got %x, want one ExPlayScene % x", got, scene)
	}
	if got := pick(w.srv.ReadQueued(t, w.second), serverpackets.OpcodeExtended, serverpackets.OpcodeActionFailed); len(got) != 0 {
		t.Fatalf("bystander got %x, want nothing", got)
	}
}
