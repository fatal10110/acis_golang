package character

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// regenRates is a player's HP, MP and CP regeneration rates.
type regenRates struct{ hp, mp, cp float64 }

func readRegenRates(c *player.Character) regenRates {
	return regenRates{c.HPRegenRate(), c.MPRegenRate(), c.CPRegenRate()}
}

func (r regenRates) scaled(mul float64) regenRates {
	return regenRates{r.hp * mul, r.mp * mul, r.cp * mul}
}

func (r regenRates) near(o regenRates) bool {
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
	return near(r.hp, o.hp) && near(r.mp, o.mp) && near(r.cp, o.cp)
}

// regenTermsTemplate is the shared human fighter class with flat HP, MP
// and CP regeneration tables, which the shared class template leaves
// empty.
func regenTermsTemplate() gameservertest.Option {
	tmpl := gameservertest.ClassTemplate()
	tmpl.CON, tmpl.MEN = 43, 25
	tmpl.HPRegenTable, tmpl.MPRegenTable, tmpl.CPRegenTable = []float64{2}, []float64{0.9}, []float64{2}
	return gameservertest.WithClassTemplate(tmpl)
}

// bootRegenTerms boots the level 1 human fighter "Newbie" into the world
// at its spawn point, inside zones when one is given, and returns the
// server and its online character.
func bootRegenTerms(t *testing.T, zones *zone.Index) (*gameservertest.Server, *player.Character) {
	t.Helper()
	opts := []gameservertest.Option{
		regenTermsTemplate(),
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
	}
	if zones != nil {
		opts = append(opts, gameservertest.WithZones(zones))
	}
	srv := gameservertest.Boot(t, opts...)
	enterWorld(t, srv.Client)
	drainQuiet(t, srv.Client)
	obj, ok := srv.State.Player(srv.SoleObjectID(t))
	if !ok {
		t.Fatal("player not in the world")
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("%T is not an online character", obj)
	}
	return srv, c
}

