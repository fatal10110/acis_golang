package admin

import (
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// abnormalStealth is AbnormalEffect.STEALTH's mask.
const abnormalStealth = 0x100000

// charInfoHidden returns a CharInfo frame's object id and its invisible
// byte, read field by field up to it.
func charInfoHidden(t *testing.T, frame []byte) (int32, byte) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeCharInfo {
		t.Fatalf("opcode = %#x, want CharInfo", frame[0])
	}
	r := wire.NewReader(frame[1:])
	r.ReadBytes(4 * 4) // x, y, z, boat
	id := r.ReadInt32()
	r.ReadString()        // name
	r.ReadBytes(3 * 4)    // race, sex, class
	r.ReadBytes(12 * 4)   // paperdoll
	r.ReadBytes(4*2 + 4)  // padding, right-hand augmentation
	r.ReadBytes(12*2 + 4) // padding, left-hand augmentation
	r.ReadBytes(4 * 2)    // padding
	r.ReadBytes(6 * 4)    // pvp flag, karma, casting/attack speed, pvp flag, karma
	r.ReadBytes(8 * 4)    // speeds
	r.ReadBytes(4 * 8)    // speed multipliers, collision
	r.ReadBytes(3 * 4)    // hair style, hair color, face
	r.ReadString()        // title
	r.ReadBytes(5 * 4)    // clan, crest, ally, ally crest, relation
	r.ReadBytes(4)        // standing, running, in combat, alike dead
	hidden := r.ReadUint8()
	if err := r.Err(); err != nil {
		t.Fatalf("decode CharInfo: %v", err)
	}
	return id, hidden
}

// objectFrames keeps the frames of frames with opcode op whose first int32
// is objectID.
func objectFrames(frames [][]byte, op byte, objectID int32) [][]byte {
	var out [][]byte
	for _, f := range frames {
		if f[0] == op && len(f) >= 5 && int32(binary.LittleEndian.Uint32(f[1:])) == objectID {
			out = append(out, f)
		}
	}
	return out
}

// requireRediscovered requires frames to forget objectID (DeleteObject) and
// then show it again with one CharInfo whose invisible byte is hidden.
func requireRediscovered(t *testing.T, who string, frames [][]byte, objectID int32, hidden byte) {
	t.Helper()
	deleted, info := -1, -1
	for i, f := range frames {
		if f[0] == serverpackets.OpcodeDeleteObject && int32(binary.LittleEndian.Uint32(f[1:])) == objectID && deleted < 0 {
			deleted = i
		}
		if f[0] == serverpackets.OpcodeCharInfo {
			if id, got := charInfoHidden(t, f); id == objectID {
				if info >= 0 {
					t.Fatalf("%s: two CharInfo of %d in %x", who, objectID, testsupport.FrameOpcodes(frames))
				}
				info = i
				if got != hidden {
					t.Fatalf("%s: CharInfo invisible byte = %d, want %d", who, got, hidden)
				}
			}
		}
	}
	if deleted < 0 || info < deleted {
		t.Fatalf("%s: frames %x (DeleteObject at %d, CharInfo at %d), want %d forgotten then shown again", who, testsupport.FrameOpcodes(frames), deleted, info, objectID)
	}
}

// onlineCharacter returns objID's character in the world.
func onlineCharacter(t *testing.T, srv *gameservertest.Server, objID int32) *player.Character {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("player %d not in the world", objID)
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %d is %T, not an online character", objID, obj)
	}
	return c
}

// onlyUserInfoFrame requires frames to hold exactly one UserInfo and
// returns it.
func onlyUserInfoFrame(t *testing.T, frames [][]byte) []byte {
	t.Helper()
	var found []byte
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeUserInfo {
			if found != nil {
				t.Fatalf("frames %x hold two UserInfo", testsupport.FrameOpcodes(frames))
			}
			found = f
		}
	}
	if found == nil {
		t.Fatalf("frames %x hold no UserInfo", testsupport.FrameOpcodes(frames))
	}
	return found
}

func encodeJoinParty(name string, loot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestJoinParty)
	w.WriteString(name)
	w.WriteInt32(loot)
	return w.Bytes()
}

