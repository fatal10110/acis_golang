package quest

import (
	"strconv"
	"testing"
	"time"

	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	quest001 "github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q001"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q230"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// Test of the Summoner's first arcana master and its duel
// (Q230_TestOfTheSummoner.java).
const (
	q230Name         = "Q230_TestOfTheSummoner"
	q230Path         = "quest.Q230_TestOfTheSummoner"
	almorsID         = 30635
	pakoID           = 27102
	beginnerArcana   = 3353
	almorsArcana     = 3354
	crystalProgress  = 3360
	crystalInDuel    = 3361
	crystalFoul      = 3362
	crystalDefeat    = 3363
	crystalVictory   = 3364
	duelBuffSkill    = 4126
	pakoStartsDuel   = "Whhiisshh!"
	pakoCallsFoul    = "Rule violation!"
	pakoLosesTheDuel = "I'm sorry, Lord!"
)

// q230ItemTemplates is the shared item catalog plus the trial's items, as
// stackable quest items.
func q230ItemTemplates() gameservertest.Option {
	templates := gameservertest.ItemTemplates().All()
	for id := int32(3336); id <= 3389; id++ {
		templates = append(templates, &item.Template{
			ID: id, Name: "Summoner Item " + strconv.Itoa(int(id)), Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{Type: item.EtcItemQuest},
		})
	}
	return gameservertest.WithItemTemplates(item.NewTable(templates))
}

func q230Pages() map[string]string {
	return chatPages(questPages(q230Name, map[string][]string{
		"30635-01.htm": {"30635-02.htm"}, "30635-02.htm": nil, "30635-03.htm": {"30635-04.htm"}, "30635-04.htm": nil,
		"30635-05.htm": nil, "30635-06.htm": nil, "30635-07.htm": nil, "30635-08.htm": nil, "30635-09.htm": nil,
	}), almorsID)
}

// q230World boots a wizard of level 39 whose trial is started with Almors'
// duel in state almors, holding items (id to count), with Pako the Cat and
// a servitor as templates. The trial runs behind Letters of Love, the
// fixtures' helper quest.
func q230World(t *testing.T, almors int, items map[int32]int32, opts ...gameservertest.Option) *dialogWorld {
	t.Helper()
	return bootSeamQuest(t, 39, q230Pages(), func(srv *gameservertest.Server, objID int32) {
		seedJournal(t, srv, objID, []string{q230Name + ":STARTED"})
		insertJournal(t, srv.DB, journalRow{objID, q230Name, "Almors", val(strconv.Itoa(almors))})
		for id, n := range items {
			srv.GiveItem(t, objID, id, n)
		}
	}, append([]gameservertest.Option{
		q230ItemTemplates(),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{proofMonster(pakoID), deathServitorTemplate()})),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{almorsID: script.KindFolk, pakoID: script.KindHostile},
			[]script.Listing{{Path: "quest." + q001}, {Path: q230Path}},
			script.Catalog{"quest." + q001: quest001.New, q230Path: q230.New},
		),
	}, opts...)...)
}

// almorsDuel is a world whose duel with Pako is about to start: Almors'
// duel ready, the progress crystal held, the servitor out and Pako beside
// the player at full HP.
type almorsDuel struct {
	*dialogWorld
	pako     *npc.Hostile
	servitor *summon.Actor
}

func newAlmorsDuel(t *testing.T) (*almorsDuel, func(dmg int)) {
	t.Helper()
	w := q230World(t, 2, map[int32]int32{crystalProgress: 1})
	servitor := w.summonServitor(t)
	pako := w.spawnHostile(t, "pako", pakoID, "Monster", 40)
	hit := func(dmg int) {
		onPlayerQueue(t, w.srv, w.player, func() { pako.TakeDamage(dmg, servitor) })
	}
	return &almorsDuel{dialogWorld: w, pako: pako, servitor: servitor}, hit
}

