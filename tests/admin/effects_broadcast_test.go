package admin

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: AdminEffects.useAdminCommand admin_earthquake, admin_play_sound,
// admin_social, admin_jukebox and admin_undying; Earthquake, PlaySound and
// SocialAction writeImpl; CreatureStatus/PlayerStatus.reduceHp's "blocked
// to 1 if the Creature isn't mortal".

// earthquakeBytes is the Earthquake packet: opcode 0xc4, the location,
// intensity, duration and the NPC flag.
func earthquakeBytes(x, y, z int, intensity, duration int32) []byte {
	w := wire.NewPacketWriter(0xc4)
	w.WriteInt32(int32(x))
	w.WriteInt32(int32(y))
	w.WriteInt32(int32(z))
	w.WriteInt32(intensity)
	w.WriteInt32(duration)
	w.WriteInt32(0)
	return w.Bytes()
}

// playSoundBytes is a PlaySound bound to nothing: opcode 0x98, type, file,
// bind flag, object id, the dummy location and no delay.
func playSoundBytes(soundType int32, file string) []byte {
	w := wire.NewPacketWriter(0x98)
	w.WriteInt32(soundType)
	w.WriteString(file)
	for range 6 {
		w.WriteInt32(0)
	}
	return w.Bytes()
}

// socialBytes is a SocialAction: opcode 0x2d, the object id and action.
func socialBytes(objID, action int32) []byte {
	w := wire.NewPacketWriter(0x2d)
	w.WriteInt32(objID)
	w.WriteInt32(action)
	return w.Bytes()
}

// holdsFrame reports whether frames hold want byte for byte.
func holdsFrame(frames [][]byte, want []byte) bool {
	for _, f := range frames {
		if bytes.Equal(f, want) {
			return true
		}
	}
	return false
}

// requireOnly fails unless frames are want alone.
func requireOnly(t *testing.T, what string, frames [][]byte, want []byte) {
	t.Helper()
	if len(frames) != 1 || !bytes.Equal(frames[0], want) {
		t.Fatalf("%s frames = %x, want % x alone", what, frames, want)
	}
}

// TestAdminEffectsEarthquake pins //earthquake: every online player, the GM
// included, gets its own copy of one earthquake at the GM's position; a
// missing or malformed number answers the usage to the GM alone; the
// effects panel bypass is the same command; a player without the access
// level is refused.
func TestAdminEffectsEarthquake(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	first, _ := addPlayer(t, srv, "player2", "First", userLevel)
	second, _ := addPlayer(t, srv, "player3", "Second", userLevel)
	clients := []*testsupport.ScriptedClient{gm, first, second}
	for _, c := range clients {
		drain(t, c)
	}
	x, y, z := srv.PlayerPosition(t, gmID)

	for _, command := range []string{"earthquake 5 10", "earthquake 5 10 extra"} {
		want := earthquakeBytes(x, y, z, 5, 10)
		gm.Send(encodeBuildCmd(command))
		received := make([][]byte, len(clients))
		for i, c := range clients {
			received[i] = c.Read()
			if !bytes.Equal(received[i], want) {
				t.Fatalf("//%s client %d frame = % x, want % x", command, i, received[i], want)
			}
		}
		received[0][0] ^= 0xff
		for i := 1; i < len(received); i++ {
			if !bytes.Equal(received[i], want) {
				t.Fatalf("//%s client %d frame changed with client 0's", command, i)
			}
		}
	}
	gm.Send(encodeBypass("admin_earthquake -3 2147483647"))
	for i, c := range clients {
		if got, want := c.Read(), earthquakeBytes(x, y, z, -3, 2147483647); !bytes.Equal(got, want) {
			t.Fatalf("admin_earthquake bypass client %d frame = % x, want % x", i, got, want)
		}
	}

	for _, bad := range []string{"earthquake", "earthquake 5", "earthquake a 5", "earthquake 5 2147483648", "earthquake 5.0 1"} {
		assertTexts(t, exchange(t, gm, encodeBuildCmd(bad)), "Use: //earthquake <intensity> <duration>")
	}
	assertTexts(t, exchange(t, first, encodeBuildCmd("earthquake 5 10")), "You don't have the access right to use this command.")
	for i, c := range []*testsupport.ScriptedClient{gm, second} {
		if frames := barrier(t, c); len(frames) != 0 {
			t.Fatalf("client %d frames after bad or refused //earthquake = %x, want none", i, testsupport.FrameOpcodes(frames))
		}
	}
}

