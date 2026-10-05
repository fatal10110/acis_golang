package items

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Shipped manor data (aCis_datapack/data/xml): Seed: Dark Coda (item 5016,
// handler Seeds, skill 2097-1 SOW, ONE, hitTime 1800, reuse 10000) is
// Gludio's (castle 1) level 10 seed of crop Dark Coda (5073); Alternative
// Dark Coda Seed (5650) is its alternative; Seed: Lacrima (5024) is Dion's
// (castle 2). Harvester (5125, handler Harvesters) casts Harvesting
// (2098-1, CORPSE_MOB, hitTime 500). Lesser Succubus (npc 20048) is a
// seedable level 20 Monster, and manorAreas.xml gludio_1621_001 is a
// Gludio area. The handlers are Seeds.java and Harvesters.java, the skills
// Sow.java and Harvest.java.
const (
	darkCodaSeedID    int32 = 5016
	altDarkCodaSeedID int32 = 5650
	dionSeedID        int32 = 5024
	harvesterID       int32 = 5125
	darkCodaCropID    int32 = 5073
	succubusNPCID           = 20048
)

// gludioHome is a spawn point inside the Gludio manor area gludio_1621_001.
var gludioHome = location.Location{X: -110000, Y: 110000, Z: -3000}

// shippedManor loads the shipped seed rows, priced from the shipped item
// templates, the manor areas and the Lesser Succubus template once.
var shippedManor = sync.OnceValues(func() (network.ManorConfig, *npc.Template) {
	// Callers run datapack.Require first, so the checkout is present here.
	dir, _ := datapack.Find()
	root := filepath.Join(dir, "data", "xml")
	seeds, err := xmldata.LoadManors(filepath.Join(root, "manors.xml"))
	if err != nil {
		panic(err)
	}
	areas, err := xmldata.LoadManorAreas(filepath.Join(root, "manorAreas.xml"))
	if err != nil {
		panic(err)
	}
	index, skipped := manor.NewAreaIndex(areas)
	if len(skipped) != 0 {
		panic(skipped[0])
	}
	skills, items := shippedData()
	// As the boot's loadManor does: reference prices from the item
	// templates, 1 for an unknown item.
	seeds.ApplyReferencePrices(func(id int32) (int32, bool) {
		tmpl, ok := items.Get(id)
		if !ok {
			return 0, false
		}
		return tmpl.ReferencePrice, true
	})
	npcs, err := xmldata.LoadNPCTemplates(filepath.Join(root, "npcs"), items, skills, zerolog.Nop())
	if err != nil {
		panic(err)
	}
	shipped, ok := npcs.Get(succubusNPCID)
	if !ok {
		panic("Lesser Succubus template missing")
	}
	// The suite runs no decay task: its corpses are kept up by hand (see
	// killForCorpse), so the template schedules none.
	succubus := *shipped
	succubus.CorpseTime = 0
	return network.ManorConfig{Allowed: true, CropRate: 1, Seeds: seeds, Areas: index}, &succubus
})

// bootManorItems boots a level 20 character against the shipped skill table
// and manor data, with the suite's item fixtures plus the shipped seeds,
// harvester and crop. allowed is AllowManor.
func bootManorItems(t *testing.T, allowed bool) (*gameservertest.Server, *npc.Template) {
	t.Helper()
	datapack.Require(t)
	skills, shippedItems := shippedData()
	cfg, succubus := shippedManor()
	cfg.Allowed = allowed
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{darkCodaSeedID, altDarkCodaSeedID, dionSeedID, harvesterID, darkCodaCropID} {
		tmpl, ok := shippedItems.Get(id)
		if !ok {
			t.Fatalf("shipped item %d missing", id)
		}
		templates = append(templates, tmpl)
	}
	db := sqltest.SharedDB(t)
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), skills, gamesql.NewCharacterSkillStore(db))),
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithManor(cfg),
		gameservertest.WithCharacter("Farmer", 20, 0),
		gameservertest.WithWantChars(1))
	return srv, succubus
}

