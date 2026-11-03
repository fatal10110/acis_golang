package admin

import (
	"bytes"
	"encoding/binary"
	"slices"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// polymorphNPCID is the NPC //polymorph shows a player as. Its template id
// differs, so a view showing the template id instead of the NPC id fails.
const polymorphNPCID = 20001

// polymorphForm is the form polymorphNPCs' NPC gives a player.
var polymorphForm = player.NPCForm{
	NPCID: polymorphNPCID, Name: "Gremlin", RightHand: 57, LeftHand: 58,
	CollisionRadius: 10, CollisionHeight: 15,
}

// polymorphNPCs holds the one NPC //polymorph knows.
func polymorphNPCs() gameservertest.Option {
	return gameservertest.WithNPCs(npc.NewTable([]*npc.Template{{
		ID: polymorphNPCID, TemplateID: polymorphNPCID + 1, Name: polymorphForm.Name,
		RightHand: int(polymorphForm.RightHand), LeftHand: int(polymorphForm.LeftHand),
		CollisionRadius: polymorphForm.CollisionRadius, CollisionHeight: polymorphForm.CollisionHeight,
	}}))
}

// frameObjectID is the object id a DeleteObject or NPCInfo frame names.
func frameObjectID(f []byte) int32 { return int32(binary.LittleEndian.Uint32(f[1:])) }

// morphView requires frames to show objectID as forgotten and then as
// its NPC form, with no CharInfo of it, and returns the NPCInfo.
func morphView(t *testing.T, who string, frames [][]byte, objectID int32) []byte {
	t.Helper()
	deleted, shown := -1, -1
	for i, f := range frames {
		switch {
		case f[0] == serverpackets.OpcodeDeleteObject && frameObjectID(f) == objectID && deleted < 0:
			deleted = i
		case f[0] == serverpackets.OpcodeNPCInfo && frameObjectID(f) == objectID:
			if shown >= 0 {
				t.Fatalf("%s: frames %x show %d twice", who, testsupport.FrameOpcodes(frames), objectID)
			}
			shown = i
		case f[0] == serverpackets.OpcodeCharInfo:
			if id, _ := charInfoHidden(t, f); id == objectID {
				t.Fatalf("%s: frames %x hold a CharInfo of the polymorphed %d", who, testsupport.FrameOpcodes(frames), objectID)
			}
		}
	}
	if shown < 0 || deleted > shown {
		t.Fatalf("%s: frames %x, want %d's NPCInfo (after its DeleteObject)", who, testsupport.FrameOpcodes(frames), objectID)
	}
	return frames[shown]
}

// lastUserInfo returns the name and title the last UserInfo of frames
// shows.
func lastUserInfo(t *testing.T, who string, frames [][]byte) (name, title string) {
	t.Helper()
	var f []byte
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeUserInfo {
			f = frame
		}
	}
	if f == nil {
		t.Fatalf("%s: frames %x hold no UserInfo", who, testsupport.FrameOpcodes(frames))
	}
	r := wire.NewReader(f[1:])
	for range 5 { // x, y, z, heading, object id
		r.ReadInt32()
	}
	return r.ReadString(), decodeUserInfoAccess(t, f).title
}

// requirePolymorph requires objID to be shown as the NPC npcID, 0 for none.
func requirePolymorph(t *testing.T, srv *gameservertest.Server, objID, npcID int32) {
	t.Helper()
	form := onlineCharacter(t, srv, objID).Polymorph()
	switch {
	case npcID == 0 && form != nil:
		t.Fatalf("%d still shown as %d", objID, form.NPCID)
	case npcID != 0 && (form == nil || *form != polymorphForm):
		t.Fatalf("%d form = %+v, want %+v", objID, form, polymorphForm)
	}
}

