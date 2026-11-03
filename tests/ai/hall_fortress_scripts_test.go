package ai

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/siegablehall/fortressofresistance"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/siegablehall/fortressofthedead"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The Fortress of Resistance's and the Fortress of the Dead's hall
// scripts. These scenarios bind each alone to its hall's NPCs, so each NPC
// event runs the hall script's hooks and none of the NPCs' behaviors.
const (
	resistanceScript = "script.siegablehall.FortressOfResistance"
	deadScript       = "script.siegablehall.FortressOfTheDead"
)

// lidiaSelfCast is the skill the Fortress of the Dead's script has Lidia
// cast on herself every half minute.
const lidiaSelfCast = modelskill.ID(4997)

// bootResistanceScript boots the character, of class, at at beside Nurka and a partisan soldier of his clan, standing still, with the
// Fortress of Resistance's hall script bound to them, every script draw
// draw, with extra.
func bootResistanceScript(t *testing.T, class, draw int, at location.Location, extra raidBoot) *warriorWorld {
	t.Helper()
	kinds := map[int32]script.NPCKind{}
	var templates []*npc.Template
	for _, id := range []int32{nurkaID, partisanSoldierID} {
		tmpl := agitTemplate(id)
		tmpl.CanMove = false
		templates = append(templates, tmpl)
		kinds[id] = script.KindHostile
	}
	extra.opts = append(extra.opts, gameservertest.WithNPCScripts(kinds,
		[]script.Listing{{Path: resistanceScript}}, script.Catalog{resistanceScript: fortressofresistance.New}))
	return bootHalls(t, class, draw, at, extra, templates...)
}

// TestResistanceScriptSendsPlayersAwayAsNurkaAppears ports
// FortressOfResistance's created hook: Nurka appearing 100 from the
// character, 3000 from the landing spot, sends every player in his
// territory to 51952, 111060, -1970, up to 200 away on each axis.
func TestResistanceScriptSendsPlayersAwayAsNurkaAppears(t *testing.T) {
	t.Parallel()
	w := bootResistanceScript(t, 0, 50, location.Location{X: nurkaLanding.X + 3000, Y: nurkaLanding.Y, Z: nurkaLanding.Z}, raidBoot{})
	w.spawn(t, nurkaID, 100, 0)
	x, y, _ := w.srv.PlayerPosition(t, w.player.ObjectID())
	if abs(x-nurkaLanding.X) > 200 || abs(y-nurkaLanding.Y) > 200 {
		t.Fatalf("the character stands at %d,%d, want within 200 of %v", x, y, nurkaLanding)
	}
}

// TestResistanceScriptNurkaAnswersHisClan ports FortressOfResistance's
// clan-attacked hook: the character's hit on a partisan soldier of Nurka's
// clan makes Nurka want to cast his magic strike (4042) at the character,
// weight 1000000, on a draw under 2, and nothing on a draw of 2. The
// soldier, called by its own clan, wants nothing.
func TestResistanceScriptNurkaAnswersHisClan(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		draw int
		want bool
	}{{"draw 1", 1, true}, {"draw 2", 2, false}} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := bootResistanceScript(t, 0, c.draw, nurkaLanding, raidBoot{})
			soldier := w.spawn(t, partisanSoldierID, 100, 0)
			nurka := w.spawn(t, nurkaID, 100, 150)
			w.hit(t, soldier, siegeTwentieth)
			if _, ok := castDesireOf(nurka, hallNurkaStrike); ok != c.want {
				t.Fatalf("Nurka has a cast desire of 4042 = %v, want %v", ok, c.want)
			}
			if c.want {
				wantCast(t, "Nurka", nurka, hallNurkaStrike, w.player, 1000000, true)
			}
			if _, ok := castDesireOf(soldier, hallNurkaStrike); ok {
				t.Fatal("the soldier wants to cast Nurka's strike")
			}
		})
	}
}

