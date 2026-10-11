package quest

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q234"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// What the Fate's Whispers (Q234) scenario cannot show, since it holds no
// item: Baium's blood on the pipette knife, the crystals, the weapon
// choice and the forging; and where a raid boss's chest stands and how
// long (Q234_FatesWhisper.onAttacked, onTalk, onAdvEvent, onMyDying).

const (
	q234Name = "Q234_FatesWhisper"
	q234Path = "quest.Q234_FatesWhisper"

	q234Reorin      = 31002
	q234Kaspar      = 30833
	q234ChestKernon = 31028
	q234Kernon      = 25054
	q234Baium       = 29020

	q234PipetteKnife    = 4665
	q234RedPipetteKnife = 4673
	q234ReorinMold      = 4671
	q234CrystalB        = 1460
	q234StarOfDestiny   = 5011

	// The B-grade weapon chosen and the A-grade one made of it.
	q234SwordOfDamascus = 79
	q234TallumBlade     = 80

	q234BloodCry = "Who dares to try and steal my noble blood?"
)

func bootQ234(t *testing.T, cond string, before func(srv *gameservertest.Server, objID int32), opts ...gameservertest.Option) *dialogWorld {
	t.Helper()
	return bootX1Quest(t, 75, q234Path, q234.New, []int32{q234Reorin, q234Kaspar, q234ChestKernon}, []int32{q234Baium, q234Kernon},
		func(srv *gameservertest.Server, objID int32) {
			seedLevelExp(t, srv, objID, 75)
			insertJournal(t, srv.DB,
				journalRow{objID, q234Name, "<state>", val("STARTED")},
				journalRow{objID, q234Name, "<cond>", val(cond)},
			)
			if before != nil {
				before(srv, objID)
			}
		},
		append([]gameservertest.Option{x1Items([]int32{q234RedPipetteKnife, q234ReorinMold, q234CrystalB, q234StarOfDestiny},
			x1Weapon{q234PipetteKnife, "Pipette Knife"}, x1Weapon{q234SwordOfDamascus, "Sword of Damascus"}, x1Weapon{q234TallumBlade, "Tallum Blade"})},
			opts...)...)
}

// weaponPage is the rendered shipped Reorin page file with the weapon name
// filled in.
func (w *dialogWorld) weaponPage(t *testing.T, file, name string) string {
	t.Helper()
	text := strings.ReplaceAll(datapackPage(t, "script/quest/"+q234Name+"/"+file), "%weaponname%", name)
	return "S NpcHtmlMessage obj=reorin item=0 html=" + scriptcontract.Quote(text)
}