// spawnGludioSuccubus spawns a Lesser Succubus beside the player whose
// spawn point lies in the Gludio manor area.
func spawnGludioSuccubus(t *testing.T, srv *gameservertest.Server, tmpl *npc.Template) *npc.Hostile {
	t.Helper()
	mob := srv.SpawnHostileNPCTemplateHomedAt(t, tmpl, targetCastSpot, gludioHome)
	drainUntilQuiet(t, srv.Client)
	return mob
}

// killForCorpse has a guard kill mob and keeps its corpse up. Nobody fought
// it, so the kill pays no exp that would move the player's level.
func killForCorpse(t *testing.T, srv *gameservertest.Server, mob *npc.Hostile) {
	t.Helper()
	guard := srv.SpawnHostileNPCKindAt(t, "Guard", location.Location{X: targetCastSpot.X + 40, Y: targetCastSpot.Y, Z: targetCastSpot.Z})
	if !mob.TakeDamage(1_000_000, guard) {
		t.Fatal("lethal hit did not kill the monster")
	}
	mob.SetCorpseDeadline(time.Now().Add(time.Minute))
	drainUntilQuiet(t, srv.Client)
}

// systemMessageAmong reports the first of ids found among frames' system
// messages, or 0.
func systemMessageAmong(t *testing.T, frames [][]byte, ids ...int) int {
	t.Helper()
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		got := systemMessageID(t, f)
		for _, id := range ids {
			if got == id {
				return id
			}
		}
	}
	return 0
}

// maxSowTries bounds TestSeedSowsTargetedMonster's retries. Seed: Dark Coda
// (level 10) on a level 20 Lesser Succubus by a level 20 sower lands at 65%,
// so all tries fail in about 0.35^16 < 1e-7 of runs.
const maxSowTries = 16

// sowReuse clears Sowing's 10 s reuse between tries.
const sowReuse = 11 * time.Second

// TestSeedSowsTargetedMonster uses Seed: Dark Coda on targeted Lesser
// Succubi that spawned in a Gludio manor area and have since walked out of
// it: each use starts Sowing (2097) at once on the target and spends the
// seed it carries. A failed roll answers THE_SEED_WAS_NOT_SOWN and leaves
// the monster unsown; the test retries on fresh monsters until a sow lands,
// which answers THE_SEED_WAS_SUCCESSFULLY_SOWN and marks the monster sown by
// the user: the user may claim its harvest and a stranger may not.
func TestSeedSowsTargetedMonster(t *testing.T) {
	t.Parallel()
	srv, succubus := bootManorItems(t, true)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seed := srv.GiveItem(t, objID, darkCodaSeedID, maxSowTries+1)
	startInWorld(t, c)

	var sown *npc.Hostile
	tries := 0
	for sown == nil && tries < maxSowTries {
		if tries > 0 {
			srv.Advance(t, sowReuse)
		}
		tries++
		mob := spawnGludioSuccubus(t, srv, succubus)
		selectAt(t, c, mob.ObjectID(), targetCastSpot)

		c.Send(encodeUseItem(seed, false))
		assertTargetCast(t, c, objID, mob.ObjectID(), 2097, 1800)
		srv.AdvanceUntil(t, "Sowing cast ends", func() bool { return !srv.PlayerCastingNow(t, objID) })
		frames := collectUntilQuiet(t, c)
		switch systemMessageAmong(t, frames, serverpackets.SystemMessageSeedSuccessfullySown, serverpackets.SystemMessageSeedNotSown) {
		case serverpackets.SystemMessageSeedSuccessfullySown:
			if !mob.Seeded() {
				t.Fatal("THE_SEED_WAS_SUCCESSFULLY_SOWN on an unsown monster")
			}
			sown = mob
		case serverpackets.SystemMessageSeedNotSown:
			if mob.Seeded() {
				t.Fatal("THE_SEED_WAS_NOT_SOWN on a sown monster")
			}
			// Sowing's next action is an attack on the monster; end it so
			// the next seed is not queued behind a swing.
			killForCorpse(t, srv, mob)
		default:
			t.Fatalf("try %d: the landed sow answered neither sown nor not sown", tries)
		}
	}
	if sown == nil {
		t.Fatalf("no sow landed in %d tries", maxSowTries)
	}
	t.Logf("sow landed on try %d", tries)
	if got := sown.SeedState().ClaimHarvest(objID+1, nil); got != npc.HarvestNotAuthorized {
		t.Fatalf("stranger ClaimHarvest = %v, want HarvestNotAuthorized", got)
	}
	if got := sown.SeedState().ClaimHarvest(objID, nil); got != npc.HarvestClaimed {
		t.Fatalf("sower ClaimHarvest = %v, want HarvestClaimed", got)
	}
	srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, c)
	assertItemCount(t, srv, objID, seed, maxSowTries+1-tries)
}

