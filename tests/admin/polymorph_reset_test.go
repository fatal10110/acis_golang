package admin

import (
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/maker"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/rs/zerolog"
)

const resetNPCID = 100

// resetTemplates are the NPC //polymorph is tried on, of kind, and the
// form it takes.
func resetTemplates(kind string) (*npc.Template, *npc.Table) {
	own := &npc.Template{
		ID: resetNPCID, TemplateID: resetNPCID, Type: kind, Name: "Own " + kind, Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		UsingServerSideName: true,
	}
	return own, npc.NewTable([]*npc.Template{own, {
		ID: polymorphNPCID, TemplateID: polymorphNPCID + 1, Name: polymorphForm.Name,
		RightHand: int(polymorphForm.RightHand), LeftHand: int(polymorphForm.LeftHand),
		CollisionRadius: polymorphForm.CollisionRadius, CollisionHeight: polymorphForm.CollisionHeight,
	}})
}

// createdCounter is a script whose created hook counts the NPCs it runs
// for.
func createdCounter(created *atomic.Int32, kind script.NPCKind) gameservertest.Option {
	const path = "test.CreatedCounter"
	return gameservertest.WithNPCScripts(map[int32]script.NPCKind{resetNPCID: kind}, []script.Listing{{Path: path}}, script.Catalog{
		path: func() script.Script {
			return script.Script{Behavior: true, NPCs: []int32{resetNPCID}, Hooks: script.Hooks{
				OnCreated: func(*script.Script, script.Created) { created.Add(1) },
			}}
		},
	})
}

// TestAdminPolymorphRerunsCreatedHooks pins the in-place spawn a
// //polymorph re-show is (Creature.polymorph and unpolymorph: decayMe,
// spawnMe; Npc.onSpawn): the NPC's AI drops its desires and its created
// hooks run again, for a monster and for a civilian alike, each time it
// takes or leaves a form.
func TestAdminPolymorphRerunsCreatedHooks(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		kind    string
		script  script.NPCKind
		spawn   func(*testing.T, *gameservertest.Server, *npc.Template, location.Location) morphTarget
		desires func(morphTarget) *ai.DesireQueue
	}{
		{"Monster", script.KindHostile, func(t *testing.T, srv *gameservertest.Server, tmpl *npc.Template, at location.Location) morphTarget {
			return srv.SpawnHostileNPCTemplateAt(t, tmpl, at)
		}, func(n morphTarget) *ai.DesireQueue { return n.(*npc.Hostile).AI().Desires() }},
		{"Folk", script.KindFolk, func(t *testing.T, srv *gameservertest.Server, tmpl *npc.Template, at location.Location) morphTarget {
			return srv.SpawnFolkNPCAt(t, tmpl, at)
		}, func(n morphTarget) *ai.DesireQueue { return n.(*npc.Folk).Desires() }},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			t.Parallel()
			var created atomic.Int32
			own, templates := resetTemplates(tc.kind)
			srv, _ := bootAdmin(t, adminLevel, gameservertest.WithNPCs(templates), createdCounter(&created, tc.script))
			gm := srv.Client
			enterWorld(t, gm)
			target := tc.spawn(t, srv, own, location.Location{X: spawnX + 40, Y: spawnY, Z: spawnZ})
			srv.RunCreated(t, target.(attackable.Combatant))
			if got := created.Load(); got != 1 {
				t.Fatalf("created hooks at spawn = %d, want 1", got)
			}
			// A route desire stays queued for the NPC's life.
			tc.desires(target).AddOrUpdate(&ai.Desire{Kind: ai.IntentionMoveRoute, RouteName: "reset_route", Weight: 1})

			exchange(t, gm, encodeAction(target.ObjectID()))
			exchange(t, gm, encodeBuildCmd("polymorph "+strconv.Itoa(polymorphNPCID)))
			srv.Settle(t)
			if got := created.Load(); got != 2 {
				t.Fatalf("created hooks after the polymorph = %d, want 2", got)
			}
			if n := tc.desires(target).Len(); n != 0 {
				t.Fatalf("desires after the polymorph = %d, want none", n)
			}
			exchange(t, gm, encodeAction(target.ObjectID()))
			exchange(t, gm, encodeBuildCmd("polymorph"))
			srv.Settle(t)
			if got := created.Load(); got != 3 {
				t.Fatalf("created hooks after the unpolymorph = %d, want 3", got)
			}
		})
	}
}

