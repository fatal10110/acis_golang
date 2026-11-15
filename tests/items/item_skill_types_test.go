package items

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Reference: the CREATE_ITEM, CHANGE_APPEARANCE and SPAWN skill types.
//
//   - L2SkillCreateItem.useSkill: a living Playable caster gets
//     addItem(Rnd.get(create_item_id), create_item_count +
//     Rnd.get(random_count), true): YOU_PICKED_UP_S2_S1 (29) for more than
//     one unit (Player.java:1806-1852). Item 5250 (Greater Compressed
//     Package of Soulshots: No-grade) casts 2104-1: 1000 of item 1835.
//   - L2SkillAppearance.useSkill: every Player target gets faceId,
//     hairColorId and hairStyleId where not negative, then
//     broadcastUserInfo: UserInfo to itself, CharInfo to the players around
//     it. Player.store's UPDATE_CHARACTER persists face, hairStyle and
//     hairColor. Item 8664 (Masking Oil) casts 2291-1 Facelifting Potion,
//     faceId 0.
//   - L2SkillSpawn.useSkill: a living caster places new Spawn(npcId) at its
//     own position (Spawn.setLoc takes the geo height there and the
//     caster's heading), doSpawn(false), then Npc.scheduleDespawn
//     (Npc.java:842: deleteMe unless decayed) after despawnDelay ms. Item
//     6389 (Squash Seed) casts 2003-1: Young Squash (12774) for 120000 ms.
const (
	soulshotNoGradeID int32 = 1835
	maskingOilID      int32 = 8664
	squashSeedID      int32 = 6389
	youngSquashNPCID        = 12774
	squashDespawn           = 120 * time.Second
)

// shippedNPCs loads the shipped NPC templates once.
var shippedNPCs = sync.OnceValue(func() *npc.Table {
	// Callers run datapack.Require first, so the checkout is present here.
	dir, _ := datapack.Find()
	skills, items := shippedData()
	npcs, err := xmldata.LoadNPCTemplates(filepath.Join(dir, "data", "xml", "npcs"), items, skills, zerolog.Nop())
	if err != nil {
		panic(err)
	}
	return npcs
})

// bootItemSkillTypes boots a level 20 character against the shipped skill
// table and the suite's item fixtures plus the shipped items these skills
// use and create, with any extra options.
func bootItemSkillTypes(t *testing.T, extra ...gameservertest.Option) *gameservertest.Server {
	t.Helper()
	datapack.Require(t)
	skills, shippedItems := shippedData()
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{soulshotPackageID, soulshotNoGradeID, maskingOilID, squashSeedID} {
		tmpl, ok := shippedItems.Get(id)
		if !ok {
			t.Fatalf("shipped item %d missing", id)
		}
		templates = append(templates, tmpl)
	}
	db := sqltest.SharedDB(t)
	return gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), skills, gamesql.NewCharacterSkillStore(db))),
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithCharacter("Newbie", 20, 0),
		gameservertest.WithWantChars(1),
	}, extra...)...)
}

// useItemSkill uses itemObjID on c, requires the cast's MagicSkillUse of
// skillID, lets the cast finish and returns every frame c got from then
// until it went quiet.
func useItemSkill(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, objID, itemObjID, skillID int32) [][]byte {
	t.Helper()
	c.Send(encodeUseItem(itemObjID, false))
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeMagicSkillUse, "MagicSkillUse")
	r := wire.NewReader(frame[1:])
	r.ReadInt32() // caster
	r.ReadInt32() // target
	if got := r.ReadInt32(); got != skillID {
		t.Fatalf("MagicSkillUse skill = %d, want %d", got, skillID)
	}
	srv.AdvanceUntil(t, "item skill cast ends", func() bool { return !srv.PlayerCastingNow(t, objID) })
	return collectUntilQuiet(t, c)
}

