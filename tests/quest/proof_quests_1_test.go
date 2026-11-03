package quest

import (
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q211"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q316"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// What the scenarios of the first proof-quest group cannot show: the NPCs
// Trial of the Challenger (Q211) spawns as its monsters die, and the cry
// of Varool Foulclaw in Destroy Plague Carriers (Q316).

const (
	q211Name      = "Q211_TrialOfTheChallenger"
	q211Path      = "quest.Q211_TrialOfTheChallenger"
	shyslassysID  = 27110
	barahamID     = 27113
	raldoID       = 30646
	chestID       = 30647
	q316Path      = "quest.Q316_DestroyPlagueCarriers"
	varoolID      = 27020
	varoolCryText = "Why do you oppress us so?"
)

// proofMonster is a level 1 monster template of 1000 HP with npc id id.
func proofMonster(id int) *npc.Template {
	return &npc.Template{
		ID: id, TemplateID: id, Type: "Monster", Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}
}

// q211ItemTemplates is the shared item catalog plus the broken key and the
// scroll Shyslassys' death gives.
func q211ItemTemplates() gameservertest.Option {
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{2631, 2632} {
		templates = append(templates, &item.Template{
			ID: id, Name: "Quest Item " + strconv.Itoa(int(id)), Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	return gameservertest.WithItemTemplates(item.NewTable(templates))
}

// killOnPlayerQueue lands a lethal hit of the player objID on h, then lets
// the dying hooks' three seconds pass.
func killOnPlayerQueue(t *testing.T, srv *gameservertest.Server, objID int32, h *npc.Hostile) {
	t.Helper()
	p := playerCombatant(t, srv, objID)
	done := make(chan struct{})
	if !srv.PlayerQueue(t, objID).Post(func() {
		defer close(done)
		h.TakeDamage(h.MaxHP()+1, p)
	}) {
		t.Fatal("the player's queue is closed")
	}
	<-done
	srv.Advance(t, 3500*time.Millisecond)
}

// npcsOf returns the NPCs of npc id id in the world.
func npcsOf(srv *gameservertest.Server, id int32) []*script.NPC {
	var out []*script.NPC
	for _, obj := range srv.State.Objects() {
		c, ok := obj.(attackable.Combatant)
		if !ok {
			continue
		}
		if n := script.NPCOf(c); n != nil && n.NpcID() == id {
			out = append(out, n)
		}
	}
	return out
}

// TestQ211DyingMonstersLeaveTheirNPCs: Shyslassys' death, with the trial
// at cond 1, leaves his chest where he fell for 200 s; Baraham's death
// leaves Raldo where he fell for 100 s whatever the trial's cond
// (Q211_TrialOfTheChallenger.onMyDying, Quest.addSpawn).
func TestQ211DyingMonstersLeaveTheirNPCs(t *testing.T) {
	t.Parallel()
	w := bootSeamQuest(t, 36, nil, func(srv *gameservertest.Server, objID int32) {
		seedJournal(t, srv, objID, []string{q211Name + ":STARTED"})
	},
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			proofMonster(shyslassysID), proofMonster(barahamID),
			gameservertest.FolkTemplate("Folk", raldoID), gameservertest.FolkTemplate("Folk", chestID),
		})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{shyslassysID: script.KindHostile, barahamID: script.KindHostile, raldoID: script.KindFolk, chestID: script.KindFolk},
			[]script.Listing{{Path: q211Path}},
			script.Catalog{q211Path: q211.New},
		),
		q211ItemTemplates(),
	)

	for _, tc := range []struct {
		name     string
		monster  int
		dx       int
		left     int32
		despawn  time.Duration
		wantCond string
	}{
		{"shyslassys", shyslassysID, 40, chestID, 200 * time.Second, "2"},
		{"baraham", barahamID, -40, raldoID, 100 * time.Second, "2"},
	} {
		h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(tc.monster), location.Location{X: w.at.X + tc.dx, Y: w.at.Y, Z: w.at.Z})
		hx, hy, _ := h.Position()
		killOnPlayerQueue(t, w.srv, w.player, h)
		left := npcsOf(w.srv, tc.left)
		if len(left) != 1 {
			t.Fatalf("%s: %d NPCs %d in the world after the death, want 1", tc.name, len(left), tc.left)
		}
		if at := left[0].SpawnLocation(); at.X != hx || at.Y != hy {
			t.Fatalf("%s: NPC %d stands at (%d, %d), want where the monster fell (%d, %d)", tc.name, tc.left, at.X, at.Y, hx, hy)
		}
		if vars := w.srv.QuestVars(t, w.player, q211Name); vars["<cond>"] != tc.wantCond {
			t.Fatalf("%s: Q211 vars %v, want cond %s", tc.name, vars, tc.wantCond)
		}
		w.srv.Advance(t, tc.despawn-time.Second)
		if n := len(npcsOf(w.srv, tc.left)); n != 1 {
			t.Fatalf("%s: %d NPCs %d a second before the despawn, want 1", tc.name, n, tc.left)
		}
		w.srv.Advance(t, 2*time.Second)
		if n := len(npcsOf(w.srv, tc.left)); n != 0 {
			t.Fatalf("%s: %d NPCs %d after the despawn, want 0", tc.name, n, tc.left)
		}
	}
}

// TestQ316VaroolCriesOutOncePerLife: the first hit on Varool Foulclaw makes
// him say NpcString 31603 to the players around; later hits say nothing
// (Q316_DestroyPlagueCarriers.onAttacked, the script value).
func TestQ316VaroolCriesOutOncePerLife(t *testing.T) {
	t.Parallel()
	w := bootSeamQuest(t, 18, nil, nil,
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{varoolID: script.KindHostile},
			[]script.Listing{{Path: q316Path}},
			script.Catalog{q316Path: q316.New},
		),
	)
	h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(varoolID), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
	w.srv.ReadQueued(t, w.srv.Client)
	p := playerCombatant(t, w.srv, w.player)
	for i := range 3 {
		done := make(chan struct{})
		if !w.srv.PlayerQueue(t, w.player).Post(func() {
			defer close(done)
			h.TakeDamage(10, p)
		}) {
			t.Fatal("the player's queue is closed")
		}
		<-done
		says := framesOf(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeNpcSay)
		want := 0
		if i == 0 {
			want = 1
		}
		if len(says) != want {
			t.Fatalf("hit %d: %d NpcSay frames, want %d", i+1, len(says), want)
		}
		if want == 1 {
			r := says[0].r
			if obj, _, _, line := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadString(); obj != h.ObjectID() || line != varoolCryText {
				t.Fatalf("hit 1: NpcSay obj %d %q, want obj %d %q", obj, line, h.ObjectID(), varoolCryText)
			}
		}
	}
}
