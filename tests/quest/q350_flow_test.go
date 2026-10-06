package quest

import (
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	quest001 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q001"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q350"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// Enhance Your Weapon's master, crystals and a monster that charges them
// (Q350_EnhanceYourWeapon.java; soulCrystals.xml: Red Soul Crystal 4629
// stages to 4630 or breaks to 4662; Timak Orc Archer 20584 stages levels
// 0 and 1 at 100 and breaks at 20 out of 1000, needs the crystal skill and
// tries the last hitter's crystal; npcs/20000-20999.xml: level 40).
const (
	q350Name       = "Q350_EnhanceYourWeapon"
	masterID       = 30115
	redCrystal     = 4629
	redStage1      = 4630
	redBroken      = 4662
	timakArcherID  = 20584
	timakLevel     = 40
	absorbSucceed  = 974
	absorbFailed   = 975
	crystalBroke   = 976
	absorbRefused  = 978
	drainSoulSkill = 2096
)

// shippedSeamData is the shipped skill and item tables and soul crystal
// data; callers run datapack.Require first.
var shippedSeamData = sync.OnceValues(func() (*modelskill.Table, *item.Table) {
	dir, _ := datapack.Find()
	root := filepath.Join(dir, "data", "xml")
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

func q350Pages() map[string]string {
	return chatPages(questPages(q350Name, map[string][]string{
		"30115-01.htm": {"30115-04.htm"}, "30115-04.htm": {"30115-09.htm"},
		"30115-09.htm": nil, "30115-03.htm": nil, "30115-21.htm": {"30115-09.htm"},
	}), masterID)
}

// q350Fixture boots Q350 behind Q001 over the shipped soul crystal data,
// skills and crystals, with roll as every script draw.
func q350Fixture(t *testing.T, roll int) []gameservertest.Option {
	t.Helper()
	datapack.Require(t)
	skills, shipped := shippedSeamData()
	crystals, err := xmldata.LoadSoulCrystalData(datapack.Path(t, "data", "xml", "soulCrystals.xml"))
	if err != nil {
		t.Fatal(err)
	}
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{redCrystal, redStage1, redBroken} {
		tmpl, ok := shipped.Get(id)
		if !ok {
			t.Fatalf("shipped item %d missing", id)
		}
		templates = append(templates, tmpl)
	}
	db := sqltest.SharedDB(t)
	return []gameservertest.Option{
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), skills, gamesql.NewCharacterSkillStore(db))),
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithScriptRand(func(int) int { return roll }),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{masterID: script.KindFolk, timakArcherID: script.KindHostile, darinID: script.KindFolk},
			[]script.Listing{{Path: "quest.Q001_LettersOfLove"}, {Path: "quest.Q350_EnhanceYourWeapon"}},
			script.Catalog{
				"quest.Q001_LettersOfLove":     quest001.New,
				"quest.Q350_EnhanceYourWeapon": func() script.Script { return q350.New(crystals) },
			},
		),
	}
}

// timakArcher is the Timak Orc Archer with the fixture's stats.
func timakArcher() *npc.Template {
	return &npc.Template{
		ID: timakArcherID, TemplateID: timakArcherID, Type: "Monster", Name: "Timak Orc Archer", Level: timakLevel,
		HPMax: 1000, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 9, CollisionHeight: 20,
	}
}

// chargeCrystal spawns a Timak Orc Archer beside the player, brings it to
// half HP unless full is set, selects it and uses the crystal on it,
// waiting for the crystal's skill to land.
func (w *dialogWorld) chargeCrystal(t *testing.T, crystal int32, full bool) *npc.Hostile {
	t.Helper()
	return w.chargeCrystalThen(t, crystal, full, nil)
}