// TestSeedRefusals pins Seeds.java's gates in order. None of them casts or
// spends the seed.
func TestSeedRefusals(t *testing.T) {
	t.Parallel()
	type scene struct {
		srv      *gameservertest.Server
		succubus *npc.Template
		objID    int32
	}
	select_ := func(t *testing.T, s scene, mob *npc.Hostile) {
		t.Helper()
		drainUntilQuiet(t, s.srv.Client)
		selectAt(t, s.srv.Client, mob.ObjectID(), targetCastSpot)
	}
	for _, tc := range []struct {
		name    string
		allowed bool
		seedID  int32
		setup   func(t *testing.T, s scene)
		message int
	}{
		{"manor off", false, darkCodaSeedID, func(t *testing.T, s scene) {
			select_(t, s, spawnGludioSuccubus(t, s.srv, s.succubus))
		}, 0},
		{"no target", true, darkCodaSeedID, func(*testing.T, scene) {}, serverpackets.SystemMessageTargetUnavailableForSeeding},
		{"guard target", true, darkCodaSeedID, func(t *testing.T, s scene) {
			select_(t, s, s.srv.SpawnHostileNPCKindAt(t, "Guard", targetCastSpot))
		}, serverpackets.SystemMessageTargetUnavailableForSeeding},
		{"unseedable monster", true, darkCodaSeedID, func(t *testing.T, s scene) {
			tmpl := *s.succubus
			tmpl.Seedable = false
			select_(t, s, s.srv.SpawnHostileNPCTemplateHomedAt(t, &tmpl, targetCastSpot, gludioHome))
		}, serverpackets.SystemMessageTargetUnavailableForSeeding},
		{"spawned outside every manor area", true, darkCodaSeedID, func(t *testing.T, s scene) {
			select_(t, s, s.srv.SpawnHostileNPCTemplateHomedAt(t, s.succubus, targetCastSpot, targetCastSpot))
		}, serverpackets.SystemMessageTargetUnavailableForSeeding},
		{"another castle's seed", true, dionSeedID, func(t *testing.T, s scene) {
			select_(t, s, spawnGludioSuccubus(t, s.srv, s.succubus))
		}, serverpackets.SystemMessageSeedMayNotBeSownHere},
		{"another castle's seed on a corpse", true, dionSeedID, func(t *testing.T, s scene) {
			mob := spawnGludioSuccubus(t, s.srv, s.succubus)
			killForCorpse(t, s.srv, mob)
			select_(t, s, mob)
		}, serverpackets.SystemMessageSeedMayNotBeSownHere},
		{"sown corpse", true, darkCodaSeedID, func(t *testing.T, s scene) {
			mob := spawnGludioSuccubus(t, s.srv, s.succubus)
			mob.SeedState().Sow(s.objID, manor.Seed{CropID: int(darkCodaCropID), Level: 10, CastleID: 1})
			killForCorpse(t, s.srv, mob)
			select_(t, s, mob)
		}, serverpackets.SystemMessageInvalidTarget},
		{"already sown", true, darkCodaSeedID, func(t *testing.T, s scene) {
			mob := spawnGludioSuccubus(t, s.srv, s.succubus)
			mob.SeedState().Sow(s.objID+1, manor.Seed{CropID: int(darkCodaCropID), Level: 10, CastleID: 1})
			select_(t, s, mob)
		}, serverpackets.SystemMessageSeedHasBeenSown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, succubus := bootManorItems(t, tc.allowed)
			c, objID := srv.Client, srv.SoleObjectID(t)
			seed := srv.GiveItem(t, objID, tc.seedID, 3)
			startInWorld(t, c)
			tc.setup(t, scene{srv: srv, succubus: succubus, objID: objID})

			c.Send(encodeUseItem(seed, false))
			assertRefusedAlone(t, srv, objID, tc.message, tc.name)
			assertItemCount(t, srv, objID, seed, 3)
		})
	}
}