// pakoSays returns the lines Pako says among the frames sent since the
// last read.
func (d *almorsDuel) pakoSays(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, f := range framesOf(d.srv.ReadQueued(t, d.srv.Client), serverpackets.OpcodeNpcSay) {
		obj, _, _, line := f.r.ReadInt32(), f.r.ReadInt32(), f.r.ReadInt32(), f.r.ReadString()
		if obj != d.pako.ObjectID() {
			t.Fatalf("NpcSay from object %d, want Pako %d", obj, d.pako.ObjectID())
		}
		out = append(out, line)
	}
	return out
}

// wantDuel checks Almors' duel state and the crystals held.
func (d *almorsDuel) wantDuel(t *testing.T, step string, state string, crystals map[int32]int) {
	t.Helper()
	if got := d.srv.QuestVars(t, d.player, q230Name)["Almors"]; got != state {
		t.Fatalf("%s: Almors = %q, want %q", step, got, state)
	}
	for id, want := range crystals {
		if got := d.srv.PlayerItemCount(t, d.player, id); got != want {
			t.Fatalf("%s: item %d count %d, want %d", step, id, got, want)
		}
	}
}

// startDuel lands the servitor's first hit on Pako at full HP: the duel
// starts.
func (d *almorsDuel) startDuel(t *testing.T, hit func(int)) {
	t.Helper()
	hit(10)
	if got := d.pakoSays(t); len(got) != 1 || got[0] != pakoStartsDuel {
		t.Fatalf("Pako at the first servitor hit says %q, want %q", got, pakoStartsDuel)
	}
	d.wantDuel(t, "duel started", "3", map[int32]int{crystalProgress: 0, crystalInDuel: 1})
}