// TestSoulshotPackageCreatesSoulshots opens item 5250: skill 2104 creates
// 1000 No-grade soulshots, named as picked up with the count, and the
// stack reaches the database.
func TestSoulshotPackageCreatesSoulshots(t *testing.T) {
	t.Parallel()
	srv := bootItemSkillTypes(t)
	c, objID := srv.Client, srv.SoleObjectID(t)
	pkg := srv.GiveItem(t, objID, soulshotPackageID, 1)
	startInWorld(t, c)

	frames := useItemSkill(t, srv, c, objID, pkg, 2104)
	messages := systemMessages(frames)
	if len(messages) != 1 {
		t.Fatalf("system messages = %d (frames %x), want the one pickup line", len(messages), testsupport.FrameOpcodes(frames))
	}
	assertGrantMessage(t, messages[0], serverpackets.SystemMessageYouPickedUpS2S1, itemNameParam(soulshotNoGradeID), itemNumberParam(1000))
	if got := carriedCount(t, srv, objID, soulshotNoGradeID); got != 1000 {
		t.Fatalf("carried soulshots = %d, want 1000", got)
	}
	if got := carriedCount(t, srv, objID, soulshotPackageID); got != 0 {
		t.Fatalf("packages left = %d, want 0", got)
	}

	srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, c)
	srv.FlushItems(t)
	persisted := 0
	for _, inst := range persistedItems(t, srv, objID) {
		if inst.TemplateID == soulshotNoGradeID {
			persisted += inst.Count
		}
	}
	if persisted != 1000 {
		t.Fatalf("persisted soulshots = %d, want 1000", persisted)
	}
}

// appearanceAfter decodes the face, hair style and hair color the last
// UserInfo (self) or the last CharInfo of objID (another player) among
// frames shows.
func appearanceAfter(t *testing.T, frames [][]byte, opcode byte, objID int32) (face, hairStyle, hairColor int32) {
	t.Helper()
	var last []byte
	for _, f := range frames {
		if f[0] != opcode {
			continue
		}
		r := wire.NewReader(f[1:])
		for range 4 { // x, y, z, heading (UserInfo) or vehicle (CharInfo)
			r.ReadInt32()
		}
		if r.ReadInt32() == objID {
			last = f
		}
	}
	if last == nil {
		t.Fatalf("frames %x hold no %#x of %d", testsupport.FrameOpcodes(frames), opcode, objID)
	}
	r := wire.NewReader(last[1:])
	for range 5 {
		r.ReadInt32()
	}
	r.ReadString()
	if opcode == serverpackets.OpcodeUserInfo {
		for range 4 { // race, sex, class, level
			r.ReadInt32()
		}
		r.ReadInt64()
		for range 6 + 8 + 2*17 { // STR..MEN, vitals/SP/weight/slots, paperdoll ids
			r.ReadInt32()
		}
		for _, n := range []int{14, 12, 4} { // augmentation pairs, with the hand ids between
			for range n {
				r.ReadUint16()
			}
			if n != 4 {
				r.ReadInt32()
			}
		}
		for range 12 + 8 { // combat stats, PvP flag, karma, speeds
			r.ReadInt32()
		}
	} else {
		for range 3 + 12 { // race, sex, class, paperdoll ids
			r.ReadInt32()
		}
		for i, n := range []int{4, 12, 4} { // padding, with the hand augmentation ids between
			for range n {
				r.ReadUint16()
			}
			if i < 2 {
				r.ReadInt32()
			}
		}
		for range 6 + 8 { // PvP flag, karma, attack speeds, speeds
			r.ReadInt32()
		}
	}
	for range 4 { // speed and attack speed multipliers, collision radius and height
		r.ReadFloat64()
	}
	hairStyle, hairColor, face = r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	return face, hairStyle, hairColor
}

// TestMaskingOilChangesFaceSeenByAll drinks item 8664 on a character with
// face 2, hair style 3 and hair color 1: skill 2291 gives it face 0 and
// keeps its hair. Its own UserInfo and the CharInfo a player beside it gets
// both show the new face, and the next save persists it.
func TestMaskingOilChangesFaceSeenByAll(t *testing.T) {
	t.Parallel()
	srv := bootItemSkillTypes(t)
	observer := srv.Client
	startInWorld(t, observer)

	masked := srv.SeedCharacterFor(t, "player2", "Masked", 20, 0)
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET face = 2, hairStyle = 3, hairColor = 1 WHERE obj_Id = ?", masked.ID); err != nil {
		t.Fatalf("seed appearance: %v", err)
	}
	oil := srv.GiveItem(t, masked.ID, maskingOilID, 2)
	c := srv.DialClient(t, "player2", 1)
	startInWorld(t, c)
	drainUntilQuiet(t, c)
	drainUntilQuiet(t, observer)

	frames := useItemSkill(t, srv, c, masked.ID, oil, 2291)
	if face, style, color := appearanceAfter(t, frames, serverpackets.OpcodeUserInfo, masked.ID); face != 0 || style != 3 || color != 1 {
		t.Fatalf("own UserInfo face/style/color = %d/%d/%d, want 0/3/1", face, style, color)
	}
	seen := collectUntilQuiet(t, observer)
	if face, style, color := appearanceAfter(t, seen, serverpackets.OpcodeCharInfo, masked.ID); face != 0 || style != 3 || color != 1 {
		t.Fatalf("observer CharInfo face/style/color = %d/%d/%d, want 0/3/1", face, style, color)
	}

	srv.TickAutosave(t)
	var face, style, color int
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT face, hairStyle, hairColor FROM characters WHERE obj_Id = ?", masked.ID).Scan(&face, &style, &color); err != nil {
		t.Fatalf("read appearance: %v", err)
	}
	if face != 0 || style != 3 || color != 1 {
		t.Fatalf("persisted face/style/color = %d/%d/%d, want 0/3/1", face, style, color)
	}
}