// TestAdminPolymorphPlayer pins //polymorph on a selected player
// (AdminPolymorph, Creature.polymorph, Player.polymorph): everyone around
// forgets it (decayMe) and sees it again as the NPC (spawnMe, PcMorphInfo
// in Player.sendInfo), then it reads the NPC's name and the title
// "Morphed" (UserInfo). Selected again, a second //polymorph is a toggle,
// whatever follows: the player is forgotten and shown again by its
// CharInfo, and reads its own name and title.
func TestAdminPolymorphPlayer(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel, polymorphNPCs())
	gm := srv.Client
	enterWorld(t, gm)
	target, targetID := addPlayer(t, srv, "player2", "Target", userLevel)
	watcher, _ := addPlayer(t, srv, "player3", "Watcher", userLevel)
	exchange(t, gm, encodeAction(targetID))
	settle(t, target)
	settle(t, watcher)

	gmFrames := exchange(t, gm, encodeBuildCmd("polymorph "+strconv.Itoa(polymorphNPCID)))
	requirePolymorph(t, srv, targetID, polymorphNPCID)
	want := bytes.Clone(serverpackets.FramePcMorphInfo(onlineCharacter(t, srv, targetID), polymorphForm).Bytes()[2:])
	if got := morphView(t, "Watcher", settle(t, watcher), targetID); !bytes.Equal(got, want) {
		t.Fatalf("Watcher PcMorphInfo = %x, want %x", got, want)
	}
	if got := morphView(t, "GM", gmFrames, targetID); !bytes.Equal(got, want) {
		t.Fatalf("GM PcMorphInfo = %x, want %x", got, want)
	}
	if got := binary.LittleEndian.Uint32(want[5:]); got != polymorphNPCID+1000000 {
		t.Fatalf("PcMorphInfo npc id = %d, want the NPC id %d", got, polymorphNPCID+1000000)
	}
	if name, title := lastUserInfo(t, "Target", settle(t, target)); name != "Gremlin" || title != "Morphed" {
		t.Fatalf("Target UserInfo name, title = %q, %q, want Gremlin, Morphed", name, title)
	}

	// Forgetting the target dropped the GM's selection (removeKnownObject).
	exchange(t, gm, encodeAction(targetID))
	settle(t, watcher)
	exchange(t, gm, encodeBuildCmd("polymorph whatever"))
	requirePolymorph(t, srv, targetID, 0)
	frames := settle(t, watcher)
	requireOneCharInfo(t, "Watcher", frames, targetID, 0)
	deleted := slices.IndexFunc(frames, func(f []byte) bool {
		return f[0] == serverpackets.OpcodeDeleteObject && frameObjectID(f) == targetID
	})
	shown := slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeCharInfo })
	if deleted < 0 || deleted > shown {
		t.Fatalf("Watcher frames %x, want the Target forgotten before its CharInfo", testsupport.FrameOpcodes(frames))
	}
	if name, title := lastUserInfo(t, "Target", settle(t, target)); name != "Target" || title != "" {
		t.Fatalf("Target UserInfo name, title = %q, %q, want its own", name, title)
	}
}

// TestAdminPolymorphRefusals pins what //polymorph answers on gm itself
// with nothing selected: an id no NPC has is refused
// (APPLICANT_INFORMATION_INCORRECT), a missing or unreadable one gets the
// usage, a level the table does not allow is refused; none takes a form.
func TestAdminPolymorphRefusals(t *testing.T) {
	t.Parallel()
	t.Run("input", func(t *testing.T) {
		t.Parallel()
		srv, gmID := bootAdmin(t, adminLevel, polymorphNPCs())
		enterWorld(t, srv.Client)
		frames := exchange(t, srv.Client, encodeBuildCmd("polymorph 99999"))
		if len(frames) != 1 {
			t.Fatalf("unknown npc frames = %x, want one message", testsupport.FrameOpcodes(frames))
		}
		assertStatic(t, frames[0], serverpackets.SystemMessageApplicantInformationIncorrect)
		for _, line := range []string{"polymorph", "polymorph abc", "polymorph 1x"} {
			assertTexts(t, exchange(t, srv.Client, encodeBuildCmd(line)), "Usage: //polymorph npcId")
		}
		requirePolymorph(t, srv, gmID, 0)
	})
	t.Run("access", func(t *testing.T) {
		t.Parallel()
		srv, gmID := bootAdmin(t, adminLevel, polymorphNPCs(), gameservertest.WithAdmin(adminDataRestricting(t, "admin_polymorph")))
		enterWorld(t, srv.Client)
		assertTexts(t, exchange(t, srv.Client, encodeBuildCmd("polymorph "+strconv.Itoa(polymorphNPCID))), "You don't have the access right to use this command.")
		requirePolymorph(t, srv, gmID, 0)
	})
}

