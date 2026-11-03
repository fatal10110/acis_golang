package ai

import (
	"fmt"
	"slices"
	"testing"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// adenWizardID is an Aden Castle wizard (GludioWizard).
const adenWizardID = int32(35236)

// TestGludioWizardWalksUpToCastAtWhatItSees ports GludioWizard's
// see-creature hook: the wizard that sees the character wants to cast its
// DD magic on it, walking to it, weight 1000000, and casts it at its next
// think.
func TestGludioWizardWalksUpToCastAtWhatItSees(t *testing.T) {
	t.Parallel()
	w := bootSiegeGuards(t, raidBoot{}, siegeCasterTemplate(adenWizardID, npc.SkillTypeDDMagic, siegeDDMagicID))
	wizard := w.spawn(t, adenWizardID, 150, 0)
	w.srv.InstallNPCCastSeam(t, wizard, siegeNPCSkills())

	frames := w.ticks(t, 1)
	wantCasts(t, "after seeing the character", "wizard", wizard, map[modelskill.ID]int32{siegeDDMagicID: w.player.ObjectID()})
	w.srv.AdvanceUntil(t, "the wizard's DD magic", func() bool {
		frames = append(frames, cycle(t, w.srv)...)
		return len(castsOf(frames, wizard, siegeDDMagicID)) > 0
	})
	if got := castsOf(frames, wizard, siegeDDMagicID); !slices.Equal(got, []int32{w.player.ObjectID()}) {
		t.Fatalf("wizard DD magic casts = %v, want one on the character", got)
	}
}

// TestGludioWizardsInPeaceGoHome ports the peace zone branch of the
// see-creature hooks of GludioWizard and GludioWizardUseSkill: a wizard
// standing in a peace zone off its spawn point that sees the character is
// teleported back to its spawn point and drops its attack desires, the
// ones its own hook just added included.
func TestGludioWizardsInPeaceGoHome(t *testing.T) {
	t.Parallel()
	for _, id := range []int32{adenWizardID, wizardUseSkillID} {
		t.Run(fmt.Sprint(id), func(t *testing.T) {
			t.Parallel()
			form, err := zone.NewCuboid(200, 3_000, -1_000, 1_000, -1_000, 1_000)
			if err != nil {
				t.Fatalf("zone form: %v", err)
			}
			zones := zone.NewIndex()
			zones.Add(zone.NewPeace(1, form))
			w := bootSiegeGuards(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithZones(zones)}}, siegeGuardTemplate(id))
			// Out of the character's sight until displaced.
			wizard := w.spawnAt(t, id, location.Location{X: 900, Y: w.at.Y, Z: w.at.Z})
			home := script.NPCOf(wizard).SpawnLocation()
			homeLoc := location.Location{X: home.X, Y: home.Y, Z: home.Z}

			w.teleport(t, wizard, -700, 0)
			if positionOf(wizard) == homeLoc || !wizard.InsideZone(zone.FlagPeace) {
				t.Fatalf("setup: wizard at %v, in peace %v, want off its spawn point %v inside the peace zone", positionOf(wizard), wizard.InsideZone(zone.FlagPeace), homeLoc)
			}
			w.ticks(t, 1)
			if got := positionOf(wizard); got != homeLoc {
				t.Fatalf("wizard at %v after seeing the character in peace, want its spawn point %v", got, homeLoc)
			}
			if d, ok := desire(wizard, actorai.IntentionAttack); ok {
				t.Fatalf("wizard sent home still wants %+v", d)
			}
			if got := wizard.AggroHate(w.player); got != 0 {
				t.Fatalf("wizard sent home hate of the character = %v, want none", got)
			}
		})
	}
}

