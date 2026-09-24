package skills

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The tests below each boot a server with one gameplay setting off its
// shipped default and run in parallel with every default-valued Boot in
// this package, so a setting that leaked into process state, or stopped
// reaching its consumer, fails here.

// TestBootMagicFailuresOffSkipsResistRoll casts MDAM with every
// magic-success roll forced to fail: with MagicFailures off the cast rolls
// no resist at all, deals damage, and sends no ATTACK_FAILED.
func TestBootMagicFailuresOffSkipsResistRoll(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Mage", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithMagicFailures(false),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{
				ID: 45, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
				CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
				SkillType: "MDAM", Power: 1_000_000,
			},
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 45, 1)
	startInWorld(t, c)

	worldObj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world player %d missing", objID)
	}
	caster, ok := worldObj.(interface{ SetRollSource(func(int) int) })
	if !ok {
		t.Fatalf("world player %d = %T, want SetRollSource", objID, worldObj)
	}
	magicRolls := 0
	caster.SetRollSource(func(n int) int {
		if n == 10000 {
			magicRolls++
			return 0
		}
		if n <= 0 {
			return 0
		}
		return n - 1
	})

	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)
	maxHP := targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(45, false, false))
	readCastStartFrames(t, c, objID, 45, 1, 500, 60_000, hostile.ObjectID())
	srv.AdvanceUntil(t, "MDAM damage", func() bool { return hostile.CurrentHP() < maxHP })
	drainAssertingNoAttackFailed(t, c)
	if magicRolls != 0 {
		t.Fatalf("magic-success rolls = %d, want 0 with MagicFailures off", magicRolls)
	}
}

type fixedNight bool

func (n fixedNight) IsNight() bool { return bool(n) }

// TestBootNightSourceReachesPlayerAndNpcLists checks the server's night
// source reaches the effect list of a logged-in player and of a fixture
// hostile, the lists <game night=.../> conditions and melee hit chance read.
func TestBootNightSourceReachesPlayerAndNpcLists(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNightSource(fixedNight(true)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)

	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world player %d missing", objID)
	}
	player, ok := obj.(interface{ EffectList() *effect.List })
	if !ok {
		t.Fatalf("world player %d = %T has no EffectList", objID, obj)
	}
	if !player.EffectList().IsNight() {
		t.Fatal("player EffectList().IsNight() = false, want the server's night source")
	}
	if hostile := srv.SpawnHostileNPC(t); !hostile.EffectList().IsNight() {
		t.Fatal("hostile EffectList().IsNight() = false, want the server's night source")
	}
}

// TestBootMaxGeoPathFailCountReachesFixtureHostiles checks a fixture
// hostile wraps its pathfinding-fail streak at the server's threshold: MAX,
// MAX+1, then zero, instead of the shipped 50.
func TestBootMaxGeoPathFailCountReachesFixtureHostiles(t *testing.T) {
	t.Parallel()
	const max = 3
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithMaxGeoPathFailCount(max),
	)
	hostile := srv.SpawnHostileNPC(t)
	for want := 1; want <= max+1; want++ {
		hostile.AddGeoPathFailCount()
		if got := hostile.GeoPathFailCount(); got != want {
			t.Fatalf("GeoPathFailCount() = %d, want %d", got, want)
		}
	}
	hostile.AddGeoPathFailCount()
	if got := hostile.GeoPathFailCount(); got != 0 {
		t.Fatalf("GeoPathFailCount() past MAX+1 = %d, want 0", got)
	}
}
