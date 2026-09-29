package effect

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// castingCreature is a non-player caster, an NPC or a summon, with a cast in
// flight when casting is set. It counts its cast stops and appearance
// refreshes.
type castingCreature struct {
	neutralActor
	world.Presence
	casting, magic bool
	stops          int
	refreshes      int
}

func (c *castingCreature) CastingNow() bool          { return c.casting }
func (c *castingCreature) CurrentSkillIsMagic() bool { return c.casting && c.magic }
func (c *castingCreature) InterruptCast()            {}
func (c *castingCreature) StopCast() {
	c.stops++
	c.casting = false
}
func (c *castingCreature) UpdateAbnormalEffect() { c.refreshes++ }

var _ CasterActor = (*castingCreature)(nil)

// TestCastStoppingEffectsStopNonPlayerCasts pins EffectMute,
// EffectPhysicalMute and EffectSilenceMagicPhysical.onStart on a creature
// that is not a player: Mute stops a magic cast only, PhysicalMute a
// physical one only, SilenceMagicPhysical any cast. Each refreshes the
// creature's appearance once, whether or not it stopped the cast.
func TestCastStoppingEffectsStopNonPlayerCasts(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		magic   bool
		stopped bool
	}{
		{name: "Mute", magic: true, stopped: true},
		{name: "Mute", magic: false},
		{name: "PhysicalMute", magic: false, stopped: true},
		{name: "PhysicalMute", magic: true},
		{name: "SilenceMagicPhysical", magic: true, stopped: true},
		{name: "SilenceMagicPhysical", magic: false, stopped: true},
	} {
		kind := "physical"
		if tt.magic {
			kind = "magic"
		}
		t.Run(tt.name+" on "+kind+" cast", func(t *testing.T) {
			t.Parallel()
			target := &castingCreature{casting: true, magic: tt.magic}
			e := mustNewEffect(t, Skill{ID: 101, Level: 1, Debuff: true}, tt.name)
			e.Effected = target
			if !e.OnStart(e) {
				t.Fatalf("%s OnStart() = false, want true", tt.name)
			}
			if got := target.stops == 1; got != tt.stopped {
				t.Fatalf("cast stops = %d, want stopped %v", target.stops, tt.stopped)
			}
			if target.refreshes != 1 {
				t.Fatalf("appearance refreshes = %d, want 1", target.refreshes)
			}
		})
	}
}

// blessedSummon is a summon that records its blessing stops.
type blessedSummon struct {
	castingCreature
	stopped []string
}

func (*blessedSummon) OwnerID() int32                                  { return 0 }
func (*blessedSummon) OwnerObject() (world.Tracked, bool)              { return nil, false }
func (*blessedSummon) TryToAttack(world.Tracked)                       {}
func (*blessedSummon) TryToFollow(world.Tracked)                       {}
func (*blessedSummon) RandomConfusionTarget(int) (world.Tracked, bool) { return nil, false }
func (*blessedSummon) Think() error                                    { return nil }

func (s *blessedSummon) StopCharmOfLuck(*Effect) {
	s.stopped = append(s.stopped, "CharmOfLuck")
}

func (s *blessedSummon) StopPhoenixBlessing(*Effect) {
	s.stopped = append(s.stopped, "PhoenixBless")
}

var _ SummonActor = (*blessedSummon)(nil)

// TestBlessingExitReachesSummon pins EffectCharmOfLuck.onExit and
// EffectPhoenixBless.onExit on a summon: the ending blessing is handed to
// the summon's own stop (Playable.stopCharmOfLuck / stopPhoenixBlessing).
func TestBlessingExitReachesSummon(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"CharmOfLuck", "PhoenixBless"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			target := &blessedSummon{}
			e := mustNewEffect(t, Skill{ID: 1, Level: 1}, name)
			e.Effected = target
			e.OnExit(e)
			if len(target.stopped) != 1 || target.stopped[0] != name {
				t.Fatalf("summon blessing stops = %v, want [%s]", target.stopped, name)
			}
		})
	}
}