// TestGludioWizardAnswersAClanCall ports GludioWizard's clan-attacked
// hook: before it has seen anyone, a hit on a clan mate makes the wizard
// want its DD magic on the character, walking to it, weight 1000000, when
// the draw is under 10, and nothing on a draw of 10.
func TestGludioWizardAnswersAClanCall(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		draw int
		want bool
	}{{0, true}, {9, true}, {10, false}} {
		t.Run(fmt.Sprintf("draw %d", c.draw), func(t *testing.T) {
			t.Parallel()
			w := bootSiegeGuards(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithScriptRand(func(int) int { return c.draw })}},
				siegeGuardTemplate(adenKnightID), siegeCasterTemplate(adenWizardID, npc.SkillTypeDDMagic, siegeDDMagicID))
			knight := w.spawn(t, adenKnightID, 100, 0)
			wizard := w.spawn(t, adenWizardID, 100, 150)
			w.srv.InstallNPCCastSeam(t, wizard, siegeNPCSkills())

			w.hit(t, knight, siegeTwentieth)
			want := map[modelskill.ID]int32{}
			if c.want {
				want[siegeDDMagicID] = w.player.ObjectID()
			}
			wantCasts(t, "after the clan call", "wizard", wizard, want)
		})
	}
}

// TestGludioWizardUseSkillShoutsAndCastsAtWhatItSees ports
// GludioWizardUseSkill's see-creature hook: the wizard that sees the
// character wants to attack it with weight 200 and to cast its DD magic on
// it, weight 1000000, a draw of 0 making it shout NpcString 1800012 first.
func TestGludioWizardUseSkillShoutsAndCastsAtWhatItSees(t *testing.T) {
	t.Parallel()
	w := bootSiegeGuards(t, raidBoot{}, siegeSkillfulTemplate(wizardUseSkillID))
	wizard := w.spawn(t, wizardUseSkillID, 150, 0)
	w.srv.InstallNPCCastSeam(t, wizard, siegeSkillfulNPCSkills())

	frames := w.ticks(t, 1)
	if got := shouts(t, frames, wizard.ObjectID()); !slices.Equal(got, []string{siegeShoutLordsOfDawn}) {
		t.Fatalf("wizard shouts = %q, want %q", got, siegeShoutLordsOfDawn)
	}
	if d, ok := desire(wizard, actorai.IntentionAttack); !ok || d.Weight != 200 || d.FinalTarget == nil || d.FinalTarget.ObjectID() != w.player.ObjectID() {
		t.Fatalf("wizard attack desire = %+v, %v, want one on the character, weight 200", d, ok)
	}
	wantCasts(t, "after seeing the character", "wizard", wizard, map[modelskill.ID]int32{siegeDDMagicID: w.player.ObjectID()})
}

// TestGludioWizardUseSkillAnswersAHitOrAClanCall ports
// GludioWizardUseSkill's attacked and clan-attacked hooks: a clanless
// wizard the character hits, and a wizard a hit clan mate calls, each want
// its DD magic on the character on a draw under 50 and its hold magic on a
// draw under 25, both at weight 1000000. Neither hook adds an attack desire
// of its own.
func TestGludioWizardUseSkillAnswersAHitOrAClanCall(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		draw     int
		dd, hold bool
	}{{0, true, true}, {24, true, true}, {25, true, false}, {49, true, false}, {50, false, false}} {
		for _, called := range []bool{false, true} {
			name := fmt.Sprintf("hit, draw %d", c.draw)
			if called {
				name = fmt.Sprintf("clan call, draw %d", c.draw)
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				tmpl := siegeSkillfulTemplate(wizardUseSkillID)
				if !called {
					// Its hit calls no one, itself included.
					tmpl.Clans = nil
				}
				w := bootSiegeGuards(t, raidBoot{opts: []gameservertest.Option{gameservertest.WithScriptRand(func(int) int { return c.draw })}},
					tmpl, siegeGuardTemplate(adenKnightID))
				// Out of the character's sight.
				wizard := w.spawn(t, wizardUseSkillID, 1500, 150)
				w.srv.InstallNPCCastSeam(t, wizard, siegeSkillfulNPCSkills())
				if called {
					w.hit(t, w.spawn(t, adenKnightID, 1500, 0), siegeTwentieth)
				} else {
					w.hit(t, wizard, siegeTwentieth)
				}

				want := map[modelskill.ID]int32{}
				if c.dd {
					want[siegeDDMagicID] = w.player.ObjectID()
				}
				if c.hold {
					want[siegeHoldMagicID] = w.player.ObjectID()
				}
				wantCasts(t, name, "wizard", wizard, want)
				if d, ok := desire(wizard, actorai.IntentionAttack); ok {
					t.Fatalf("wizard wants %+v", d)
				}
			})
		}
	}
}