// TestQ230AlmorsOffersAndBuffsTheDuel: with the beginner's arcana held,
// Almors' duel link answers its arcana page; the fight link readies the
// duel, swaps the arcana for the progress crystal, and Almors casts the
// duel buff on the player at its next AI tick.
func TestQ230AlmorsOffersAndBuffsTheDuel(t *testing.T) {
	t.Parallel()
	w := q230World(t, 1, map[int32]int32{beginnerArcana: 1}, gameservertest.WithAITask())
	defs, err := xmldata.LoadSkillDefinitions(datapack.Path(t, "data", "xml", "skills"), zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	almors := w.srv.SpawnCastingFolkNPCAt(t, gameservertest.FolkTemplate("Folk", almorsID),
		location.Location{X: w.at.X, Y: w.at.Y + 60, Z: w.at.Z}, defs)
	for range 3 {
		if err := w.srv.AI.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	w.srv.ReadQueued(t, w.srv.Client)

	if got := w.questWindow(t, almors.ObjectID()); !shows(got, "30635-01.htm") {
		t.Fatalf("Almors' window = %q, want 30635-01.htm", got)
	}
	if got := w.bypass(t, "Quest "+q230Name+" 30635-02.htm"); !shows(got, "30635-03.htm") {
		t.Fatalf("Almors' duel link = %q, want 30635-03.htm", got)
	}
	if got := w.bypass(t, "Quest "+q230Name+" 30635-04.htm"); !shows(got, "30635-04.htm") {
		t.Fatalf("Almors' fight link = %q, want 30635-04.htm", got)
	}
	d := &almorsDuel{dialogWorld: w}
	d.wantDuel(t, "duel readied", "2", map[int32]int{beginnerArcana: 0, crystalProgress: 1})

	var cast []msu
	for range 3 {
		if err := w.srv.AI.Tick(); err != nil {
			t.Fatal(err)
		}
		for _, f := range framesOf(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeMagicSkillUse) {
			cast = append(cast, readMSU(f))
		}
	}
	if len(cast) == 0 || cast[0].caster != almors.ObjectID() || cast[0].target != w.player || cast[0].skill != duelBuffSkill || cast[0].level != 1 {
		t.Fatalf("casts after the fight link = %+v, want Almors casting %d level 1 on the player", cast, duelBuffSkill)
	}
}

// TestQ230DuelWon: the servitor's first hit on Pako at full HP starts the
// duel, a later servitor hit is no foul, and the servitor's kill wins it:
// the victory crystal, which Almors swaps for his arcana.
func TestQ230DuelWon(t *testing.T) {
	t.Parallel()
	d, hit := newAlmorsDuel(t)
	d.startDuel(t, hit)

	hit(10)
	if got := d.pakoSays(t); len(got) != 0 {
		t.Fatalf("Pako at a later servitor hit says %q, want nothing", got)
	}
	d.wantDuel(t, "second servitor hit", "3", nil)

	hit(int(d.pako.MaxHP()) + 1)
	d.srv.Advance(t, 3500*time.Millisecond)
	if got := d.pakoSays(t); len(got) != 1 || got[0] != pakoLosesTheDuel {
		t.Fatalf("Pako at its death says %q, want %q", got, pakoLosesTheDuel)
	}
	d.wantDuel(t, "duel won", "6", map[int32]int{crystalInDuel: 0, crystalVictory: 1})

	almors := d.spawnFolk(t, "almors", almorsID, 0, 60, 0)
	if got := d.questWindow(t, almors.ObjectID()); !shows(got, "30635-07.htm") {
		t.Fatalf("Almors' window after the win = %q, want 30635-07.htm", got)
	}
	d.wantDuel(t, "arcana given", "7", map[int32]int{crystalVictory: 0, almorsArcana: 1})
}

// TestQ230DuelLostByDeath: Pako killing the servitor, or the player,
// during the duel loses it: the defeat crystal, through the death hook of
// the player's quests.
func TestQ230DuelLostByDeath(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		kill func(d *almorsDuel)
		// talks: the player, still alive, asks Almors about the duel.
		talks bool
	}{
		{"servitor", func(d *almorsDuel) {
			onPlayerQueue(t, d.srv, d.player, func() { d.servitor.TakeDamage(1_000_000, d.pako) })
		}, true},
		{"player", func(d *almorsDuel) {
			c := onlineCharacter(t, d.srv, d.player)
			onPlayerQueue(t, d.srv, d.player, func() { c.TakeDamage(1_000_000, d.pako) })
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, hit := newAlmorsDuel(t)
			d.startDuel(t, hit)
			tc.kill(d)
			d.wantDuel(t, "duel lost", "4", map[int32]int{crystalInDuel: 1, crystalDefeat: 1})
			if !tc.talks {
				return
			}
			almors := d.spawnFolk(t, "almors", almorsID, 0, 60, 0)
			if got := d.questWindow(t, almors.ObjectID()); !shows(got, "30635-05.htm") {
				t.Fatalf("Almors' window after the loss = %q, want 30635-05.htm", got)
			}
		})
	}
}

// TestQ230DuelFouled: the player's own hit on Pako during the duel fouls
// it: the crystals of the duel go for the foul crystal, Pako calls the
// foul and dies at once, and its death wins nothing.
func TestQ230DuelFouled(t *testing.T) {
	t.Parallel()
	d, hit := newAlmorsDuel(t)
	d.startDuel(t, hit)

	self := playerCombatant(t, d.srv, d.player)
	onPlayerQueue(t, d.srv, d.player, func() { d.pako.TakeDamage(10, self) })
	if got := d.pakoSays(t); len(got) != 1 || got[0] != pakoCallsFoul {
		t.Fatalf("Pako at the player's hit says %q, want %q", got, pakoCallsFoul)
	}
	if !d.pako.Dead() {
		t.Fatal("Pako lives on after the foul")
	}
	d.wantDuel(t, "duel fouled", "5", map[int32]int{crystalProgress: 0, crystalInDuel: 0, crystalFoul: 1})

	d.srv.Advance(t, 3500*time.Millisecond)
	if got := d.pakoSays(t); len(got) != 0 {
		t.Fatalf("Pako's death after the foul says %q, want nothing", got)
	}
	d.wantDuel(t, "after Pako's death", "5", map[int32]int{crystalVictory: 0})
	almors := d.spawnFolk(t, "almors", almorsID, 0, 60, 0)
	if got := d.questWindow(t, almors.ObjectID()); !shows(got, "30635-06.htm") {
		t.Fatalf("Almors' window after the foul = %q, want 30635-06.htm", got)
	}
}