// TestAdminPolymorphClearsMonsterState pins Monster.onSpawn on a
// //polymorph re-show: the monster's over-hit strike, spoil, seed and
// soul crystal charges are gone once it is shown in its form.
func TestAdminPolymorphClearsMonsterState(t *testing.T) {
	t.Parallel()
	own, templates := resetTemplates("Monster")
	srv, gmID := bootAdmin(t, adminLevel, gameservertest.WithNPCs(templates))
	gm := srv.Client
	enterWorld(t, gm)
	monster := srv.SpawnHostileNPCTemplateAt(t, own, location.Location{X: spawnX + 40, Y: spawnY, Z: spawnZ})
	obj, ok := srv.State.Player(gmID)
	if !ok {
		t.Fatal("GM missing from world")
	}
	player, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("world player %T is not an online character", obj)
	}

	done := make(chan struct{})
	monster.Queue().Post(func() {
		defer close(done)
		// An invulnerable monster records a lethal over-hit strike and
		// lives on.
		monster.Live.SetInvul(true)
		monster.EnableOverhit()
		monster.ReduceHP(monster.HP()*2, player, modelskill.Definition{})
	})
	<-done
	monster.SpoilPool().Mark(gmID)
	monster.SeedState().Sow(gmID, manor.Seed{CropID: 5073, Level: 1})
	monster.AddAbsorber(gmID, 1)
	if !monster.OverhitValid(player) || !monster.SpoilPool().IsSpoiled() || !monster.SeedState().Seeded() {
		t.Fatal("setup: the monster holds no over-hit, spoil or seed")
	}
	if _, ok := monster.Absorber(gmID); !ok {
		t.Fatal("setup: the monster holds no soul crystal charge")
	}

	exchange(t, gm, encodeAction(monster.ObjectID()))
	exchange(t, gm, encodeBuildCmd("polymorph "+strconv.Itoa(polymorphNPCID)))
	srv.Settle(t)
	if monster.OverhitValid(player) {
		t.Fatal("the polymorphed monster keeps its over-hit strike")
	}
	if monster.SpoilPool().IsSpoiled() {
		t.Fatal("the polymorphed monster stays spoiled")
	}
	if monster.SeedState().Seeded() {
		t.Fatal("the polymorphed monster stays sown")
	}
	if _, ok := monster.Absorber(gmID); ok {
		t.Fatal("the polymorphed monster keeps its soul crystal charge")
	}
}

// TestAdminPolymorphRerunsMakerHook pins the maker side of a //polymorph
// re-show (Npc.onSpawn, MultiSpawn.onSpawn, NpcMaker.onSpawn): the
// npcmaker that placed the NPC hears it created again, once per re-show,
// and the NPC stays its one live NPC.
func TestAdminPolymorphRerunsMakerHook(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	body := `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="reset_field" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="reset_den" territory="reset_field" maximumNpcs="1">
		<ai type="counting_maker"/>
		<npc id="100" total="1" pos="50;20;30;0" respawn="60sec"/>
	</npcmaker>
</list>`
	if err := os.WriteFile(filepath.Join(dir, "den.xml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write spawnlist: %v", err)
	}
	makers, err := gamexml.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("load spawnlist: %v", err)
	}
	var created atomic.Int32
	counting := func() script.Maker {
		m := maker.Default()
		base := m.OnNPCCreated
		m.OnNPCCreated = func(m *script.Maker, e script.MakerNPC) {
			created.Add(1)
			base(m, e)
		}
		return m
	}
	_, templates := resetTemplates("Monster")
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithNPCs(templates), gameservertest.WithNpcSpawns(makers),
		gameservertest.WithMakers(script.MakerCatalog{"counting_maker": counting}))
	gm := srv.Client
	enterWorld(t, gm)
	srv.Settle(t)
	if got := created.Load(); got != 1 {
		t.Fatalf("maker created hooks at boot = %d, want 1", got)
	}
	den := npcsOf(srv, resetNPCID)
	if len(den) != 1 {
		t.Fatalf("den NPCs = %d, want 1", len(den))
	}
	id := den[0].(*npc.Hostile).ObjectID()

	exchange(t, gm, encodeAction(id))
	exchange(t, gm, encodeBuildCmd("polymorph "+strconv.Itoa(polymorphNPCID)))
	srv.Settle(t)
	if got := created.Load(); got != 2 {
		t.Fatalf("maker created hooks after the polymorph = %d, want 2", got)
	}
	if den := npcsOf(srv, resetNPCID); len(den) != 1 || den[0].(*npc.Hostile).ObjectID() != id {
		t.Fatalf("den NPCs after the polymorph = %v, want the one NPC %d", den, id)
	}
}