// TestAdminEffectsPlaySound pins //play_sound: the GM and the players that
// know it hear the file, a name holding a dot as a sound (type 0), any
// other as music (type 1); a missing file answers the usage.
func TestAdminEffectsPlaySound(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	near, _ := addPlayer(t, srv, "player2", "Near", userLevel)
	drain(t, gm)
	drain(t, near)

	for _, tc := range []struct {
		command string
		want    []byte
	}{
		{"play_sound ItemSound.quest_accept", playSoundBytes(0, "ItemSound.quest_accept")},
		{"play_sound Rm01_A", playSoundBytes(1, "Rm01_A")},
	} {
		requireOnly(t, "//"+tc.command+" GM", exchange(t, gm, encodeBuildCmd(tc.command)), tc.want)
		if frames := settle(t, near); !holdsFrame(frames, tc.want) {
			t.Fatalf("//%s known player frames = %x, want % x", tc.command, frames, tc.want)
		}
	}
	requireOnly(t, "admin_play_sound bypass", exchange(t, gm, encodeBypass("admin_play_sound Rm01_B")), playSoundBytes(1, "Rm01_B"))
	settle(t, near)
	assertTexts(t, exchange(t, gm, encodeBuildCmd("play_sound")), "Usage: //play_sound soundFile")
	if frames := settle(t, near); len(frames) != 0 {
		t.Fatalf("known player frames after //play_sound usage = %x, want none", testsupport.FrameOpcodes(frames))
	}
}

// TestAdminEffectsSocialPlayers pins //social on players: the GM itself
// without a selection, a selected player otherwise, playing actions 2 to
// 16 for itself and every player that knows it; 1 and 17 are refused as an
// invalid target; a missing or malformed action answers the usage.
func TestAdminEffectsSocialPlayers(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)
	drain(t, user)

	requireOnly(t, "//social 2 GM", exchange(t, gm, encodeBuildCmd("social 2")), socialBytes(gmID, 2))
	if frames := settle(t, user); !holdsFrame(frames, socialBytes(gmID, 2)) {
		t.Fatalf("//social 2 bystander frames = %x, want the GM's social action", frames)
	}
	for _, refused := range []string{"social 1", "social 17", "social -2"} {
		frames := exchange(t, gm, encodeBuildCmd(refused))
		if len(frames) != 1 {
			t.Fatalf("//%s frames = %x, want INVALID_TARGET alone", refused, testsupport.FrameOpcodes(frames))
		}
		assertStatic(t, frames[0], serverpackets.SystemMessageInvalidTarget)
	}
	for _, bad := range []string{"social", "social wave", "social 2147483648"} {
		assertTexts(t, exchange(t, gm, encodeBuildCmd(bad)), "Usage: //social actionId")
	}
	if frames := settle(t, user); len(frames) != 0 {
		t.Fatalf("bystander frames after refused //social = %x, want none", testsupport.FrameOpcodes(frames))
	}

	selectPlayer(t, gm, user, userID)
	frames := exchange(t, gm, encodeBypass("admin_social 16"))
	if !holdsFrame(frames, socialBytes(userID, 16)) {
		t.Fatalf("admin_social 16 GM frames = %x, want the player's social action", frames)
	}
	if frames := settle(t, user); !holdsFrame(frames, socialBytes(userID, 16)) {
		t.Fatalf("admin_social 16 target frames = %x, want its own social action", frames)
	}
}