// TestHarvesterHarvestsSownCorpse uses the Harvester on the corpse of a
// Lesser Succubus the player sowed with Dark Coda: Harvesting (2098) starts
// on the corpse, the harvester stays, and the player earns the seed's crop
// Dark Coda (5073, not its mature crop), six of them (level 20 is five
// levels above the seed's 10 plus five), told EARNED_S2_S1_S.
func TestHarvesterHarvestsSownCorpse(t *testing.T) {
	t.Parallel()
	srv, succubus := bootManorItems(t, true)
	c, objID := srv.Client, srv.SoleObjectID(t)
	harvester := srv.GiveItem(t, objID, harvesterID, 1)
	startInWorld(t, c)
	mob := spawnGludioSuccubus(t, srv, succubus)
	cfg, _ := shippedManor()
	darkCoda, ok := cfg.Seeds.Seed(darkCodaSeedID)
	if !ok {
		t.Fatal("seed 5016 missing")
	}
	if !mob.SeedState().Sow(objID, darkCoda) {
		t.Fatal("fresh monster already sown")
	}
	killForCorpse(t, srv, mob)
	selectAt(t, c, mob.ObjectID(), targetCastSpot)

	c.Send(encodeUseItem(harvester, false))
	assertTargetCast(t, c, objID, mob.ObjectID(), 2098, 500)
	srv.AdvanceUntil(t, "Harvesting cast ends", func() bool { return !srv.PlayerCastingNow(t, objID) })
	srv.InventoryUpdates.Tick()
	frames := collectUntilQuiet(t, c)
	if systemMessageAmong(t, frames, serverpackets.SystemMessageEarnedS2S1S) == 0 {
		t.Fatal("harvest earned nothing the player was told of")
	}
	if got := srv.PlayerInventory(t, objID).ItemCount(darkCodaCropID, -1, true); got != 6 {
		t.Fatalf("Dark Coda crops = %d, want 6", got)
	}
	if got := srv.PlayerInventory(t, objID).ItemCount(5103, -1, true); got != 0 {
		t.Fatalf("mature Dark Coda = %d, want none", got)
	}
	assertItemCount(t, srv, objID, harvester, 1)
}

// TestHarvesterRefusals pins Harvesters.java's gates: a disabled manor is
// silent, and a target that is no corpse is an INVALID_TARGET. Neither
// casts.
func TestHarvesterRefusals(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		allowed  bool
		corpse   bool
		selected bool
		message  int
	}{
		{"manor off", false, true, true, 0},
		{"no target", true, false, false, serverpackets.SystemMessageInvalidTarget},
		{"living monster", true, false, true, serverpackets.SystemMessageInvalidTarget},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, succubus := bootManorItems(t, tc.allowed)
			c, objID := srv.Client, srv.SoleObjectID(t)
			harvester := srv.GiveItem(t, objID, harvesterID, 1)
			startInWorld(t, c)
			mob := spawnGludioSuccubus(t, srv, succubus)
			if tc.corpse {
				killForCorpse(t, srv, mob)
			}
			if tc.selected {
				selectAt(t, c, mob.ObjectID(), targetCastSpot)
			}

			c.Send(encodeUseItem(harvester, false))
			assertRefusedAlone(t, srv, objID, tc.message, tc.name)
			assertItemCount(t, srv, objID, harvester, 1)
		})
	}
}