// TestResistanceScriptNurkaStrikesASpellCaster ports FortressOfResistance's
// see-spell hook: a spell Nurka sees the character cast, on the soldier
// beside him, makes him want to cast his magic strike at the character,
// weight 1000000, on a draw under 15, and nothing on a draw of 15.
func TestResistanceScriptNurkaStrikesASpellCaster(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		draw int
		want bool
	}{{"draw 14", 14, true}, {"draw 15", 15, false}} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w := bootResistanceScript(t, classBishop, c.draw, nurkaLanding, siegeSkillBoot(t))
			soldier := w.spawn(t, partisanSoldierID, 100, 0)
			nurka := w.spawn(t, nurkaID, 100, 150)
			// Nurka's appearance sent the character to his landing spot: the
			// client appears there, as a real one does.
			w.srv.Client.Send(wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
			w.srv.Settle(t)
			w.srv.ReadQueued(t, w.srv.Client)
			w.selectTarget(t, soldier)
			w.cast(t, siegeBlessID)
			if _, ok := castDesireOf(nurka, hallNurkaStrike); ok != c.want {
				t.Fatalf("Nurka has a cast desire of 4042 = %v, want %v", ok, c.want)
			}
			if c.want {
				wantCast(t, "Nurka", nurka, hallNurkaStrike, w.player, 1000000, true)
			}
		})
	}
}

// bootDeadScript boots the character at fortressSpot beside the Fortress
// of the Dead's templates, standing still, with the hall script bound to
// them, every script draw draw, and raises Lidia at lidiaSpot. It returns
// the world, Lidia, Giselle, Alfred and the frames Lidia's appearance sent.
func bootDeadScript(t *testing.T, draw int) (w *warriorWorld, lidia, giselle, alfred *npc.Hostile, frames [][]byte) {
	t.Helper()
	kinds := map[int32]script.NPCKind{}
	var templates []*npc.Template
	for _, id := range []int32{lidiaID, alfredID, giselleID, vampireID} {
		tmpl := agitTemplate(id)
		tmpl.CanMove = false
		templates = append(templates, tmpl)
		kinds[id] = script.KindHostile
	}
	skills := append(hallNPCSkills().All(), modelskill.Definition{
		ID: lidiaSelfCast, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "BUFF", HitTime: 1000, StaticHitTime: true, StaticReuse: true,
	})
	w = bootHalls(t, 0, draw, fortressSpot, raidBoot{opts: []gameservertest.Option{
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: deadScript}}, script.Catalog{deadScript: fortressofthedead.New}),
		gameservertest.WithNpcSpawnSkills(modelskill.NewTable(skills)),
	}}, templates...)
	lidia, frames = w.spawnSeen(t, lidiaID, lidiaSpot)
	return w, lidia, privateOf(t, lidia, giselleID), privateOf(t, lidia, alfredID), frames
}

// TestDeadScriptRaisesLidiasFamily ports FortressOfTheDead's created hook:
// Lidia shouts NpcString 1010624 and raises Giselle, Alfred and 15 vampire
// leaders, Giselle facing 54000, Alfred 48000 and the leaders 33540, Giselle
// and Alfred where the reference places them; as they appear Alfred shouts
// 1010636 and Giselle 1010637.
func TestDeadScriptRaisesLidiasFamily(t *testing.T) {
	t.Parallel()
	_, lidia, giselle, alfred, frames := bootDeadScript(t, 0)
	for _, c := range []struct {
		name string
		h    *npc.Hostile
		line int32
	}{{"Lidia", lidia, 1010624}, {"Alfred", alfred, 1010636}, {"Giselle", giselle, 1010637}} {
		if got := says(frames, c.h.ObjectID()); !slices.Equal(got, []string{script.NpcString(c.line)}) {
			t.Errorf("%s lines = %q, want NpcString %d", c.name, got, c.line)
		}
	}
	counts := map[int]int{}
	headings := map[int]int{giselle.NpcID(): 54000, alfred.NpcID(): 48000, int(vampireID): 33540}
	for _, m := range lidia.Minions() {
		h := m.(*npc.Hostile)
		counts[h.NpcID()]++
		if got := script.NPCOf(h).SpawnLocation().Heading; got != headings[h.NpcID()] {
			t.Errorf("private %d faces %d, want %d", h.NpcID(), got, headings[h.NpcID()])
		}
	}
	if want := map[int]int{int(giselleID): 1, int(alfredID): 1, int(vampireID): 15}; !maps.Equal(counts, want) {
		t.Fatalf("Lidia's privates = %v, want %v", counts, want)
	}
	for name, c := range map[string]struct {
		h    *npc.Hostile
		x, y int
	}{"Giselle": {giselle, 56619, -27866}, "Alfred": {alfred, 59282, -26496}} {
		if at := positionOf(c.h); at.X != c.x || at.Y != c.y {
			t.Errorf("%s stands at %v, want %d,%d", name, at, c.x, c.y)
		}
	}
}