// TestAdminPolymorphSelfSeenByNewcomer pins a polymorphed gm with nothing
// selected as the players around keep seeing it: one entering its range is
// shown its NPC form (Player.sendInfo), and its info refresh, a walk/run
// toggle, sends the NPC form instead of its CharInfo
// (Player.broadcastUserInfo).
func TestAdminPolymorphSelfSeenByNewcomer(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel, polymorphNPCs())
	gm := srv.Client
	enterWorld(t, gm)
	frames := exchange(t, gm, encodeBuildCmd("polymorph "+strconv.Itoa(polymorphNPCID)))
	requirePolymorph(t, srv, gmID, polymorphNPCID)
	if name, title := lastUserInfo(t, "GM", frames); name != "Gremlin" || title != "Morphed" {
		t.Fatalf("GM UserInfo name, title = %q, %q, want Gremlin, Morphed", name, title)
	}

	srv.SeedCharacterFor(t, "player2", "Newcomer", 1, 0)
	newcomer := srv.DialClient(t, "player2", 1)
	frames = append(enterWorldFrames(t, newcomer), settle(t, newcomer)...)
	morphView(t, "Newcomer", frames, gmID)

	exchange(t, gm, encodeActionUse(actionWalkRun))
	frames = settle(t, newcomer)
	got := morphView(t, "Newcomer after walk/run", frames, gmID)
	want := serverpackets.FramePcMorphInfo(onlineCharacter(t, srv, gmID), polymorphForm).Bytes()[2:]
	if !bytes.Equal(got, want) {
		t.Fatalf("refreshed PcMorphInfo = %x, want %x", got, want)
	}
}

// TestAdminPolymorphDismountsFirst pins the dismount //polymorph starts
// with on a mounted player, taking a form or leaving it.
func TestAdminPolymorphDismountsFirst(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel, polymorphNPCs())
	gm := srv.Client
	enterWorld(t, gm)
	for _, step := range []struct {
		line  string
		npcID int32
	}{{"polymorph " + strconv.Itoa(polymorphNPCID), polymorphNPCID}, {"polymorph", 0}} {
		exchange(t, gm, encodeBuildCmd("ride strider"))
		frames := exchange(t, gm, encodeBuildCmd(step.line))
		if got := testsupport.FrameOpcodes(frames); len(got) <= len(dismountOrder) || !bytes.Equal(got[:len(dismountOrder)], dismountOrder) {
			t.Fatalf("//%s frames = %x, want the dismount %x first", step.line, got, dismountOrder)
		}
		if liveCharacter(t, srv, gmID).Mounted() {
			t.Fatalf("//%s left the GM mounted", step.line)
		}
		requirePolymorph(t, srv, gmID, step.npcID)
	}
}

// TestAdminPolymorphSummonRefused pins //polymorph on a summon: only an
// NPC or a player takes a form (Creature.polymorph), so even the summon's
// own NPC is refused, and a missing id still gets the usage.
func TestAdminPolymorphSummonRefused(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel, summonAdminOptions(t)...)
	collar := srv.GiveItem(t, gmID, infoCollarID, 1)
	gm := srv.Client
	enterWorld(t, gm)
	pet := callPet(t, srv, gm, gmID, collar)
	exchange(t, gm, encodeAction(pet.ObjectID()))

	frames := exchange(t, gm, encodeBuildCmd("polymorph "+strconv.Itoa(infoPetID)))
	if len(frames) != 1 {
		t.Fatalf("summon frames = %x, want one message", testsupport.FrameOpcodes(frames))
	}
	assertStatic(t, frames[0], serverpackets.SystemMessageApplicantInformationIncorrect)
	assertTexts(t, exchange(t, gm, encodeBuildCmd("polymorph")), "Usage: //polymorph npcId")
	requirePolymorph(t, srv, gmID, 0)
}