// TestAdminHide pins //hide (AdminEffects.java admin_hide) and what an
// invisible player is (Appearance.isVisible):
//
//   - hiding takes the GM off the grid and back: the players around forget
//     it and are shown it again, drawn invisible (CharInfo invisible byte 1)
//     to a non-GM and visible to a GM; the GM's own UserInfo carries the
//     stealth abnormal mask (UserInfo.java:190);
//   - only a GM knows an invisible player (Creature.knows): a player's
//     trade request is the wrong target (TradeRequest.java:38), a party
//     invitation the wrong target (RequestJoinParty.java:50), and a monster
//     neither knows it nor may auto-attack it (Npc.canAutoAttack);
//   - showing again resends UserInfo without the mask and CharInfo with the
//     invisible byte 0, and everything above is undone.
func TestAdminHide(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	watcher, watcherID := addPlayer(t, srv, "player2", "Watcher", userLevel)
	otherGM, otherGMID := addPlayer(t, srv, "player3", "OtherGM", adminLevel)
	monster := srv.SpawnHostileNPCAt(t, location.Location{X: spawnX + 40, Y: spawnY, Z: spawnZ})
	drain(t, gm)
	drain(t, watcher)
	drain(t, otherGM)

	gmChar := onlineCharacter(t, srv, gmID)
	watcherChar := onlineCharacter(t, srv, watcherID)
	otherGMChar := onlineCharacter(t, srv, otherGMID)
	if !monster.AutoAttackTargetValid(gmChar, 1000, true) || !monster.Knows(gmChar) || !watcherChar.Knows(gmChar) {
		t.Fatal("control: a visible GM is not a known, auto-attackable target")
	}

	hideFrames := exchange(t, gm, encodeBuildCmd("hide"))
	if !gmChar.Invisible() {
		t.Fatal("//hide left the GM visible")
	}
	hidden := onlyUserInfoFrame(t, hideFrames)
	requireRediscovered(t, "GM sees Watcher", hideFrames, watcherID, 0)
	requireRediscovered(t, "GM sees OtherGM", hideFrames, otherGMID, 0)
	requireRediscovered(t, "Watcher", settle(t, watcher), gmID, 1)
	requireRediscovered(t, "OtherGM", settle(t, otherGM), gmID, 0)

	if watcherChar.Knows(gmChar) {
		t.Fatal("a player knows an invisible GM")
	}
	if !otherGMChar.Knows(gmChar) {
		t.Fatal("a GM does not know an invisible GM")
	}
	if monster.Knows(gmChar) {
		t.Fatal("a monster knows an invisible GM")
	}
	if monster.AutoAttackTargetValid(gmChar, 1000, true) {
		t.Fatal("a monster may auto-attack an invisible GM")
	}
	if !monster.AutoAttackTargetValid(watcherChar, 1000, true) {
		t.Fatal("control: the monster may not auto-attack the visible player")
	}

	frames := exchange(t, watcher, encodeTradeRequest(gmID))
	if len(frames) != 1 {
		t.Fatalf("trade request to an invisible GM: frames %x, want TARGET_IS_INCORRECT", testsupport.FrameOpcodes(frames))
	}
	assertStatic(t, frames[0], serverpackets.SystemMessageTargetIncorrect)
	frames = exchange(t, watcher, encodeJoinParty("Admin", 0))
	if len(frames) != 1 {
		t.Fatalf("party invitation to an invisible GM: frames %x, want YOU_HAVE_INVITED_THE_WRONG_TARGET", testsupport.FrameOpcodes(frames))
	}
	assertStatic(t, frames[0], serverpackets.SystemMessageYouHaveInvitedTheWrongTarget)

	showFrames := exchange(t, gm, encodeBuildCmd("hide"))
	if gmChar.Invisible() {
		t.Fatal("a second //hide left the GM invisible")
	}
	requireStealthOnly(t, onlyUserInfoFrame(t, showFrames), hidden)
	if deleted := objectFrames(showFrames, serverpackets.OpcodeDeleteObject, watcherID); len(deleted) != 0 {
		t.Fatal("showing again took the GM off the grid")
	}
	for _, viewer := range []struct {
		name string
		c    *testsupport.ScriptedClient
	}{{"Watcher", watcher}, {"OtherGM", otherGM}} {
		frames := settle(t, viewer.c)
		var shown int
		for _, f := range frames {
			if f[0] != serverpackets.OpcodeCharInfo {
				continue
			}
			if id, invisible := charInfoHidden(t, f); id == gmID {
				shown++
				if invisible != 0 {
					t.Fatalf("%s: CharInfo of the GM shown again has invisible byte %d", viewer.name, invisible)
				}
			}
		}
		if shown != 1 || len(objectFrames(frames, serverpackets.OpcodeDeleteObject, gmID)) != 0 {
			t.Fatalf("%s: frames %x, want one CharInfo of the GM and no DeleteObject", viewer.name, testsupport.FrameOpcodes(frames))
		}
	}
	if !watcherChar.Knows(gmChar) || !monster.Knows(gmChar) || !monster.AutoAttackTargetValid(gmChar, 1000, true) {
		t.Fatal("a GM shown again is still unknown or not auto-attackable")
	}
}

// requireStealthOnly requires two UserInfo frames of one player to differ
// only in the stealth bit of one field.
func requireStealthOnly(t *testing.T, visible, hidden []byte) {
	t.Helper()
	if len(visible) != len(hidden) {
		t.Fatalf("UserInfo lengths %d and %d differ", len(visible), len(hidden))
	}
	diff := -1
	for i := range visible {
		if visible[i] != hidden[i] {
			if diff >= 0 {
				t.Fatalf("UserInfo differs at %d and %d, want only the stealth bit", diff, i)
			}
			diff = i
		}
	}
	// 0x100000 is bit 4 of the field's third little-endian byte.
	if diff < 0 || hidden[diff]^visible[diff] != 0x10 || hidden[diff]&0x10 == 0 {
		t.Fatalf("UserInfo stealth diff at %d, want only %#x set while hidden", diff, abnormalStealth)
	}
}