// TestDeadScriptLidiaCastsUntilHerKinFall ports FortressOfTheDead's 1001
// timer and party-died hook. As she appears, and every 30 s after, Lidia
// asks to cast 4997 on herself, weight 1000000 (the unspent requests add
// up). Each of her kin's deaths raises her script value; the check that
// finds it above 1 still asks the cast and is the last.
func TestDeadScriptLidiaCastsUntilHerKinFall(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name   string
		kills  func(giselle, alfred *npc.Hostile) []*npc.Hostile
		value  int32
		weight float64 // after 90 s
	}{
		{"Alfred falls", func(_, alfred *npc.Hostile) []*npc.Hostile { return []*npc.Hostile{alfred} }, 1, 4000000},
		{"both fall", func(giselle, alfred *npc.Hostile) []*npc.Hostile { return []*npc.Hostile{alfred, giselle} }, 2, 2000000},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w, lidia, giselle, alfred, _ := bootDeadScript(t, 0)
			w.srv.Advance(t, 0)
			w.srv.Settle(t)
			wantCast(t, "Lidia", lidia, lidiaSelfCast, lidia, 1000000, true)

			for _, h := range c.kills(giselle, alfred) {
				w.hit(t, h, siegeGuardMaxHP)
				if !h.Dead() {
					t.Fatalf("private %d still alive", h.NpcID())
				}
			}
			if got := script.NPCOf(lidia).ScriptValue(); got != c.value {
				t.Fatalf("Lidia's script value = %d, want %d", got, c.value)
			}
			for range 3 {
				w.srv.Advance(t, 30000*time.Millisecond)
				w.srv.Settle(t)
			}
			wantDesireWeight(t, "Lidia", lidia, lidiaSelfCast, c.weight)
		})
	}
}

// TestDeadScriptFamilyShoutsAsItDies ports FortressOfTheDead's dying hook
// outside a siege: Alfred and Giselle each shout NpcString 1010625 and
// Lidia 1010638 as they die.
func TestDeadScriptFamilyShoutsAsItDies(t *testing.T) {
	t.Parallel()
	w, lidia, giselle, alfred, _ := bootDeadScript(t, 0)
	for _, c := range []struct {
		name string
		h    *npc.Hostile
		line int32
	}{{"Alfred", alfred, 1010625}, {"Giselle", giselle, 1010625}, {"Lidia", lidia, 1010638}} {
		w.hit(t, c.h, siegeGuardMaxHP)
		w.srv.Advance(t, 3*time.Second)
		w.srv.Settle(t)
		frames := w.srv.ReadQueued(t, w.srv.Client)
		if got := says(frames, c.h.ObjectID()); !slices.Equal(got, []string{script.NpcString(c.line)}) {
			t.Errorf("%s lines as it dies = %q, want NpcString %d", c.name, got, c.line)
		}
	}
}
