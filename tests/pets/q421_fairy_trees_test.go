package pets

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q421"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The fairy trees of Little Wing's Big Adventure
// (Q421_LittleWingsBigAdventure.onAttacked, onCreated, onMyDying): a tree
// only counts the blows of the pet whose collar the quest remembers, and a
// scenario has no pet to land them, so these Boot tests pin the count, the
// leaf, the tree's lines, and the ghosts a dying tree sends.
const (
	q421Name = "Q421_LittleWingsBigAdventure"
	q421Path = "quest.Q421_LittleWingsBigAdventure"

	q421Wind     = 27185
	q421Star     = 27186
	q421Abyss    = 27188
	q421Ghost    = 27189
	q421FairyLef = 4325
)

// q421World is the owner in the world with the quest at cond 2, its wolf out
// and the fairy trees' script bound.
type q421World struct {
	*petWorld
	wolf  *summon.Actor
	draws *scriptDraws
}

// q421Seed is what the quest remembers when the world opens.
type q421Seed struct {
	iCond, attack int
	leaves        int32
	// otherCollar registers another collar than the wolf's.
	otherCollar bool
}

func bootQ421(t *testing.T, seed q421Seed, draws ...int) *q421World {
	t.Helper()
	d := &scriptDraws{next: draws}
	templates := []*npc.Template{wolfTemplate(), treeTemplate()}
	kinds := map[int32]script.NPCKind{}
	for _, id := range []int{q421Wind, q421Star, 27187, q421Abyss, q421Ghost} {
		templates = append(templates, &npc.Template{
			ID: id, TemplateID: id, Type: "Monster", Level: 1, HPMax: 1000,
			AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		})
		kinds[int32(id)] = script.KindHostile
	}
	items := append(gameservertest.ItemTemplates().All(), &item.Template{
		ID: q421FairyLef, Name: "Fairy Leaf", Kind: item.KindEtcItem, Duration: -1, Stackable: true, Destroyable: true,
		EtcItem: &item.EtcItemDetail{},
	})
	srv := bootPets(t,
		gameservertest.WithItemTemplates(item.NewTable(items)),
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: q421Path}}, script.Catalog{q421Path: q421.New}),
		gameservertest.WithScriptRand(d.rand),
	)
	ownerID := srv.SoleObjectID(t)
	collarID := srv.GiveItem(t, ownerID, wolfCollarID, 1)
	if seed.leaves > 0 {
		srv.GiveItem(t, ownerID, q421FairyLef, seed.leaves)
	}
	registered := collarID
	if seed.otherCollar {
		registered++
	}
	for _, r := range [][2]string{
		{"<state>", "STARTED"},
		{"<cond>", "2"},
		{"iCond", strconv.Itoa(seed.iCond)},
		{"summonOid", strconv.Itoa(int(registered))},
		{"attack", strconv.Itoa(seed.attack)},
	} {
		if _, err := srv.DB.ExecContext(context.Background(),
			"INSERT INTO character_quests (charId,name,var,value) VALUES (?,?,?,?)", ownerID, q421Name, r[0], r[1]); err != nil {
			t.Fatal(err)
		}
	}
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: collarID, seeded: map[int32][]int32{}}
	startInWorld(t, h.client)
	wolf, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)
	return &q421World{petWorld: h, wolf: wolf, draws: d}
}

// tree stands a tree of npc id beside the owner.
func (w *q421World) tree(t *testing.T, id int32) *npc.Hostile {
	t.Helper()
	x, y, z := w.srv.PlayerPosition(t, w.ownerID)
	spawned, err := w.srv.NpcSpawns.AddSpawn(id, x+100, y, z, 0, false, 0)
	if err != nil {
		t.Fatalf("spawn the tree: %v", err)
	}
	w.srv.Settle(t)
	drainUntilQuiet(t, w.client)
	return spawned.(*npc.Hostile)
}

// petHits has the wolf land a blow of 1 on tree and returns the NPC lines
// the owner was sent.
func (w *q421World) petHits(t *testing.T, tree *npc.Hostile) []q421Line {
	t.Helper()
	runOnPetQueue(t, w.wolf, func() { tree.TakeDamage(1, w.wolf) })
	w.srv.Settle(t)
	return q421Lines(w.srv.ReadQueued(t, w.client))
}

// q421Line is a line an NPC said: who and what.
type q421Line struct {
	npcID int32
	text  string
}

func q421Lines(frames [][]byte) []q421Line {
	var out []q421Line
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeNpcSay {
			continue
		}
		r := wire.NewReader(f[1:])
		r.ReadInt32() // object id
		r.ReadInt32() // chat type
		npcID := r.ReadInt32() - 1_000_000
		out = append(out, q421Line{npcID, r.ReadString()})
	}
	return out
}

func (w *q421World) vars(t *testing.T) map[string]string {
	t.Helper()
	return w.srv.QuestVars(t, w.ownerID, q421Name)
}

func (w *q421World) ghosts() []*script.NPC {
	var out []*script.NPC
	for _, obj := range w.srv.State.Objects() {
		if c, ok := obj.(attackable.Combatant); ok {
			if n := script.NPCOf(c); n != nil && n.NpcID() == q421Ghost {
				out = append(out, n)
			}
		}
	}
	return out
}

