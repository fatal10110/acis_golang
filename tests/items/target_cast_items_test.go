package items

import (
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Shipped target-cast items (aCis_datapack/data/xml/items): Chest Key -
// Grade 8 (5197, handler Keys, skill 2065-1 DELUXE_KEY_UNLOCK, UNLOCKABLE,
// hitTime 500, reuse 3000, consumes one 5197), Red Soul Crystal (4629,
// handler SoulCrystals, skill 2096-1 DRAIN_SOUL, ONE, MP 26, hitTime 1200,
// reuse 3000, consumes nothing) and Golden Spice (6643, handler
// BeastSpices, skill 2188-1 BEAST_FEED, ONE, hitTime 1000, consumes one
// 6643). The handlers are Keys.java, SoulCrystals.java and
// BeastSpices.java; each checks the target itself and then casts through
// tryToCast with no item object id, so the item is spent, if at all, by its
// skill's own consume item.
const (
	chestKeyID    int32 = 5197
	soulCrystalID int32 = 4629
	goldenSpiceID int32 = 6643
	boxChestNPCID       = 18265
)

// targetCastSpot is inside every one of these skills' cast range of the
// fixture spawn, so no approach walk comes first.
var targetCastSpot = location.Location{X: 30, Y: spawnY, Z: spawnZ}

// bootTargetCastItems boots a character against the shipped skill table and
// the suite's item fixtures plus the shipped key, crystal and spice.
func bootTargetCastItems(t *testing.T) *gameservertest.Server {
	t.Helper()
	datapack.Require(t)
	skills, shippedItems := shippedData()
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{chestKeyID, soulCrystalID, goldenSpiceID} {
		tmpl, ok := shippedItems.Get(id)
		if !ok {
			t.Fatalf("shipped item %d missing", id)
		}
		templates = append(templates, tmpl)
	}
	db := sqltest.SharedDB(t)
	return gameservertest.Boot(t,
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), skills, gamesql.NewCharacterSkillStore(db))),
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithCharacter("Newbie", 20, 0),
		gameservertest.WithWantChars(1))
}

// boxChestTemplate is the Treasure Chest npc 18265 (type Chest, level 21,
// aCis_datapack/data/xml/npcs/18000-18999.xml:15449-15451).
func boxChestTemplate() *npc.Template {
	return &npc.Template{
		ID: boxChestNPCID, TemplateID: boxChestNPCID, Type: "Chest", Level: 21,
		HPMax: 1000, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60,
		CollisionRadius: 8, CollisionHeight: 20,
	}
}

// selectAt clicks objectID once and drains the selection answer.
func selectAt(t *testing.T, c *testsupport.ScriptedClient, objectID int32, at location.Location) {
	t.Helper()
	c.Send(encodeAction(objectID, int32(at.X), int32(at.Y), int32(at.Z), false))
	drainUntilQuiet(t, c)
}

// assertTargetCast reads the cast start of skillID by objID on targetID.
// The skills' reuse is not static, so the reuse field is the caster's
// speed-scaled value and is not pinned here.
func assertTargetCast(t *testing.T, c *testsupport.ScriptedClient, objID, targetID, skillID, hitTime int32) {
	t.Helper()
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeMagicSkillUse, "item skill MagicSkillUse")
	caster, target, sid, level, gotHit, _ := decodeMagicSkillUse(frame)
	if caster != objID || target != targetID || sid != skillID || level != 1 || gotHit != hitTime {
		t.Fatalf("MagicSkillUse = caster %d target %d skill %d-%d hit %d, want %d/%d/%d-1/%d",
			caster, target, sid, level, gotHit, objID, targetID, skillID, hitTime)
	}
}