// chargeCrystalThen is chargeCrystal running casting once the crystal's
// skill has started, before it lands.
func (w *dialogWorld) chargeCrystalThen(t *testing.T, crystal int32, full bool, casting func()) *npc.Hostile {
	t.Helper()
	mob := w.srv.SpawnHostileNPCTemplateAt(t, timakArcher(), location.Location{X: w.at.X + 20, Y: w.at.Y, Z: w.at.Z})
	if !full {
		onNPCQueue(t, mob, func() { mob.SetHP(mob.MaxHPValue() / 2) })
	}
	w.srv.ReadQueued(t, w.srv.Client)
	w.srv.Client.Send(encodeTutorialAction(mob.ObjectID(), w.at))
	w.srv.ReadQueued(t, w.srv.Client)
	w.srv.Client.Send(encodeUseItem(crystal))
	started := false
	w.srv.AdvanceUntil(t, "Soul Crystal cast starts", func() bool {
		started = started || w.srv.PlayerCastingNow(t, w.player)
		return started
	})
	if casting != nil {
		casting()
	}
	w.srv.AdvanceUntil(t, "Soul Crystal cast ends", func() bool { return !w.srv.PlayerCastingNow(t, w.player) })
	w.srv.ReadQueued(t, w.srv.Client)
	return mob
}

// killAndDie kills mob for the player and lets its dying hooks run,
// returning the system messages and sounds sent meanwhile.
func (w *dialogWorld) killAndDie(t *testing.T, mob *npc.Hostile, settled func() bool) []string {
	t.Helper()
	killer := playerCombatant(t, w.srv, w.player)
	onNPCQueue(t, mob, func() { mob.Kill(killer) })
	var frames [][]byte
	w.srv.AdvanceUntil(t, "dying hooks", func() bool {
		frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
		return settled()
	})
	w.srv.Settle(t)
	frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
	return scriptLines(t, frames)
}

// TestQ350CrystalStagesOnTheChargedMonster plays Enhance Your Weapon: the
// master starts the quest at level 40 and hands out a Red Soul Crystal;
// the crystal, used on a wounded Timak Orc Archer, records the player as
// charging it and its skill registers the player; the archer's death
// stages the crystal three seconds later with a roll under the stage
// chance: the crystal goes, the success line, the stage-1 crystal and the
// item sound follow. It runs on the inline executor and on the real pool.
func TestQ350CrystalStagesOnTheChargedMonster(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		opts []gameservertest.Option
	}{
		{"inline", nil},
		{"pool", []gameservertest.Option{gameservertest.WithRealPool()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			crystalStagesOnTheChargedMonster(t, tc.opts...)
		})
	}
}

func crystalStagesOnTheChargedMonster(t *testing.T, opts ...gameservertest.Option) {
	w := bootSeamQuest(t, timakLevel, q350Pages(), nil, append(q350Fixture(t, 50), opts...)...)
	master := w.spawnFolk(t, "master", masterID, 0, 30, 0).ObjectID()

	if got := w.questWindow(t, master); !shows(got, "30115-01.htm") {
		t.Fatalf("master's window = %q, want 30115-01.htm", got)
	}
	if got := w.bypass(t, "Quest "+q350Name+" 30115-04.htm"); !shows(got, "30115-04.htm") {
		t.Fatalf("start = %q, want 30115-04.htm", got)
	}
	if vars := w.srv.QuestVars(t, w.player, q350Name); vars["<state>"] != "STARTED" || vars["<cond>"] != "1" {
		t.Fatalf("Q350 after the start = %v, want started at condition 1", vars)
	}
	if got := w.bypass(t, "Quest "+q350Name+" 30115-09.htm"); !shows(got, "30115-09.htm") || !slices.Contains(got, itemMessage(serverpackets.SystemMessageEarnedItemS1, redCrystal)) {
		t.Fatalf("crystal link = %q, want 30115-09.htm and the crystal", got)
	}
	if got := w.questWindow(t, master); !shows(got, "30115-03.htm") {
		t.Fatalf("master's window with a crystal = %q, want 30115-03.htm", got)
	}

	crystal := heldObject(t, w, redCrystal)
	mob := w.chargeCrystal(t, crystal, false)
	ai, ok := mob.Absorber(w.player)
	if !ok || !ai.Registered || ai.ItemObjectID != crystal || ai.HPPercent != 0 || !ai.Valid(crystal) {
		t.Fatalf("absorber = %+v, %v; want registered with crystal %d at 0%%", ai, ok, crystal)
	}

	lines := w.killAndDie(t, mob, func() bool { return w.srv.PlayerItemCount(t, w.player, redStage1) == 1 })
	want := []string{
		itemMessage(serverpackets.SystemMessageS1Disappeared, redCrystal),
		"S SystemMessage id=974",
		itemMessage(serverpackets.SystemMessageEarnedItemS1, redStage1),
		"S PlaySound type=0 file=ItemSound.quest_itemget bind=0 obj=0 loc=0,0,0 delay=0",
	}
	if !containsInOrder(lines, want) {
		t.Fatalf("death sent %q, want %q in order", lines, want)
	}
	if n := w.srv.PlayerItemCount(t, w.player, redCrystal); n != 0 {
		t.Fatalf("red crystals after staging = %d, want 0", n)
	}
}