// TestQ234BaiumsBloodReddensTheKnife: at cond 7, a hit on Baium with the
// pipette knife in hand takes his blood: the item sound, the knife traded
// for the red one, and his cry to the players around. A second hit, with
// the red knife held, takes nothing. Kaspar then trades the red knife for
// Reorin's mold (cond 8).
func TestQ234BaiumsBloodReddensTheKnife(t *testing.T) {
	t.Parallel()
	var knife int32
	w := bootQ234(t, "7", func(srv *gameservertest.Server, objID int32) {
		knife = srv.GiveItem(t, objID, q234PipetteKnife, 1)
	})
	kaspar := w.spawnFolk(t, "kaspar", q234Kaspar, 10, 0, 0).ObjectID()
	w.srv.Client.Send(encodeUseItem(knife))
	w.srv.AdvanceUntil(t, "the knife is the active weapon", func() bool {
		var id int32
		w.srv.RunScript(t, w.player, q234Name, func(_ *script.Script, p *script.Player) { id = p.ActiveWeaponItemID() })
		return id == q234PipetteKnife
	})
	baium := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(q234Baium), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
	w.srv.ReadQueued(t, w.srv.Client)
	p := playerCombatant(t, w.srv, w.player)
	hit := func() [][]byte {
		done := make(chan struct{})
		if !w.srv.PlayerQueue(t, w.player).Post(func() {
			defer close(done)
			baium.TakeDamage(10, p)
		}) {
			t.Fatal("the player's queue is closed")
		}
		<-done
		return w.srv.ReadQueued(t, w.srv.Client)
	}

	frames := hit()
	if got, want := scriptLines(t, frames), []string{
		"S PlaySound type=0 file=ItemSound.quest_itemget bind=0 obj=0 loc=0,0,0 delay=0",
		itemMessage(302, q234PipetteKnife),
		itemMessage(54, q234RedPipetteKnife),
	}; !slices.Equal(got, want) {
		t.Fatalf("the first hit = %q, want %q", got, want)
	}
	says := framesOf(frames, serverpackets.OpcodeNpcSay)
	if len(says) != 1 {
		t.Fatalf("the first hit: %d NpcSay frames, want 1", len(says))
	}
	r := says[0].r
	if obj, _, _, line := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadString(); obj != baium.ObjectID() || line != q234BloodCry {
		t.Fatalf("the first hit: NpcSay obj %d %q, want obj %d %q", obj, line, baium.ObjectID(), q234BloodCry)
	}
	for id, n := range map[int32]int{q234PipetteKnife: 0, q234RedPipetteKnife: 1} {
		if c := w.srv.PlayerItemCount(t, w.player, id); c != n {
			t.Errorf("item %d after the blood = %d, want %d", id, c, n)
		}
	}
	if frames := hit(); len(scriptLines(t, frames)) != 0 || len(framesOf(frames, serverpackets.OpcodeNpcSay)) != 0 {
		t.Fatalf("the second hit = %q, want nothing", scriptLines(t, frames))
	}

	got := w.questWindow(t, kaspar)
	if !slices.Contains(got, w.page(t, "kaspar", q234Name, "30833-03.htm")) {
		t.Fatalf("Kaspar's window with the red knife = %q, want 30833-03.htm", got)
	}
	for id, n := range map[int32]int{q234RedPipetteKnife: 0, q234ReorinMold: 1} {
		if c := w.srv.PlayerItemCount(t, w.player, id); c != n {
			t.Errorf("item %d after Kaspar = %d, want %d", id, c, n)
		}
	}
	if vars := w.srv.QuestVars(t, w.player, q234Name); vars["<cond>"] != "8" {
		t.Fatalf("Q234 after Kaspar = %v, want cond 8", vars)
	}
}