// assertRefusedAlone reads messageID, or nothing at all when messageID is
// 0, and fails on any later frame: the item's own refusal is answered by
// its message alone, with no ActionFailed and no cast.
func assertRefusedAlone(t *testing.T, srv *gameservertest.Server, objID int32, messageID int, what string) {
	t.Helper()
	if messageID != 0 {
		assertStaticSystemMessage(t, srv.Client.Read(), messageID)
	}
	assertNoFrameFor(t, srv.Client, 300*time.Millisecond, what)
	if srv.PlayerCastingNow(t, objID) {
		t.Fatalf("%s started a cast", what)
	}
}

// castCtrl reads the Ctrl modifier the player's last cast request recorded.
func castCtrl(t *testing.T, srv *gameservertest.Server, objID int32) bool {
	t.Helper()
	var ctrl bool
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { ctrl, _ = pc.CastModifiers() })
	return ctrl
}

// TestChestKeyUnlocksTargetedChest uses a Grade 8 key on a targeted box
// chest: the unlock skill starts at once on the chest, the cast spends one
// key through the skill's own consume item, and the unlock handler claims
// the chest, which either opens (dies) or is destroyed.
func TestChestKeyUnlocksTargetedChest(t *testing.T) {
	t.Parallel()
	srv := bootTargetCastItems(t)
	c, objID := srv.Client, srv.SoleObjectID(t)
	key := srv.GiveItem(t, objID, chestKeyID, 3)
	startInWorld(t, c)
	chest := srv.SpawnHostileNPCTemplateAt(t, boxChestTemplate(), targetCastSpot)
	drainUntilQuiet(t, c)
	selectAt(t, c, chest.ObjectID(), targetCastSpot)

	c.Send(encodeUseItem(key, false))
	assertTargetCast(t, c, objID, chest.ObjectID(), 2065, 500)
	srv.AdvanceUntil(t, "unlock claims the chest", chest.Interacted)
	srv.Settle(t)
	if _, inWorld := srv.State.Object(chest.ObjectID()); !chest.Dead() && inWorld {
		t.Fatal("claimed chest neither opened nor destroyed")
	}
	srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, c)
	assertItemCount(t, srv, objID, key, 2)
}

// TestChestKeyRefusals pins Keys.java's gates in order: a seated user is
// told CANT_MOVE_SITTING, an immobile one gets nothing, and a target that
// is not a chest, or a dead chest, or one an unlock already claimed, is an
// INVALID_TARGET. None of them casts or spends a key.
func TestChestKeyRefusals(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		setup   func(t *testing.T, srv *gameservertest.Server, objID int32) int32
		message int
	}{
		{"seated", func(t *testing.T, srv *gameservertest.Server, _ int32) int32 {
			chest := srv.SpawnHostileNPCTemplateAt(t, boxChestTemplate(), targetCastSpot)
			drainUntilQuiet(t, srv.Client)
			selectAt(t, srv.Client, chest.ObjectID(), targetCastSpot)
			sitAndSettle(t, srv)
			return chest.ObjectID()
		}, serverpackets.SystemMessageCannotMoveWhileSitting},
		{"immobilized", func(t *testing.T, srv *gameservertest.Server, objID int32) int32 {
			chest := srv.SpawnHostileNPCTemplateAt(t, boxChestTemplate(), targetCastSpot)
			drainUntilQuiet(t, srv.Client)
			selectAt(t, srv.Client, chest.ObjectID(), targetCastSpot)
			onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.Live.SetImmobilized(true) })
			return chest.ObjectID()
		}, 0},
		{"no target", func(*testing.T, *gameservertest.Server, int32) int32 { return 0 }, serverpackets.SystemMessageInvalidTarget},
		{"monster target", func(t *testing.T, srv *gameservertest.Server, _ int32) int32 {
			mob := srv.SpawnHostileNPCKindAt(t, "Monster", targetCastSpot)
			drainUntilQuiet(t, srv.Client)
			selectAt(t, srv.Client, mob.ObjectID(), targetCastSpot)
			return mob.ObjectID()
		}, serverpackets.SystemMessageInvalidTarget},
		{"dead chest", func(t *testing.T, srv *gameservertest.Server, _ int32) int32 {
			chest := srv.SpawnHostileNPCTemplateAt(t, boxChestTemplate(), targetCastSpot)
			drainUntilQuiet(t, srv.Client)
			selectAt(t, srv.Client, chest.ObjectID(), targetCastSpot)
			chest.MarkDead()
			return chest.ObjectID()
		}, serverpackets.SystemMessageInvalidTarget},
		{"claimed chest", func(t *testing.T, srv *gameservertest.Server, _ int32) int32 {
			chest := srv.SpawnHostileNPCTemplateAt(t, boxChestTemplate(), targetCastSpot)
			drainUntilQuiet(t, srv.Client)
			selectAt(t, srv.Client, chest.ObjectID(), targetCastSpot)
			if !chest.ClaimInteraction() {
				t.Fatal("ClaimInteraction() = false on a fresh chest")
			}
			return chest.ObjectID()
		}, serverpackets.SystemMessageInvalidTarget},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := bootTargetCastItems(t)
			c, objID := srv.Client, srv.SoleObjectID(t)
			key := srv.GiveItem(t, objID, chestKeyID, 3)
			startInWorld(t, c)
			tc.setup(t, srv, objID)

			c.Send(encodeUseItem(key, false))
			assertRefusedAlone(t, srv, objID, tc.message, tc.name)
			assertItemCount(t, srv, objID, key, 3)
		})
	}
}

