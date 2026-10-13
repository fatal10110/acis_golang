package npcs

import (
	"bytes"
	"encoding/binary"
	"testing"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// musicPayload is the PlaySound payload, opcode first, of music file bound
// to objectID standing at at: type 1, the file as a UTF-16 string, bound 1,
// the object id, the location and no delay.
func musicPayload(objectID int32, file string, at location.Location) []byte {
	out := []byte{0x98}
	out = binary.LittleEndian.AppendUint32(out, 1)
	for _, r := range utf16.Encode([]rune(file)) {
		out = binary.LittleEndian.AppendUint16(out, r)
	}
	out = append(out, 0, 0)
	for _, v := range []int32{1, objectID, int32(at.X), int32(at.Y), int32(at.Z), 0} {
		out = binary.LittleEndian.AppendUint32(out, uint32(v))
	}
	return out
}

// TestNPCMusicReachesKnownPlayersInCallOrder: music played through a
// script's NPC handle (a boss's npc.broadcastPacket(new PlaySound(1, file,
// npc))) reaches every player that knows the NPC once, bound to the NPC
// where it stands, in call order with the chat line, the camera shot and
// the animation around it; music from an NPC no player knows reaches
// nobody. Each player's frame is its own copy.
func TestNPCMusicReachesKnownPlayersInCallOrder(t *testing.T) {
	t.Parallel()
	w := bootCutscene(t)
	near := w.srv.SpawnHostileNPCTemplateAt(t, spawnMonster(cutsceneMonsterID, "Boss"), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
	far := w.srv.SpawnHostileNPCTemplateAt(t, spawnMonster(cutsceneMonsterID, "Distant"), location.Location{X: w.at.X + 40_000, Y: w.at.Y, Z: w.at.Z})
	folk := w.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", 30868), location.Location{X: w.at.X, Y: w.at.Y + 40, Z: w.at.Z})
	w.srv.ReadQueued(t, w.first)
	w.srv.ReadQueued(t, w.second)

	shot := script.CameraShot{Distance: 1100, Yaw: 210, Pitch: -5, Time: 3000, Duration: 10000, Widescreen: 1}
	w.direct(t, func(_ *script.Player) {
		n := script.NPCOf(near)
		n.Say("The ground shakes.")
		n.PlayMusic("BS01_A")
		n.SpecialCamera(shot)
		n.SocialAction(1)
		script.NPCOf(far).PlayMusic("BS02_D")
		script.NPCOf(folk).PlayMusic("Rm01_A")
	})

	at := func(o interface{ Position() (int, int, int) }) location.Location {
		x, y, z := o.Position()
		return location.Location{X: x, Y: y, Z: z}
	}
	wantMusic := [][]byte{
		musicPayload(near.ObjectID(), "BS01_A", at(near)),
		musicPayload(folk.ObjectID(), "Rm01_A", at(folk)),
	}
	wantOrder := []byte{serverpackets.OpcodeNpcSay, serverpackets.OpcodePlaySound, serverpackets.OpcodeSpecialCamera, serverpackets.OpcodeSocialAction, serverpackets.OpcodePlaySound}
	music := map[string][][]byte{}
	for _, viewer := range []struct {
		name string
		c    *testsupport.ScriptedClient
	}{{"first", w.first}, {"second", w.second}} {
		frames := w.srv.ReadQueued(t, viewer.c)
		got := pick(frames, serverpackets.OpcodeNpcSay, serverpackets.OpcodePlaySound, serverpackets.OpcodeSpecialCamera, serverpackets.OpcodeSocialAction)
		if len(got) != len(wantOrder) {
			t.Fatalf("%s: %d frames, want %x", viewer.name, len(got), wantOrder)
		}
		for i, f := range got {
			if f[0] != wantOrder[i] {
				t.Fatalf("%s: frame %d opcode %#x, want %#x", viewer.name, i, f[0], wantOrder[i])
			}
		}
		sounds := pick(got, serverpackets.OpcodePlaySound)
		for i, f := range sounds {
			if !bytes.Equal(f, wantMusic[i]) {
				t.Errorf("%s: PlaySound %d = % x, want % x", viewer.name, i, f, wantMusic[i])
			}
		}
		music[viewer.name] = sounds
	}

	music["first"][0][len(music["first"][0])-1] ^= 0xff
	if !bytes.Equal(music["second"][0], wantMusic[0]) {
		t.Fatalf("second's PlaySound changed with first's: % x", music["second"][0])
	}
}

// TestNPCShoutInRadiusReachesPlayersByDistance: a line an NPC shouts in a
// radius (npc.broadcastPacketInRadius(new NpcSay(npc, SHOUT, text), r))
// reaches each player within that radius of it, however far, as a shout
// naming the NPC, and no player beyond it. A monster and a folk NPC (the
// Four Sepulchers managers that shout this way are folk) shout alike.
func TestNPCShoutInRadiusReachesPlayersByDistance(t *testing.T) {
	t.Parallel()
	w := bootCutscene(t)
	viewer, ok := w.srv.State.PlayerByName("Viewer")
	if !ok {
		t.Fatal("the viewer is not in the world")
	}
	secondID := viewer.ObjectID()
	far := location.Location{X: w.at.X + 6000, Y: w.at.Y, Z: w.at.Z}
	if !w.srv.RunScript(t, secondID, "Director", func(_ *script.Script, p *script.Player) {
		p.InstantTeleportTo(script.Loc{X: far.X, Y: far.Y, Z: far.Z})
	}) {
		t.Fatal("teleport invocation panicked")
	}
	w.srv.AdvanceUntil(t, "the viewer stands 6000 away", func() bool {
		x, _, _ := w.srv.PlayerPosition(t, secondID)
		return x == far.X
	})
	const folkID = 31921
	crier := w.srv.SpawnHostileNPCTemplateAt(t, spawnMonster(cutsceneMonsterID, "Crier"), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
	folk := w.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", folkID), location.Location{X: w.at.X, Y: w.at.Y + 40, Z: w.at.Z})
	w.srv.ReadQueued(t, w.first)
	w.srv.ReadQueued(t, w.second)

	w.direct(t, func(_ *script.Player) {
		for _, n := range []*script.NPC{script.NPCOf(crier), script.NPCOf(folk)} {
			n.ShoutInRadius("Far and wide", 11500)
			n.ShoutInRadius("Close by", 600)
		}
	})

	say := func(objectID int32, npcID int, text string) []byte {
		out := []byte{serverpackets.OpcodeNpcSay}
		for _, v := range []int32{objectID, 1, int32(npcID) + 1_000_000} {
			out = binary.LittleEndian.AppendUint32(out, uint32(v))
		}
		for _, r := range utf16.Encode([]rune(text)) {
			out = binary.LittleEndian.AppendUint16(out, r)
		}
		return append(out, 0, 0)
	}
	crierSay := func(text string) []byte { return say(crier.ObjectID(), cutsceneMonsterID, text) }
	folkSay := func(text string) []byte { return say(folk.ObjectID(), folkID, text) }
	for _, viewer := range []struct {
		name string
		c    *testsupport.ScriptedClient
		want [][]byte
	}{
		{"first", w.first, [][]byte{crierSay("Far and wide"), crierSay("Close by"), folkSay("Far and wide"), folkSay("Close by")}},
		{"second", w.second, [][]byte{crierSay("Far and wide"), folkSay("Far and wide")}},
	} {
		got := pick(w.srv.ReadQueued(t, viewer.c), serverpackets.OpcodeNpcSay)
		if len(got) != len(viewer.want) {
			t.Fatalf("%s: %d NpcSay, want %d", viewer.name, len(got), len(viewer.want))
		}
		for i := range got {
			if !bytes.Equal(got[i], viewer.want[i]) {
				t.Errorf("%s: NpcSay %d = % x, want % x", viewer.name, i, got[i], viewer.want[i])
			}
		}
	}
}