// TestQ350CrystalOutcomes tries the roll's other outcomes and the refusals
// on a quest seeded started: a roll inside the break chance breaks the
// crystal, one past it fails, and a crystal registered on an unwounded
// monster is refused. A player with the crystal but no started quest is
// not recorded at all, by the item use or by the skill.
func TestQ350CrystalOutcomes(t *testing.T) {
	t.Parallel()
	started := func(srv *gameservertest.Server, objID int32) {
		seedJournal(t, srv, objID, []string{q350Name + ":STARTED"})
		srv.GiveItem(t, objID, redCrystal, 1)
	}
	for _, tc := range []struct {
		name    string
		roll    int
		full    bool
		seed    func(*gameservertest.Server, int32)
		want    []string
		held    int32
		charged bool
	}{
		{"break", 110, false, started, []string{
			itemMessage(serverpackets.SystemMessageS1Disappeared, redCrystal),
			"S SystemMessage id=976",
			itemMessage(serverpackets.SystemMessageEarnedItemS1, redBroken),
		}, redBroken, true},
		{"fail", 120, false, started, []string{"S SystemMessage id=975"}, redCrystal, true},
		{"unwounded", 50, true, started, []string{"S SystemMessage id=978"}, redCrystal, true},
		{"not started", 50, false, func(srv *gameservertest.Server, objID int32) {
			srv.GiveItem(t, objID, redCrystal, 1)
		}, nil, redCrystal, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := bootSeamQuest(t, timakLevel, nil, tc.seed, q350Fixture(t, tc.roll)...)
			crystal := heldObject(t, w, redCrystal)
			mob := w.chargeCrystal(t, crystal, tc.full)
			if _, ok := mob.Absorber(w.player); ok != tc.charged {
				t.Fatalf("absorber recorded = %v, want %v", ok, tc.charged)
			}
			var lines []string
			if tc.want == nil {
				// Nothing reacts: let the dying delay pass in full.
				lines = w.killAndDie(t, mob, func() bool { return mob.Dead() })
				w.srv.Advance(t, 3500*time.Millisecond)
				lines = append(lines, scriptLines(t, w.srv.ReadQueued(t, w.srv.Client))...)
			} else {
				lines = w.killAndDie(t, mob, func() bool {
					return w.srv.PlayerItemCount(t, w.player, tc.held) == 1 && (tc.held == redCrystal || w.srv.PlayerItemCount(t, w.player, redCrystal) == 0)
				})
				if tc.held == redCrystal {
					w.srv.Advance(t, 3500*time.Millisecond)
					lines = append(lines, scriptLines(t, w.srv.ReadQueued(t, w.srv.Client))...)
				}
			}
			if !slices.Equal(lines, tc.want) {
				t.Fatalf("death sent %q, want %q", lines, tc.want)
			}
			if n := w.srv.PlayerItemCount(t, w.player, tc.held); n != 1 {
				t.Fatalf("item %d after the death = %d, want 1", tc.held, n)
			}
		})
	}
}

