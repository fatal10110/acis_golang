package skills

import (
	"slices"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// The cancel family strips at most the skill's maxNegated effects per target
// and, for the two banes, only effects of their own stack types. Which of the
// eligible effects a cast reaches is random (the order is shuffled), so these
// scenarios choose eligible sets and caps whose outcome does not depend on the
// order, or count how many rolls were drawn. A caster pinned to roll 0 lands
// every strip it attempts: each skill's power puts its rate above the clamp
// floor of 25 (cancel) or 40 (banes).

// baneWorld boots a caster whose single skill is of skillType, capped at
// maxNegated effects per target, with power 30 and the caster's own level.
func baneWorld(t *testing.T, skillType modelskill.SkillType, maxNegated int) cancelWorld {
	t.Helper()
	def := modelskill.Definition{
		ID: cancelSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 0, StaticHitTime: true, SkillType: skillType, Power: 30, MagicLevel: cancelCasterLevel,
	}
	extra := *def.Extra()
	extra.MaxNegatedEffects = maxNegated
	return bootCancelWorld(t, def.WithExtra(extra))
}

// stackedBuffs lands one buff per stack type on the caster, in order.
func (w cancelWorld) stackedBuffs(t *testing.T, sk effect.Skill, stackTypes ...string) []*effect.Effect {
	t.Helper()
	effects := make([]*effect.Effect, len(stackTypes))
	for i, st := range stackTypes {
		tmpl := buffTemplate(3600)
		tmpl.StackType = st
		effects[i] = w.addCandidate(t, sk, tmpl)
	}
	return effects
}

// cast has the caster draw its dice from roll, casts once, and returns the
// subset of candidates still on its effect list. The strip happens at the
// hit, a separate step after the launch frame: the cast has landed once the
// launch is read and the server has run what the launch armed, so the effects
// are read then, not after a read that may find the hit still pending.
func (w cancelWorld) cast(t *testing.T, roll func(int) int, candidates []*effect.Effect) (kept []*effect.Effect) {
	t.Helper()
	onPlayerQueue(t, w.srv, w.objID, func(pc *player.Character) { pc.SetRollSource(roll) })
	w.srv.Client.Send(encodeRequestMagicSkillUse(cancelSkillID, false, false))
	readUntil(t, w.srv.Client, serverpackets.OpcodeMagicSkillLaunched)
	w.srv.Settle(t)
	drainUntilQuiet(t, w.srv.Client)
	onPlayerQueue(t, w.srv, w.objID, func(pc *player.Character) {
		held := pc.EffectList().All()
		for _, e := range candidates {
			if slices.Contains(held, e) {
				kept = append(kept, e)
			}
		}
	})
	return kept
}

func rollZero(int) int { return 0 }

// partition splits candidates into those in set and those not.
func partition(candidates, set []*effect.Effect) (in, out []*effect.Effect) {
	for _, e := range candidates {
		if slices.Contains(set, e) {
			in = append(in, e)
		} else {
			out = append(out, e)
		}
	}
	return in, out
}

// TestCancelStripsAtMostMaxNegatedEffects casts a CANCEL skill capped at 2
// against four held buffs with every roll succeeding: exactly two leave and
// two stay, whichever two the shuffle reaches.
func TestCancelStripsAtMostMaxNegatedEffects(t *testing.T) {
	t.Parallel()
	w := baneWorld(t, "CANCEL", 2)
	buffs := w.stackedBuffs(t, effect.Skill{}, "cancel_a", "cancel_b", "cancel_c", "cancel_d")
	kept := w.cast(t, rollZero, buffs)
	if len(kept) != 2 {
		t.Fatalf("%d of 4 buffs survive a cast capped at 2, want 2", len(kept))
	}
}

// TestCancelWithoutCapStripsEveryEligibleEffect casts a CANCEL skill whose
// maxNegated is -1 (no cap) at four buffs, a toggle and a debuff: every buff
// leaves, the toggle and the debuff are never candidates.
func TestCancelWithoutCapStripsEveryEligibleEffect(t *testing.T) {
	t.Parallel()
	w := baneWorld(t, "CANCEL", -1)
	buffs := w.stackedBuffs(t, effect.Skill{}, "cancel_a", "cancel_b", "cancel_c", "cancel_d")
	toggle := w.stackedBuffs(t, effect.Skill{Toggle: true}, "cancel_toggle")
	debuff := w.stackedBuffs(t, effect.Skill{Debuff: true, CanBeDispelled: true}, "cancel_debuff")
	all := slices.Concat(buffs, toggle, debuff)

	kept := w.cast(t, rollZero, all)
	want := slices.Concat(toggle, debuff)
	if !slices.Equal(sortedByStack(kept), sortedByStack(want)) {
		t.Fatalf("survivors = %v, want the toggle and the debuff only", stackNames(kept))
	}
}

// TestCancelFailedRollStillSpendsCount casts a CANCEL skill capped at 2
// against four buffs with the first roll failing and the rest landing: the
// failed strip spent one of the two, so one buff leaves, not two.
func TestCancelFailedRollStillSpendsCount(t *testing.T) {
	t.Parallel()
	w := baneWorld(t, "CANCEL", 2)
	buffs := w.stackedBuffs(t, effect.Skill{}, "cancel_a", "cancel_b", "cancel_c", "cancel_d")
	var drawn atomic.Int32
	roll := func(int) int {
		if drawn.Add(1) == 1 {
			return 99
		}
		return 0
	}
	kept := w.cast(t, roll, buffs)
	if len(kept) != 3 {
		t.Fatalf("%d of 4 buffs survive (%d rolls drawn), want 3: a failed roll spends the count", len(kept), drawn.Load())
	}
	if got := drawn.Load(); got != 2 {
		t.Fatalf("%d rolls drawn, want 2 (one per count spent)", got)
	}
}

// TestMageBaneStripsOnlyItsStackTypes casts MAGE_BANE with no cap at buffs of
// its two stack types (one spelled in capitals) and of four others: only the
// matching ones leave.
func TestMageBaneStripsOnlyItsStackTypes(t *testing.T) {
	t.Parallel()
	w := baneWorld(t, "MAGE_BANE", -1)
	matching := w.stackedBuffs(t, effect.Skill{}, "casting_time_down", "ma_up", "MA_UP")
	other := w.stackedBuffs(t, effect.Skill{}, "attack_time_down", "speed_up", "cancel_a", "cancel_b")

	kept := w.cast(t, rollZero, slices.Concat(matching, other))
	if !slices.Equal(sortedByStack(kept), sortedByStack(other)) {
		t.Fatalf("survivors = %v, want exactly %v", stackNames(kept), stackNames(other))
	}
}

// TestWarriorBaneStripsOnlyItsStackTypes is the mirror for WARRIOR_BANE.
func TestWarriorBaneStripsOnlyItsStackTypes(t *testing.T) {
	t.Parallel()
	w := baneWorld(t, "WARRIOR_BANE", -1)
	matching := w.stackedBuffs(t, effect.Skill{}, "attack_time_down", "speed_up", "SPEED_UP")
	other := w.stackedBuffs(t, effect.Skill{}, "casting_time_down", "ma_up", "cancel_a", "cancel_b")

	kept := w.cast(t, rollZero, slices.Concat(matching, other))
	if !slices.Equal(sortedByStack(kept), sortedByStack(other)) {
		t.Fatalf("survivors = %v, want exactly %v", stackNames(kept), stackNames(other))
	}
}

// TestBaneFilteredEffectsDoNotSpendCount caps each bane at 2 and holds
// exactly two matching buffs among six of other types: the cap counts only
// effects the bane considers, so both matching buffs leave whatever order
// the shuffle puts the others in.
func TestBaneFilteredEffectsDoNotSpendCount(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		skillType modelskill.SkillType
		matching  []string
		other     []string
	}{
		{"MAGE_BANE", []string{"casting_time_down", "ma_up"}, []string{"attack_time_down", "speed_up", "cancel_a", "cancel_b", "cancel_c", "cancel_d"}},
		{"WARRIOR_BANE", []string{"attack_time_down", "speed_up"}, []string{"casting_time_down", "ma_up", "cancel_a", "cancel_b", "cancel_c", "cancel_d"}},
	} {
		t.Run(string(tc.skillType), func(t *testing.T) {
			t.Parallel()
			w := baneWorld(t, tc.skillType, 2)
			// Other buffs land first so a cap spent on them would show.
			other := w.stackedBuffs(t, effect.Skill{}, tc.other...)
			matching := w.stackedBuffs(t, effect.Skill{}, tc.matching...)

			kept := w.cast(t, rollZero, slices.Concat(other, matching))
			if left, _ := partition(kept, matching); len(left) != 0 {
				t.Errorf("matching buffs %v survive a cap of 2 with two eligible", stackNames(left))
			}
			if len(kept) != len(other) {
				t.Errorf("%d buffs survive, want the %d of other types", len(kept), len(other))
			}
		})
	}
}

// TestBaneCapStripsAtMostMaxNegatedMatching holds three matching buffs under
// a cap of 2: exactly two leave, and the unmatched ones are untouched.
func TestBaneCapStripsAtMostMaxNegatedMatching(t *testing.T) {
	t.Parallel()
	w := baneWorld(t, "MAGE_BANE", 2)
	matching := w.stackedBuffs(t, effect.Skill{}, "casting_time_down", "ma_up", "MA_UP")
	other := w.stackedBuffs(t, effect.Skill{}, "speed_up")

	kept := w.cast(t, rollZero, slices.Concat(matching, other))
	keptMatching, keptOther := partition(kept, matching)
	if len(keptMatching) != 1 || len(keptOther) != 1 {
		t.Fatalf("survivors = %v, want one matching buff and the speed_up buff", stackNames(kept))
	}
}

func stackNames(effects []*effect.Effect) []string {
	names := make([]string, len(effects))
	for i, e := range effects {
		names[i] = e.Template.StackType
	}
	return names
}

func sortedByStack(effects []*effect.Effect) []string {
	names := stackNames(effects)
	slices.Sort(names)
	return names
}
