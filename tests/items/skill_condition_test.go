package items

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Shipped items whose attached skills carry a <cond>: 5250 (Greater
// Compressed Package of Soulshots: No-grade, skill 2104-1, AI cast, cond
// msgId 129 invSize=1 AND weight=3) and 8193 (Fisherman's Potion - Green,
// skill 2274-1, instant potion, cond msgId 113 addName, flying=False AND
// active_skill_id_lvl=1315,4).
const (
	soulshotPackageID int32 = 5250
	fishermanPotionID int32 = 8193
	// ballastItemID is a synthetic non-stackable item heavy enough to push
	// any character into the top weight-penalty band.
	ballastItemID int32 = 9750
	// fillerItemID is a synthetic weightless non-stackable item that takes
	// one inventory slot per instance.
	fillerItemID int32 = 9751
)

var shippedData = sync.OnceValues(func() (*modelskill.Table, *item.Table) {
	_, thisFile, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "aCis_datapack", "data", "xml")
	if _, err := os.Stat(root); err != nil {
		return nil, nil
	}
	skills, err := xmldata.LoadSkillDefinitions(filepath.Join(root, "skills"), zerolog.Nop())
	if err != nil {
		panic(err)
	}
	items, err := xmldata.LoadItemTemplates(filepath.Join(root, "items"), zerolog.Nop())
	if err != nil {
		panic(err)
	}
	return skills, items
})

// bootShippedConditionItems boots a character against the shipped skill
// table and the suite's item fixtures plus the shipped condition items.
// known seeds learned skills before the character enters the world.
func bootShippedConditionItems(t *testing.T, known map[int]int) *gameservertest.Server {
	t.Helper()
	skills, shippedItems := shippedData()
	if skills == nil {
		t.Skip("aCis_datapack not checked out near the module root")
	}
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{soulshotPackageID, fishermanPotionID} {
		tmpl, ok := shippedItems.Get(id)
		if !ok {
			t.Fatalf("shipped item %d missing", id)
		}
		templates = append(templates, tmpl)
	}
	templates = append(templates,
		&item.Template{ID: ballastItemID, Name: "Ballast", Kind: item.KindEtcItem, Duration: -1, Weight: 10_000_000, EtcItem: &item.EtcItemDetail{Type: item.EtcItemMaterial}},
		&item.Template{ID: fillerItemID, Name: "Filler", Kind: item.KindEtcItem, Duration: -1, EtcItem: &item.EtcItemDetail{Type: item.EtcItemMaterial}},
	)
	db := sqltest.SharedDB(t)
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), skills, gamesql.NewCharacterSkillStore(db))),
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithWeightLimitMultiplier(1),
		gameservertest.WithCharacter("Newbie", 20, 0),
		gameservertest.WithWantChars(1))
	for id, level := range known {
		if err := srv.KnownSkills.SetKnownSkill(context.Background(), srv.SoleObjectID(t), 0, id, level); err != nil {
			t.Fatalf("seed known skill %d: %v", id, err)
		}
	}
	return srv
}

// assertConditionRejectionOnly reads the rejection message and then fails
// on any further frame: a failed skill <cond> answers with its message
// alone, with no ActionFailed and no cast broadcast.
func assertConditionRejectionOnly(t *testing.T, srv *gameservertest.Server, what string) {
	t.Helper()
	if frame := srv.Client.ReadWithTimeout(300 * time.Millisecond); frame != nil {
		t.Fatalf("%s sent extra opcode %#x after the condition message", what, frame[0])
	}
}

// TestSoulshotPackageConditionGatesAICast pins ItemSkills.java:59-62 for
// the AI-cast half: skill 2104's <cond msgId="129"> (invSize=1, weight=3)
// is evaluated against the user before the reuse check, and a failure sends
// SLOTS_FULL (129) alone and consumes nothing. With a free slot and no
// weight penalty the package opens.
func TestSoulshotPackageConditionGatesAICast(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		fill   func(t *testing.T, srv *gameservertest.Server, objID int32)
		reject bool
	}{
		{"free slot and light", func(*testing.T, *gameservertest.Server, int32) {}, false},
		{"weight penalty 3 or more", func(t *testing.T, srv *gameservertest.Server, objID int32) {
			srv.GiveItem(t, objID, ballastItemID, 1)
		}, true},
		{"no free inventory slot", func(t *testing.T, srv *gameservertest.Server, objID int32) {
			for range 80 {
				srv.GiveItem(t, objID, fillerItemID, 1)
			}
		}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := bootShippedConditionItems(t, nil)
			c, objID := srv.Client, srv.SoleObjectID(t)
			pkg := srv.GiveItem(t, objID, soulshotPackageID, 1)
			tt.fill(t, srv, objID)
			// An overloaded character's EnterWorld burst carries extra
			// weight-penalty packets, so drain it rather than pin it.
			c.Send(encodeRequestGameStart(0))
			c.Send(encodeEnterWorld())
			drainUntilQuiet(t, c)

			c.Send(encodeUseItem(pkg, false))
			if tt.reject {
				assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageSlotsFull)
				assertConditionRejectionOnly(t, srv, tt.name)
				srv.FlushItems(t)
				if inst := mustFindItem(t, srv, objID, pkg); inst.Count != 1 {
					t.Fatalf("package count after condition rejection = %d, want 1", inst.Count)
				}
				return
			}
			assertMagicSkillUseSelf(t, c.Read(), objID, 2104, 1, 0, 0)
		})
	}
}

// TestFishermanPotionReadsKnownFishingMasteryLevel pins skill 2274-1's
// active_skill_id_lvl="1315,4" on the instant-potion path: the condition
// reads the level of Fishing Mastery (1315) the user has learned. Level 3
// rejects with S1_CANNOT_BE_USED naming the potion skill at level 1 and
// consumes nothing; level 4 drinks it.
func TestFishermanPotionReadsKnownFishingMasteryLevel(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		mastery int
		reject  bool
	}{{3, true}, {4, false}} {
		t.Run(fmt.Sprintf("mastery level %d", tt.mastery), func(t *testing.T) {
			t.Parallel()
			srv := bootShippedConditionItems(t, map[int]int{1315: tt.mastery})
			c, objID := srv.Client, srv.SoleObjectID(t)
			potion := srv.GiveItem(t, objID, fishermanPotionID, 2)
			startInWorld(t, c)
			drainUntilQuiet(t, c)

			c.Send(encodeUseItem(potion, false))
			if tt.reject {
				assertSystemMessageSkill(t, c.Read(), serverpackets.SystemMessageS1CannotBeUsed, 2274, 1)
				assertConditionRejectionOnly(t, srv, "fisherman potion")
				srv.FlushItems(t)
				if inst := mustFindItem(t, srv, objID, potion); inst.Count != 2 {
					t.Fatalf("potion count after condition rejection = %d, want 2", inst.Count)
				}
				return
			}
			assertMagicSkillUseSelf(t, c.Read(), objID, 2274, 1, 0, 0)
		})
	}
}