// TestQ234ReorinForgesTheChosenWeapon: at cond 8, Reorin takes the mold
// (cond 9) and, once 984 B-grade crystals are held, takes them (cond 10)
// for the B-grade list. The choice names the weapon on his pages; until
// it is held he asks for it, then lists the A-grade weapons. The A-grade
// pick takes the B-grade weapon for the A-grade one and the Star of
// Destiny, and completes the quest.
func TestQ234ReorinForgesTheChosenWeapon(t *testing.T) {
	t.Parallel()
	w := bootQ234(t, "8", func(srv *gameservertest.Server, objID int32) {
		srv.GiveItem(t, objID, q234ReorinMold, 1)
		srv.GiveItem(t, objID, q234CrystalB, 983)
	})
	reorin := w.spawnFolk(t, "reorin", q234Reorin, 10, 0, 0).ObjectID()

	if got := w.questWindow(t, reorin); !slices.Contains(got, w.page(t, "reorin", q234Name, "31002-09.htm")) {
		t.Fatalf("Reorin's window with the mold = %q, want 31002-09.htm", got)
	}
	if got := w.questWindow(t, reorin); !slices.Contains(got, w.page(t, "reorin", q234Name, "31002-09b.htm")) {
		t.Fatalf("Reorin's window with 983 crystals = %q, want 31002-09b.htm", got)
	}
	w.srv.RunScript(t, w.player, q234Name, func(s *script.Script, p *script.Player) { s.GiveItems(p, q234CrystalB, 1) })
	w.srv.ReadQueued(t, w.srv.Client)
	if got := w.questWindow(t, reorin); !slices.Contains(got, w.page(t, "reorin", q234Name, "31002-BGradeList.htm")) {
		t.Fatalf("Reorin's window with 984 crystals = %q, want 31002-BGradeList.htm", got)
	}
	if c := w.srv.PlayerItemCount(t, w.player, q234CrystalB); c != 0 {
		t.Fatalf("crystals after the list = %d, want 0", c)
	}
	if vars := w.srv.QuestVars(t, w.player, q234Name); vars["<cond>"] != "10" {
		t.Fatalf("Q234 after the crystals = %v, want cond 10", vars)
	}

	if got := w.bypass(t, "Quest "+q234Name+" selectBGrade_79"); !slices.Contains(got, w.weaponPage(t, "31002-13.htm", "Sword of Damascus")) {
		t.Fatalf("the B-grade choice = %q, want 31002-13.htm naming the sword", got)
	}
	if got := w.bypass(t, "Quest "+q234Name+" confirmWeapon"); !slices.Contains(got, w.weaponPage(t, "31002-14.htm", "Sword of Damascus")) {
		t.Fatalf("the confirmation = %q, want 31002-14.htm naming the sword", got)
	}
	if vars := w.srv.QuestVars(t, w.player, q234Name); vars["bypass"] != "1" || vars["weaponId"] != "79" {
		t.Fatalf("Q234 after the confirmation = %v, want bypass 1 and weaponId 79", vars)
	}
	if got := w.questWindow(t, reorin); !slices.Contains(got, w.weaponPage(t, "31002-15.htm", "Sword of Damascus")) {
		t.Fatalf("Reorin's window without the sword = %q, want 31002-15.htm naming it", got)
	}

	w.srv.RunScript(t, w.player, q234Name, func(s *script.Script, p *script.Player) { s.GiveItems(p, q234SwordOfDamascus, 1) })
	w.srv.ReadQueued(t, w.srv.Client)
	if got := w.questWindow(t, reorin); !slices.Contains(got, w.weaponPage(t, "31002-AGradeList.htm", "Sword of Damascus")) {
		t.Fatalf("Reorin's window with the sword = %q, want 31002-AGradeList.htm naming it", got)
	}
	got := keep(w.bypassAll(t, "Quest "+q234Name+" selectAGrade_80"), "S SystemMessage", "S SocialAction", "S PlaySound", "S QuestList", "S NpcHtmlMessage")
	want := []string{
		itemMessage(302, q234SwordOfDamascus),
		itemMessage(54, q234TallumBlade),
		itemMessage(54, q234StarOfDestiny),
		"S SocialAction obj=player action=3",
		"S PlaySound type=0 file=ItemSound.quest_finish bind=0 obj=0 loc=0,0,0 delay=0",
		"S QuestList 234:0x00000000",
		w.weaponPage(t, "31002-12.htm", "Tallum Blade"),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("the forging = %q, want %q", got, want)
	}
	for id, n := range map[int32]int{q234SwordOfDamascus: 0, q234TallumBlade: 1, q234StarOfDestiny: 1} {
		if c := w.srv.PlayerItemCount(t, w.player, id); c != n {
			t.Errorf("item %d after the forging = %d, want %d", id, c, n)
		}
	}
	if vars := w.srv.QuestVars(t, w.player, q234Name); len(vars) != 1 || vars["<state>"] != "COMPLETED" {
		t.Fatalf("Q234 after the forging = %v, want completed alone", vars)
	}
}

// TestQ234RaidBossLeavesItsChest: Kernon's death puts the Chest of Kernon
// within 100 of him on each axis, three seconds on, for 120 seconds.
func TestQ234RaidBossLeavesItsChest(t *testing.T) {
	t.Parallel()
	w := bootQ234(t, "2", nil,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{proofMonster(q234Kernon), gameservertest.FolkTemplate("Folk", q234ChestKernon)})),
		gameservertest.WithNpcSpawns(nil))
	at := location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z}
	kernon := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(q234Kernon), at)
	killOnPlayerQueue(t, w.srv, w.player, kernon)
	chests := npcsOf(w.srv, q234ChestKernon)
	if len(chests) != 1 {
		t.Fatalf("%d chests of Kernon after his death, want 1", len(chests))
	}
	x, y := chests[0].SpawnLocation().X, chests[0].SpawnLocation().Y
	if abs(x-at.X) > 100 || abs(y-at.Y) > 100 {
		t.Fatalf("the chest stands at %d,%d, more than 100 from Kernon at %d,%d", x, y, at.X, at.Y)
	}
	w.srv.Advance(t, 119*time.Second)
	if n := len(npcsOf(w.srv, q234ChestKernon)); n != 1 {
		t.Fatalf("%d chests a second before the despawn, want 1", n)
	}
	w.srv.Advance(t, 2*time.Second)
	if n := len(npcsOf(w.srv, q234ChestKernon)); n != 0 {
		t.Fatalf("%d chests after the despawn, want 0", n)
	}
}