// TestSoulCrystalCastsOnTarget uses a Red Soul Crystal with Ctrl on a
// targeted monster: Soul Crystal (2096) starts on it with Ctrl as its
// force-use flag, costs its 26 MP, starts its 3 s reuse, and leaves the
// crystal in place.
func TestSoulCrystalCastsOnTarget(t *testing.T) {
	t.Parallel()
	srv := bootTargetCastItems(t)
	c, objID := srv.Client, srv.SoleObjectID(t)
	crystal := srv.GiveItem(t, objID, soulCrystalID, 1)
	startInWorld(t, c)
	mob := srv.SpawnHostileNPCKindAt(t, "Monster", targetCastSpot)
	drainUntilQuiet(t, c)
	selectAt(t, c, mob.ObjectID(), targetCastSpot)
	mp := srv.PlayerCurrentMP(t, objID)
	if mp < 26 {
		t.Fatalf("fixture MP = %d, below Soul Crystal's 26", mp)
	}

	c.Send(encodeUseItem(crystal, true))
	assertTargetCast(t, c, objID, mob.ObjectID(), 2096, 1200)
	if !castCtrl(t, srv, objID) {
		t.Fatal("crystal cast lost the UseItem Ctrl modifier")
	}
	srv.AdvanceUntil(t, "Soul Crystal cast ends", func() bool { return !srv.PlayerCastingNow(t, objID) })
	if got := srv.PlayerCurrentMP(t, objID); got != mp-26 {
		t.Fatalf("MP after Soul Crystal = %d, want %d", got, mp-26)
	}

	// Soul Crystal's 3 s reuse is running: using the crystal again at once
	// is refused with S1_PREPARED_FOR_REUSE, starts no cast and spends no
	// MP.
	c.Send(encodeUseItem(crystal, true))
	for {
		frame := c.ReadWithTimeout(3 * time.Second)
		if frame == nil {
			t.Fatal("repeated crystal use: no S1_PREPARED_FOR_REUSE")
		}
		if frame[0] == serverpackets.OpcodeMagicSkillUse {
			if caster, _, _, _, _, _ := decodeMagicSkillUse(frame); caster == objID {
				t.Fatal("repeated crystal use inside the reuse sent MagicSkillUse")
			}
		}
		if frame[0] == serverpackets.OpcodeSystemMessage && systemMessageID(t, frame) == serverpackets.SystemMessageS1PreparedForReuse {
			assertSystemMessageSkill(t, frame, serverpackets.SystemMessageS1PreparedForReuse, 2096, 1)
			break
		}
	}
	drainUntilQuiet(t, c)
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("repeated crystal use inside the reuse started a cast")
	}
	if got := srv.PlayerCurrentMP(t, objID); got != mp-26 {
		t.Fatalf("MP after the refused repeat = %d, want %d", got, mp-26)
	}
	assertItemCount(t, srv, objID, crystal, 1)
}