// TestQ350DrainSoulNeedsTheStartedQuest: a player whose quest is aborted
// while the crystal's skill is cast is recorded by the item use but not
// registered by the skill, so the monster's death leaves the crystal as it
// is.
func TestQ350DrainSoulNeedsTheStartedQuest(t *testing.T) {
	t.Parallel()
	w := bootSeamQuest(t, timakLevel, nil, func(srv *gameservertest.Server, objID int32) {
		seedJournal(t, srv, objID, []string{q350Name + ":STARTED"})
		srv.GiveItem(t, objID, redCrystal, 1)
	}, q350Fixture(t, 50)...)
	crystal := heldObject(t, w, redCrystal)
	mob := w.chargeCrystalThen(t, crystal, false, func() {
		w.srv.Client.Send(encodeRequestQuestAbort(350))
		w.srv.AdvanceUntil(t, "quest abort", func() bool { return w.srv.QuestVars(t, w.player, q350Name) == nil })
		if !w.srv.PlayerCastingNow(t, w.player) {
			t.Fatal("the cast landed before the abort")
		}
	})
	ai, ok := mob.Absorber(w.player)
	if !ok || ai.Registered || ai.ItemObjectID != crystal {
		t.Fatalf("absorber = %+v, %v; want recorded with crystal %d, not registered", ai, ok, crystal)
	}
	lines := w.killAndDie(t, mob, mob.Dead)
	w.srv.Advance(t, 3500*time.Millisecond)
	lines = append(lines, scriptLines(t, w.srv.ReadQueued(t, w.srv.Client))...)
	if len(lines) != 0 {
		t.Fatalf("death sent %q, want nothing", lines)
	}
	if n := w.srv.PlayerItemCount(t, w.player, redCrystal); n != 1 {
		t.Fatalf("red crystals after the death = %d, want 1", n)
	}
}

// TestQ350TwoCrystalsResonate: a player holding two Red Soul Crystals who
// charges one on a wounded Timak Orc Archer, which needs the crystal's
// skill, is registered on it; the archer's death sends the resonation line
// and exchanges neither crystal, whatever the roll.
func TestQ350TwoCrystalsResonate(t *testing.T) {
	t.Parallel()
	w := bootSeamQuest(t, timakLevel, nil, func(srv *gameservertest.Server, objID int32) {
		seedJournal(t, srv, objID, []string{q350Name + ":STARTED"})
		srv.GiveItem(t, objID, redCrystal, 1)
		srv.GiveItem(t, objID, redCrystal, 1)
	}, q350Fixture(t, 50)...)
	mob := w.chargeCrystal(t, heldObject(t, w, redCrystal), false)
	if ai, ok := mob.Absorber(w.player); !ok || !ai.Registered {
		t.Fatalf("absorber = %+v, %v; want registered", ai, ok)
	}
	lines := w.killAndDie(t, mob, mob.Dead)
	w.srv.Advance(t, 3500*time.Millisecond)
	lines = append(lines, scriptLines(t, w.srv.ReadQueued(t, w.srv.Client))...)
	if want := []string{"S SystemMessage id=977"}; !slices.Equal(lines, want) {
		t.Fatalf("death sent %q, want %q", lines, want)
	}
	if r, s := w.srv.PlayerItemCount(t, w.player, redCrystal), w.srv.PlayerItemCount(t, w.player, redStage1); r != 2 || s != 0 {
		t.Fatalf("crystals after the death = %d red, %d stage 1; want 2 and 0", r, s)
	}
}

// heldObject returns the object id of the player's one instance of itemID.
func heldObject(t *testing.T, w *dialogWorld, itemID int32) int32 {
	t.Helper()
	var objectID int32
	w.srv.RunScript(t, w.player, q001, func(_ *script.Script, p *script.Player) {
		for _, held := range p.HeldItems() {
			if held.ItemID == itemID {
				objectID = held.ObjectID
			}
		}
	})
	if objectID == 0 {
		t.Fatalf("player holds no item %d", itemID)
	}
	return objectID
}

// containsInOrder reports whether want appears in got as a subsequence.
func containsInOrder(got, want []string) bool {
	i := 0
	for _, g := range got {
		if i < len(want) && g == want[i] {
			i++
		}
	}
	return i == len(want)
}