// TestAdminEffectsSocialNPCs pins //social on NPCs: a selected NPC plays
// actions 1 to 3 for the players that know it, 0 and 4 are refused; a chest
// and a summon are refused whatever the action.
func TestAdminEffectsSocialNPCs(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel, summonAdminOptions(t)...)
	gm := srv.Client
	enterWorld(t, gm)
	x, y, z := srv.PlayerPosition(t, gmID)
	folk := srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", 30100), location.Location{X: x + 40, Y: y, Z: z})
	chest := srv.SpawnHostileNPCTemplateAt(t, &npc.Template{
		ID: 18265, TemplateID: 18265, Type: "Chest", Level: 20,
		HPMax: 1000, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}, location.Location{X: x - 40, Y: y, Z: z})
	drain(t, gm)
	invalid := func(command string) {
		t.Helper()
		frames := exchange(t, gm, encodeBuildCmd(command))
		if len(frames) != 1 {
			t.Fatalf("//%s frames = %x, want INVALID_TARGET alone", command, testsupport.FrameOpcodes(frames))
		}
		assertStatic(t, frames[0], serverpackets.SystemMessageInvalidTarget)
	}

	exchange(t, gm, encodeAction(folk.ObjectID()))
	for _, action := range []int32{1, 3} {
		frames := append(exchange(t, gm, encodeBuildCmd("social "+strconv.Itoa(int(action)))), settle(t, gm)...)
		requireOnly(t, "//social on the NPC", frames, socialBytes(folk.ObjectID(), action))
	}
	invalid("social 0")
	invalid("social 4")

	exchange(t, gm, encodeAction(chest.ObjectID()))
	invalid("social 2")

	owner, ownerID, collar := addSummoner(t, srv, "player2", "Owner")
	pet := callPet(t, srv, owner, ownerID, collar)
	drain(t, gm)
	exchange(t, gm, encodeAction(pet.ObjectID()))
	invalid("social 2")
}

// TestAdminEffectsUndying pins //undying: the GM, never its selection,
// turns immortal and back, told each time; while immortal a lethal hit or
// HP cost leaves it on 1 HP alive, mortal again the hit kills.
func TestAdminEffectsUndying(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)
	selectPlayer(t, gm, user, userID)
	gmChar, target := onlineCharacter(t, srv, gmID), onlineCharacter(t, srv, userID)
	attacker := onlineCharacter(t, srv, userID)

	assertTexts(t, exchange(t, gm, encodeBuildCmd("undying")), "You are now immortal.")
	if gmChar.Mortal() || !target.Mortal() {
		t.Fatalf("after //undying GM mortal %v, target mortal %v, want false, true", gmChar.Mortal(), target.Mortal())
	}
	lethal := func(pc *player.Character) {
		res := pc.ResourceValues()
		pc.ReduceHP(res.MaxHP+res.MaxCP+1, attacker, modelskill.Definition{})
	}
	onLearnerQueue(t, srv, gmID, lethal)
	if hp := gmChar.ResourceValues().CurrentHP; gmChar.Dead() || hp != 1 {
		t.Fatalf("immortal GM after a lethal hit: dead %v, HP %v, want alive on 1", gmChar.Dead(), hp)
	}
	onLearnerQueue(t, srv, gmID, func(pc *player.Character) {
		if died := pc.ReduceCurrentHP(1_000_000); died {
			t.Error("immortal GM died of an HP cost")
		}
	})
	if hp := gmChar.ResourceValues().CurrentHP; gmChar.Dead() || hp != 1 {
		t.Fatalf("immortal GM after a lethal cost: dead %v, HP %v, want alive on 1", gmChar.Dead(), hp)
	}

	// The GM on 1 HP may be shown regenerating meanwhile.
	if texts, _ := effectReply(t, exchange(t, gm, encodeBypass("admin_undying"))); len(texts) != 1 || texts[0] != "You are now mortal." {
		t.Fatalf("admin_undying texts = %q, want \"You are now mortal.\"", texts)
	}
	onLearnerQueue(t, srv, gmID, lethal)
	if !gmChar.Dead() {
		t.Fatal("mortal GM survived a lethal hit")
	}
}