// motherTreeAtSpawn is a zone index holding one Mother Tree zone around
// the shared spawn point, granting hp and mp and admitting race (-1: every
// race).
func motherTreeAtSpawn(t *testing.T, hp, mp, race int) *zone.Index {
	t.Helper()
	form, err := zone.NewCuboid(-500, 500, -500, 500, -10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	mt := zone.NewMotherTree(1, form)
	mt.HPBonus, mt.MPBonus, mt.AffectedRace = hp, mp, race
	zones := zone.NewIndex()
	zones.Add(mt)
	return zones
}

// TestRegenPostureMultiplier: a walking player regenerates at the base
// rate, a running one at 0.7 of it, one standing still at 1.1 and a seated
// one at 1.5, for HP, MP and CP alike. A player still sitting down counts
// as standing still.
func TestRegenPostureMultiplier(t *testing.T) {
	t.Parallel()
	srv, c := bootRegenTerms(t, nil)
	client := srv.Client
	objID := srv.SoleObjectID(t)
	still := readRegenRates(c)
	if still.hp <= 0 || still.mp <= 0 || still.cp <= 0 {
		t.Fatalf("standing regen = %+v, want every rate positive", still)
	}

	// The multiplier changes once the sit-down ends.
	client.Send(encodeRequestChangeWaitType(false))
	mustReadOpcode(t, client, serverpackets.OpcodeChangeWaitType, "sit")
	if got := readRegenRates(c); !got.near(still) {
		t.Fatalf("regen while sitting down = %+v, want standing %+v", got, still)
	}
	srv.SettlePosture(t, objID)
	seated := readRegenRates(c)
	client.Send(encodeRequestChangeWaitType(true))
	mustReadOpcode(t, client, serverpackets.OpcodeChangeWaitType, "stand")
	if got := readRegenRates(c); !got.near(still) {
		t.Fatalf("regen while standing up = %+v, want standing %+v", got, still)
	}
	srv.SettlePosture(t, objID)

	far := location.Location{X: 4000, Y: 20, Z: 30}
	client.Send(encodeMoveBackwardToLocation(far, spawnOrigin, 1))
	mustReadOpcode(t, client, serverpackets.OpcodeMoveToLocation, "run")
	if !c.IsMoving() || !c.Running() {
		t.Fatalf("moving = %v, running = %v; want a running player", c.IsMoving(), c.Running())
	}
	running := readRegenRates(c)

	client.Send(encodeRequestChangeMoveType(false))
	mustReadOpcode(t, client, serverpackets.OpcodeChangeMoveType, "walk")
	if !c.IsMoving() || c.Running() {
		t.Fatalf("moving = %v, running = %v; want a walking player", c.IsMoving(), c.Running())
	}
	walking := readRegenRates(c)

	for _, tt := range []struct {
		name string
		got  regenRates
		mul  float64
	}{
		{"standing", still, 1.1},
		{"seated", seated, 1.5},
		{"running", running, 0.7},
	} {
		if want := walking.scaled(tt.mul); !tt.got.near(want) {
			t.Errorf("%s regen = %+v, want %vx walking %+v = %+v", tt.name, tt.got, tt.mul, walking, want)
		}
	}
}

// TestRegenMotherTreeBonus: a player inside a Mother Tree zone that admits
// its race gains the zone's flat HP and MP bonus on top of its multiplied
// rate, seated or standing; CP gains nothing. A zone restricted to another
// race grants nothing.
func TestRegenMotherTreeBonus(t *testing.T) {
	t.Parallel()
	// Each subtest boots its own server; they run in order so the later
	// ones compare against the rates read outside.
	var base regenRates
	t.Run("outside", func(t *testing.T) {
		_, outside := bootRegenTerms(t, nil)
		base = readRegenRates(outside)
	})
	t.Run("inside", func(t *testing.T) {
		srv, inside := bootRegenTerms(t, motherTreeAtSpawn(t, 2, 1, -1))
		want := regenRates{base.hp + 2, base.mp + 1, base.cp}
		if got := readRegenRates(inside); !got.near(want) {
			t.Fatalf("regen in the Mother Tree = %+v, want %+v (outside %+v)", got, want, base)
		}

		// The bonus adds after the posture multiplier.
		srv.Client.Send(encodeRequestChangeWaitType(false))
		mustReadOpcode(t, srv.Client, serverpackets.OpcodeChangeWaitType, "sit")
		srv.SettlePosture(t, srv.SoleObjectID(t))
		seated := base.scaled(1.5 / 1.1)
		want = regenRates{seated.hp + 2, seated.mp + 1, seated.cp}
		if got := readRegenRates(inside); !got.near(want) {
			t.Fatalf("seated regen in the Mother Tree = %+v, want %+v", got, want)
		}
	})
	t.Run("another race", func(t *testing.T) {
		// The fixture character is human; an elf-only zone leaves it alone.
		_, refused := bootRegenTerms(t, motherTreeAtSpawn(t, 2, 1, int(player.RaceElf)))
		if got := readRegenRates(refused); !got.near(base) {
			t.Fatalf("regen in an elf-only Mother Tree = %+v, want the plain %+v", got, base)
		}
	})
}

// TestRegenTickAppliesMotherTreeBonus: a regeneration tick inside a Mother
// Tree zone restores the bonus-raised rate.
func TestRegenTickAppliesMotherTreeBonus(t *testing.T) {
	t.Parallel()
	srv, c := bootRegenTerms(t, motherTreeAtSpawn(t, 30, 20, -1))
	if !srv.DrivesClock() {
		t.Skip("pinning the regeneration tick needs the driven clock")
	}
	rates := readRegenRates(c)
	if rates.hp < 30 || rates.mp < 20 {
		t.Fatalf("regen in the Mother Tree = %+v, want the bonus included", rates)
	}
	c.SetResourceValues(player.Resources{MaxHP: 100, CurrentHP: 100, MaxMP: 100, CurrentMP: 100, MaxCP: 100, CurrentCP: 100})
	c.SetResourceValues(player.Resources{MaxHP: 100, CurrentHP: 10, MaxMP: 100, CurrentMP: 10, MaxCP: 100, CurrentCP: 10})
	srv.ReadQueued(t, srv.Client)

	regen := task.NewNPCRegen(srv.State)
	srv.Advance(t, task.NPCRegenTick)
	regen.Tick()
	srv.Settle(t)
	got := c.ResourceValues()
	want := player.Resources{
		MaxHP: 100, CurrentHP: 10 + math.Max(1, rates.hp),
		MaxMP: 100, CurrentMP: 10 + math.Max(1, rates.mp),
		MaxCP: 100, CurrentCP: 10 + math.Max(1, rates.cp),
	}
	if math.Abs(got.CurrentHP-want.CurrentHP) > 1e-9 || math.Abs(got.CurrentMP-want.CurrentMP) > 1e-9 || math.Abs(got.CurrentCP-want.CurrentCP) > 1e-9 {
		t.Fatalf("resources after one tick = %+v, want %+v", got, want)
	}
}