// TestQ421TreesCountOnlyTheRegisteredPetsBlows: the blows of the pet of the
// remembered collar count up to the tree's number; past it the draw against
// the tree's chance takes a leaf, sets the tree's bit and says so, and the
// last tree moves the cond to 3; a tree that gave its leaf already says one
// of its lines instead, and another pet's blows count for nothing.
func TestQ421TreesCountOnlyTheRegisteredPetsBlows(t *testing.T) {
	t.Parallel()
	say := func(id int32) []q421Line { return []q421Line{{q421Wind, script.NpcString(id)}} }
	for _, tc := range []struct {
		name      string
		tree      int32
		seed      q421Seed
		draws     []int
		wantICond string
		wantHit   string
		wantCond  string
		wantLeaf  int
		wantSay   []q421Line
	}{
		{"counting", q421Wind, q421Seed{3, 10, 4, false}, nil, "3", "11", "2", 4, nil},
		{"the blow past the number, a draw under the chance", q421Wind, q421Seed{3, 270, 4, false}, []int{2}, "7", "0", "2", 3, say(42111)},
		{"a draw at the chance", q421Wind, q421Seed{3, 270, 4, false}, []int{3}, "3", "270", "2", 4, nil},
		{"no leaf to give", q421Wind, q421Seed{3, 270, 0, false}, []int{0}, "3", "270", "2", 0, nil},
		{"the last tree", q421Star, q421Seed{55, 400, 1, false}, []int{1}, "63", "0", "3", 0, []q421Line{{q421Star, script.NpcString(42111)}}},
		{"the leaf is given already", q421Wind, q421Seed{7, 5, 3, false}, []int{2}, "7", "5", "2", 3, say(42113)},
		{"another pet", q421Wind, q421Seed{3, 10, 4, true}, nil, "3", "10", "2", 4, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := bootQ421(t, tc.seed, tc.draws...)
			tree := w.tree(t, tc.tree)
			if !tree.AttackDisabled() {
				t.Fatal("the tree attacks on its own: its core AI is not disabled")
			}
			got := w.petHits(t, tree)
			if len(got) != len(tc.wantSay) || (len(got) == 1 && got[0] != tc.wantSay[0]) {
				t.Fatalf("lines = %v, want %v", got, tc.wantSay)
			}
			vars := w.vars(t)
			if vars["iCond"] != tc.wantICond || vars["attack"] != tc.wantHit || vars["<cond>"] != tc.wantCond {
				t.Fatalf("iCond/attack/cond = %q/%q/%q, want %q/%q/%q", vars["iCond"], vars["attack"], vars["<cond>"], tc.wantICond, tc.wantHit, tc.wantCond)
			}
			if n := w.srv.PlayerItemCount(t, w.ownerID, q421FairyLef); n != tc.wantLeaf {
				t.Fatalf("fairy leaves = %d, want %d", n, tc.wantLeaf)
			}
		})
	}
}

// TestQ421OwnersBlowsGiveNoLeaf: the owner's own blow on a tree is no pet's:
// it only meets the 30 in 100 curse draw and counts for nothing.
func TestQ421OwnersBlowsGiveNoLeaf(t *testing.T) {
	t.Parallel()
	w := bootQ421(t, q421Seed{3, 270, 4, false}, 99)
	tree := w.tree(t, q421Wind)
	owner, ok := w.srv.State.Object(w.ownerID)
	if !ok {
		t.Fatal("the owner is not in the world")
	}
	w.draws.mu.Lock()
	w.draws.bounds = nil
	w.draws.mu.Unlock()
	runOn(t, w.srv.PlayerQueue(t, w.ownerID), func() { tree.TakeDamage(1, owner.(attackable.Combatant)) })
	w.srv.Settle(t)
	w.draws.mu.Lock()
	bounds := w.draws.bounds
	w.draws.mu.Unlock()
	if len(bounds) != 1 || bounds[0] != 100 {
		t.Fatalf("script draws = %v, want one of 100", bounds)
	}
	if vars := w.vars(t); vars["attack"] != "270" || vars["iCond"] != "3" {
		t.Fatalf("attack/iCond = %q/%q, want 270/3", vars["attack"], vars["iCond"])
	}
}

// TestQ421DyingTreeSendsTwentyGhosts: a dying tree calls twenty ghosts of
// the tree guardian for 300 s, each saying its line on appearing.
func TestQ421DyingTreeSendsTwentyGhosts(t *testing.T) {
	t.Parallel()
	w := bootQ421(t, q421Seed{3, 0, 4, false})
	tree := w.tree(t, q421Abyss)
	runOnPetQueue(t, w.wolf, func() { tree.TakeDamage(tree.MaxHP()+1, w.wolf) })
	w.srv.Advance(t, 3500*time.Millisecond)
	if n := len(w.ghosts()); n != 20 {
		t.Fatalf("%d ghosts, want 20", n)
	}
	lines := q421Lines(w.srv.ReadQueued(t, w.client))
	said := 0
	for _, l := range lines {
		if l.npcID == q421Ghost && l.text == script.NpcString(42118) {
			said++
		}
	}
	if said != 20 {
		t.Fatalf("%d ghosts said their line, want 20 (lines %v)", said, lines)
	}
	w.srv.Advance(t, 299*time.Second)
	if n := len(w.ghosts()); n != 20 {
		t.Fatalf("%d ghosts 299 s after the tree fell, want 20", n)
	}
	w.srv.Advance(t, 2*time.Second)
	if n := len(w.ghosts()); n != 0 {
		t.Fatalf("%d ghosts after the 300 s, want 0", n)
	}
}