// TestSquashSeedSpawnsYoungSquashThatDespawns plants item 6389: skill 2003
// places a Young Squash where the player stands, facing its way, which is
// deleted once 120 seconds have passed.
func TestSquashSeedSpawnsYoungSquashThatDespawns(t *testing.T) {
	t.Parallel()
	srv := bootItemSkillTypes(t, gameservertest.WithNPCs(shippedNPCs()), gameservertest.WithNpcSpawns(nil))
	c, objID := srv.Client, srv.SoleObjectID(t)
	seed := srv.GiveItem(t, objID, squashSeedID, 2)
	startInWorld(t, c)
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player not in the world")
	}
	player, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatal("player not online")
	}
	px, py, pz := player.Position()
	heading := player.Heading()

	frames := useItemSkill(t, srv, c, objID, seed, 2003)
	var squashID int32
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeNPCInfo {
			continue
		}
		r := wire.NewReader(f[1:])
		id, tmpl := r.ReadInt32(), r.ReadInt32()
		r.ReadInt32() // attackable
		x, y, z, h := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
		if tmpl != youngSquashNPCID+1000000 {
			t.Fatalf("NpcInfo template = %d, want Young Squash", tmpl-1000000)
		}
		if int(x) != px || int(y) != py || int(z) != pz || int(h) != heading {
			t.Fatalf("Young Squash at (%d,%d,%d) heading %d, want the player's (%d,%d,%d) heading %d", x, y, z, h, px, py, pz, heading)
		}
		squashID = id
	}
	if squashID == 0 {
		t.Fatalf("frames %x show no NpcInfo", testsupport.FrameOpcodes(frames))
	}
	if got := carriedCount(t, srv, objID, squashSeedID); got != 1 {
		t.Fatalf("seeds left = %d, want 1", got)
	}
	if _, ok := srv.State.Object(squashID); !ok {
		t.Fatal("Young Squash not in the world")
	}

	srv.Advance(t, squashDespawn-time.Second)
	drainUntilQuiet(t, c)
	if _, ok := srv.State.Object(squashID); !ok {
		t.Fatal("Young Squash gone before its despawn delay")
	}
	srv.Advance(t, time.Second)
	gone := collectUntilQuiet(t, c)
	if _, ok := srv.State.Object(squashID); ok {
		t.Fatal("Young Squash still in the world after its despawn delay")
	}
	deleted := false
	for _, f := range gone {
		if f[0] == serverpackets.OpcodeDeleteObject && int32FromFrame(f, 1) == squashID {
			deleted = true
		}
	}
	if !deleted {
		t.Fatalf("frames %x hold no DeleteObject of the Young Squash", testsupport.FrameOpcodes(gone))
	}
}

// int32FromFrame reads the little-endian int32 at offset of frame.
func int32FromFrame(frame []byte, offset int) int32 {
	return wire.NewReader(frame[offset:]).ReadInt32()
}

// TestCapsulePickedEmptyRowFindsNothingInside opens a capsule whose first
// row is a lone chance field, a row of no items that always wins: nothing
// is created, the capsule is spent, and NOTHING_INSIDE_THAT answers.
func TestCapsulePickedEmptyRowFindsNothingInside(t *testing.T) {
	t.Parallel()
	srv, objID, frames := openCapsule(t, "100;20,5,100", 5)
	assertOneCapsuleSpent(t, srv, objID)

	messages := systemMessages(frames)
	if len(messages) != 1 {
		t.Fatalf("system messages = %d, want NOTHING_INSIDE_THAT alone", len(messages))
	}
	assertStaticSystemMessage(t, messages[0], serverpackets.SystemMessageNothingInsideThat)
	if got := carriedCount(t, srv, objID, capsulePotionID); got != 0 {
		t.Fatalf("carried potions = %d, want none from the empty row", got)
	}
}