// TestSoulCrystalRefusals: with no creature selected the crystal answers
// INVALID_TARGET itself. A creature that is not a Monster passes the
// crystal's own check but Soul Crystal is a DRAIN_SOUL skill, which a
// player may only cast on a Monster: a SiegeGuard (attackable, and let
// through by the ONE target check) is refused with INVALID_TARGET before
// any MP is spent.
func TestSoulCrystalRefusals(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		kind string
	}{
		{"no target", ""},
		{"siege guard target", "SiegeGuard"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := bootTargetCastItems(t)
			c, objID := srv.Client, srv.SoleObjectID(t)
			crystal := srv.GiveItem(t, objID, soulCrystalID, 1)
			startInWorld(t, c)
			if tc.kind != "" {
				target := srv.SpawnHostileNPCKindAt(t, tc.kind, targetCastSpot)
				drainUntilQuiet(t, c)
				selectAt(t, c, target.ObjectID(), targetCastSpot)
			}
			mp := srv.PlayerCurrentMP(t, objID)

			c.Send(encodeUseItem(crystal, true))
			assertRefusedAlone(t, srv, objID, serverpackets.SystemMessageInvalidTarget, tc.name)
			if got := srv.PlayerCurrentMP(t, objID); got != mp {
				t.Fatalf("MP after refused crystal = %d, want %d", got, mp)
			}
			assertItemCount(t, srv, objID, crystal, 1)
		})
	}
}

// TestBeastSpiceFeedsFeedableBeast uses a Golden Spice with Ctrl on a
// targeted FeedableBeast: Golden Spice (2188) starts on it with no force-use
// (the handler passes no modifiers), and the cast spends one spice through
// the skill's own consume item.
func TestBeastSpiceFeedsFeedableBeast(t *testing.T) {
	t.Parallel()
	srv := bootTargetCastItems(t)
	c, objID := srv.Client, srv.SoleObjectID(t)
	spice := srv.GiveItem(t, objID, goldenSpiceID, 2)
	startInWorld(t, c)
	beast := srv.SpawnHostileNPCKindAt(t, "FeedableBeast", targetCastSpot)
	drainUntilQuiet(t, c)
	selectAt(t, c, beast.ObjectID(), targetCastSpot)

	c.Send(encodeUseItem(spice, true))
	assertTargetCast(t, c, objID, beast.ObjectID(), 2188, 1000)
	if castCtrl(t, srv, objID) {
		t.Fatal("spice cast took the UseItem Ctrl modifier as force-use")
	}
	srv.AdvanceUntil(t, "Golden Spice cast ends", func() bool { return !srv.PlayerCastingNow(t, objID) })
	srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, c)
	assertItemCount(t, srv, objID, spice, 1)
}

// TestBeastSpiceRefusesOtherTargets: any target but a FeedableBeast, or
// none, is an INVALID_TARGET with no cast and the spice kept.
func TestBeastSpiceRefusesOtherTargets(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		kind string
	}{
		{"no target", ""},
		{"monster target", "Monster"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := bootTargetCastItems(t)
			c, objID := srv.Client, srv.SoleObjectID(t)
			spice := srv.GiveItem(t, objID, goldenSpiceID, 2)
			startInWorld(t, c)
			if tc.kind != "" {
				target := srv.SpawnHostileNPCKindAt(t, tc.kind, targetCastSpot)
				drainUntilQuiet(t, c)
				selectAt(t, c, target.ObjectID(), targetCastSpot)
			}

			c.Send(encodeUseItem(spice, false))
			assertRefusedAlone(t, srv, objID, serverpackets.SystemMessageInvalidTarget, tc.name)
			assertItemCount(t, srv, objID, spice, 2)
		})
	}
}
