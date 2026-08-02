package effect

import (
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestDisablerEffectsRunLiveStartExitHooks(t *testing.T) {
	tests := []struct {
		name      string
		wantStart []string
		wantExit  []string
	}{
		{
			name:      "Stun",
			wantStart: []string{"abort:false", "idle", "abnormal"},
			wantExit:  []string{"abnormal"},
		},
		{
			name:      "Root",
			wantStart: []string{"stop-move", "abnormal"},
			wantExit:  []string{"think", "abnormal"},
		},
		{
			name:      "Sleep",
			wantStart: []string{"abort:false", "abnormal"},
			wantExit:  []string{"think", "abnormal"},
		},
		{
			name:      "Paralyze",
			wantStart: []string{"abort:false"},
			wantExit:  []string{"think"},
		},
		{
			name:      "Petrification",
			wantStart: []string{"abort:false", "invul:true"},
			wantExit:  []string{"think", "invul:false"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := &liveEffectTarget{}
			e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: tt.name})
			if err != nil {
				t.Fatalf("New() error: %v", err)
			}
			e.Effected = target
			list := NewList(nil)

			list.Add(e)
			if !reflect.DeepEqual(target.events, tt.wantStart) {
				t.Fatalf("start events = %#v, want %#v", target.events, tt.wantStart)
			}

			target.events = nil
			list.Remove(e)
			if !reflect.DeepEqual(target.events, tt.wantExit) {
				t.Fatalf("exit events = %#v, want %#v", target.events, tt.wantExit)
			}
		})
	}
}

func TestFearEffectHooksFleeAndRejectImmuneTargets(t *testing.T) {
	target := &liveEffectTarget{}
	e, err := New(Skill{ID: 1092}, modelskill.EffectTemplate{Name: "Fear"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effector = "caster"
	e.Effected = target

	list := NewList(nil)
	list.Add(e)
	if want := []string{"abort:false", "abnormal", "flee:caster:500"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("fear start events = %#v, want %#v", target.events, want)
	}

	target.events = nil
	if !e.ActionTime() {
		t.Fatal("fear action hook stopped, want continuing flee ticks")
	}
	if want := []string{"flee:caster:500"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("fear action events = %#v, want %#v", target.events, want)
	}

	target.events = nil
	list.Remove(e)
	if want := []string{"stop-effects:FEAR", "abnormal"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("fear exit events = %#v, want %#v", target.events, want)
	}

	immune := &liveEffectTarget{fearImmune: true}
	blocked, err := New(Skill{ID: 1092}, modelskill.EffectTemplate{Name: "Fear"})
	if err != nil {
		t.Fatalf("New() immune error: %v", err)
	}
	blocked.Effected = immune
	blockedList := NewList(nil)
	blockedList.Add(blocked)
	if blocked.InUse() {
		t.Fatal("blocked fear effect is in use")
	}
	if got := len(blockedList.All()); got != 0 {
		t.Fatalf("blocked fear effects in list = %d, want 0", got)
	}

	playable := &liveEffectTarget{playable: true}
	skipped, err := New(Skill{ID: 98}, modelskill.EffectTemplate{Name: "Fear", StackType: "turn_flee", StackOrder: 1})
	if err != nil {
		t.Fatalf("New() playable skip error: %v", err)
	}
	skipped.Effected = playable
	skippedList := NewList(nil)
	skippedList.Add(skipped)
	if skipped.InUse() {
		t.Fatal("playable-skipped fear effect is in use")
	}
	if got := len(skippedList.All()); got != 0 {
		t.Fatalf("playable-skipped fear effects in list = %d, want 0", got)
	}
}

// The fear tick counts below (10, halving to 5) are the count/time datapack
// values shared by skill ids 65 ("Horror"), 1092 ("Fear"), and 1169 ("Curse
// Fear")'s own Fear effect entries; id 98 ("Sword Symphony") carries the
// same count but is not one of the halved skill ids.

func TestFearEffectHalvesTickCountAgainstPlayableForListedSkillsOnly(t *testing.T) {
	tests := []struct {
		name      string
		skillID   modelskill.ID
		playable  bool
		wantCount int
	}{
		{"halved: Horror against a playable", 65, true, 5},
		{"halved: Fear against a playable", 1092, true, 5},
		{"halved: Curse Fear against a playable", 1169, true, 5},
		{"not halved: Horror against a non-playable", 65, false, 10},
		{"not halved: Fear against a non-playable", 1092, false, 10},
		{"not halved: an unlisted skill against a playable", 98, true, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := &liveEffectTarget{playable: tt.playable}
			e, err := New(Skill{ID: tt.skillID}, modelskill.EffectTemplate{Name: "Fear", Count: 10, Time: 2})
			if err != nil {
				t.Fatalf("New() error: %v", err)
			}
			e.Effector = "caster"
			e.Effected = target

			e.OnStart(e)

			if e.Template.Count != tt.wantCount {
				t.Fatalf("Template.Count = %d, want %d", e.Template.Count, tt.wantCount)
			}
		})
	}
}

func TestAbortCastEffectHook(t *testing.T) {
	tests := []struct {
		name        string
		selfCast    bool
		raidRelated bool
		castingNow  bool
		wantEvents  []string
		wantInUse   bool
	}{
		{
			name:       "interrupts an in-progress cast",
			castingNow: true,
			wantEvents: []string{"interrupt-cast"},
			wantInUse:  true,
		},
		{
			name:       "no-ops when target is not casting",
			castingNow: false,
			wantEvents: nil,
			wantInUse:  true,
		},
		{
			name:       "rejected on self-cast",
			selfCast:   true,
			castingNow: true,
			wantEvents: nil,
			wantInUse:  false,
		},
		{
			name:        "rejected on a raid-related target",
			raidRelated: true,
			castingNow:  true,
			wantEvents:  nil,
			wantInUse:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := &liveEffectTarget{castingNow: tt.castingNow, raidRelated: tt.raidRelated}
			e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "AbortCast"})
			if err != nil {
				t.Fatalf("New() error: %v", err)
			}
			e.Effected = target
			if tt.selfCast {
				e.Effector = target
			} else {
				e.Effector = "caster"
			}

			NewList(nil).Add(e)
			if !reflect.DeepEqual(target.events, tt.wantEvents) {
				t.Fatalf("events = %#v, want %#v", target.events, tt.wantEvents)
			}
			if e.InUse() != tt.wantInUse {
				t.Fatalf("InUse() = %v, want %v", e.InUse(), tt.wantInUse)
			}
		})
	}
}

func TestMuteFamilyEffectsStopMatchingCastOnly(t *testing.T) {
	tests := []struct {
		name       string
		effect     string
		castingNow bool
		castMagic  bool
		wantEvents []string
	}{
		{name: "Mute interrupts a magic cast", effect: "Mute", castingNow: true, castMagic: true, wantEvents: []string{"stop-cast", "abnormal"}},
		{name: "Mute ignores a physical cast", effect: "Mute", castingNow: true, castMagic: false, wantEvents: []string{"abnormal"}},
		{name: "PhysicalMute interrupts a physical cast", effect: "PhysicalMute", castingNow: true, castMagic: false, wantEvents: []string{"stop-cast", "abnormal"}},
		{name: "PhysicalMute ignores a magic cast", effect: "PhysicalMute", castingNow: true, castMagic: true, wantEvents: []string{"abnormal"}},
		{name: "SilenceMagicPhysical stops any cast unconditionally", effect: "SilenceMagicPhysical", castingNow: true, castMagic: true, wantEvents: []string{"stop-cast", "abnormal"}},
		{name: "SilenceMagicPhysical stops even when the target reports idle", effect: "SilenceMagicPhysical", castingNow: false, wantEvents: []string{"stop-cast", "abnormal"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := &liveEffectTarget{castingNow: tt.castingNow, castMagic: tt.castMagic}
			e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: tt.effect})
			if err != nil {
				t.Fatalf("New() error: %v", err)
			}
			e.Effected = target

			NewList(nil).Add(e)
			if !reflect.DeepEqual(target.events, tt.wantEvents) {
				t.Fatalf("events = %#v, want %#v", target.events, tt.wantEvents)
			}
		})
	}
}

func TestImmobileUntilAttackedEffectLifecycle(t *testing.T) {
	target := &liveEffectTarget{}
	e, err := New(Skill{ID: 77}, modelskill.EffectTemplate{Name: "ImmobileUntilAttacked"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	list := NewList(nil)
	list.Add(e)
	if want := []string{"abort:false", "abnormal"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("start events = %#v, want %#v", target.events, want)
	}

	target.events = nil
	if e.ActionTime() {
		t.Fatal("immobile-until-attacked action hook continued, want a one-shot end")
	}
	if want := []string{"stop-skill:77", "think", "abnormal"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("action events = %#v, want %#v", target.events, want)
	}

	target.events = nil
	list.Remove(e)
	if want := []string{"stop-skill:77", "think", "abnormal"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("exit events = %#v, want %#v", target.events, want)
	}
}

func TestImmobilizeEffectorEffectTargetsEffectorNotEffected(t *testing.T) {
	effected := &liveEffectTarget{}
	effector := &liveEffectTarget{}
	e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "ImobileBuff"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = effected
	e.Effector = effector

	list := NewList(nil)
	list.Add(e)
	if want := []string{"immobilized:true"}; !reflect.DeepEqual(effector.events, want) {
		t.Fatalf("effector start events = %#v, want %#v", effector.events, want)
	}
	if len(effected.events) != 0 {
		t.Fatalf("effected events = %#v, want none", effected.events)
	}

	list.Remove(e)
	if want := []string{"immobilized:true", "immobilized:false"}; !reflect.DeepEqual(effector.events, want) {
		t.Fatalf("effector exit events = %#v, want %#v", effector.events, want)
	}
}

func TestInvincibleEffectTogglesInvulOnStartAndExit(t *testing.T) {
	target := &liveEffectTarget{}
	e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "Invincible"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	list := NewList(nil)
	list.Add(e)
	if want := []string{"invul:true"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("start events = %#v, want %#v", target.events, want)
	}

	list.Remove(e)
	if want := []string{"invul:true", "invul:false"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("exit events = %#v, want %#v", target.events, want)
	}
}

func TestRemoveTargetEffectClearsTargetAttackAndCast(t *testing.T) {
	target := &liveEffectTarget{}
	e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "RemoveTarget"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	NewList(nil).Add(e)
	want := []string{"clear-target", "stop-attack", "stop-cast"}
	if !reflect.DeepEqual(target.events, want) {
		t.Fatalf("events = %#v, want %#v", target.events, want)
	}
}

func TestSilentMoveActionOnlyTicksContSkillsAndStopsOnLowMana(t *testing.T) {
	target := &liveEffectTarget{mp: 10}
	e, err := New(Skill{ID: 1, SkillType: "CONT"}, modelskill.EffectTemplate{Name: "SilentMove", Value: 4})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if !e.ActionTime() {
		t.Fatal("silent move action stopped on a CONT skill with enough mana")
	}
	if target.mp != 6 {
		t.Fatalf("target mp = %v, want 6", target.mp)
	}
	if want := []string{"mpdot:4"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("events = %#v, want %#v", target.events, want)
	}
	// EffectSilentMove.java:35-41 drains MP through the same reduceMp/setMp
	// chain as EffectManaDamOverTime; this tick must broadcast it too.
	if target.mpBroadcasts != 1 {
		t.Fatalf("mp broadcasts = %d, want 1", target.mpBroadcasts)
	}

	target.events = nil
	target.mp = 2
	if e.ActionTime() {
		t.Fatal("silent move action continued with insufficient mana, want stop")
	}
	if want := []string{"lack-mp"}; !reflect.DeepEqual(target.events, want) {
		t.Fatalf("low-mana events = %#v, want %#v", target.events, want)
	}
	// A toggle stopped for lack of MP never calls ReduceMP, so it must not
	// broadcast either — the count from the earlier successful tick stays.
	if target.mpBroadcasts != 1 {
		t.Fatalf("low-mana mp broadcasts = %d, want 1 (unchanged)", target.mpBroadcasts)
	}

	nonCont, err := New(Skill{ID: 1, SkillType: "BUFF"}, modelskill.EffectTemplate{Name: "SilentMove", Value: 4})
	if err != nil {
		t.Fatalf("New() non-CONT error: %v", err)
	}
	nonCont.Effected = &liveEffectTarget{mp: 10}
	if nonCont.ActionTime() {
		t.Fatal("silent move action ticked on a non-CONT skill, want immediate stop")
	}
}

func TestStunSelfEffectIdlesEffectedAndRefreshesEffector(t *testing.T) {
	effected := &liveEffectTarget{playable: true}
	effector := &liveEffectTarget{}
	e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "StunSelf"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = effected
	e.Effector = effector

	list := NewList(nil)
	list.Add(e)
	if want := []string{"idle"}; !reflect.DeepEqual(effected.events, want) {
		t.Fatalf("effected start events = %#v, want %#v", effected.events, want)
	}
	if want := []string{"abnormal"}; !reflect.DeepEqual(effector.events, want) {
		t.Fatalf("effector start events = %#v, want %#v", effector.events, want)
	}

	list.Remove(e)
	if want := []string{"abnormal", "abnormal"}; !reflect.DeepEqual(effector.events, want) {
		t.Fatalf("effector exit events = %#v, want %#v", effector.events, want)
	}
}

func TestImmobilizePetBuffEffectLocksOnlyASummonOwnedByThePlayerEffector(t *testing.T) {
	owner := &liveEffectTarget{isPlayer: true, objectID: 42}
	summon := &liveEffectTarget{ownerID: 42}
	e, err := New(Skill{}, modelskill.EffectTemplate{Name: "ImobilePetBuff"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effector = owner
	e.Effected = summon

	if !e.OnStart(e) {
		t.Fatal("immobilize pet buff effect start rejected the summon's own owner")
	}
	if want := []string{"immobilized:true"}; !reflect.DeepEqual(summon.events, want) {
		t.Fatalf("events = %#v, want %#v", summon.events, want)
	}

	e.OnExit(e)
	if want := []string{"immobilized:true", "immobilized:false"}; !reflect.DeepEqual(summon.events, want) {
		t.Fatalf("events after exit = %#v, want %#v", summon.events, want)
	}

	notOwner := &liveEffectTarget{isPlayer: true, objectID: 99}
	otherSummon := &liveEffectTarget{ownerID: 42}
	e2, err := New(Skill{}, modelskill.EffectTemplate{Name: "ImobilePetBuff"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e2.Effector = notOwner
	e2.Effected = otherSummon
	if e2.OnStart(e2) {
		t.Fatal("immobilize pet buff effect started for a non-owner effector")
	}
	if len(otherSummon.events) != 0 {
		t.Fatalf("events = %#v, want none", otherSummon.events)
	}

	nonPlayer := &liveEffectTarget{isPlayer: false, objectID: 42}
	e3, err := New(Skill{}, modelskill.EffectTemplate{Name: "ImobilePetBuff"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e3.Effector = nonPlayer
	e3.Effected = &liveEffectTarget{ownerID: 42}
	if e3.OnStart(e3) {
		t.Fatal("immobilize pet buff effect started for a non-player effector")
	}
}

func TestThrowUpEffectComputesLandingAndFliesThenTeleportsOnExit(t *testing.T) {
	effector := &liveEffectTarget{x: 100, y: 0, z: 0}
	effected := &liveEffectTarget{x: 0, y: 0, z: 0}

	e, err := New(Skill{ID: 1, FlyRadius: 600}, modelskill.EffectTemplate{Name: "ThrowUp"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effector = effector
	e.Effected = effected

	if !e.OnStart(e) {
		t.Fatal("throw-up effect start rejected a valid range")
	}
	// distance=100, offset=min(100+600,1400)=700, cos=1, sin=0:
	// x = 100 - 700*1 = -600, y = 0, z = effected's Z at cast time (0).
	want := location.Location{X: -600, Y: 0, Z: 0}
	if e.landing != want {
		t.Fatalf("landing = %+v, want %+v", e.landing, want)
	}
	if effected.flightDest != want {
		t.Fatalf("FlyTo dest = %+v, want %+v", effected.flightDest, want)
	}
	if effected.flightType != modelskill.FlightThrowUp {
		t.Fatalf("FlyTo flight = %v, want FlightThrowUp", effected.flightType)
	}
	if effected.x != 0 || effected.y != 0 || effected.z != 0 {
		t.Fatal("throw-up effect start must not move the target before exit")
	}

	e.OnExit(e)
	if effected.x != want.X || effected.y != want.Y || effected.z != want.Z {
		t.Fatalf("position after exit = (%d,%d,%d), want (%d,%d,%d)", effected.x, effected.y, effected.z, want.X, want.Y, want.Z)
	}
	if want := []string{"abort:false", "abnormal", "fly", "abnormal", "broadcast"}; !reflect.DeepEqual(effected.events, want) {
		t.Fatalf("effected events = %#v, want %#v", effected.events, want)
	}
}

func TestThrowUpEffectAppliesGeoCorrectedXYButKeepsOriginalZ(t *testing.T) {
	effector := &liveEffectTarget{x: 100, y: 0, z: 500}
	effected := &liveEffectTarget{x: 0, y: 0, z: 0}
	effected.validLocationFn = func(ox, oy, oz, tx, ty, tz int) location.Location {
		return location.Location{X: tx / 2, Y: ty, Z: 999}
	}

	e, err := New(Skill{ID: 1, FlyRadius: 600}, modelskill.EffectTemplate{Name: "ThrowUp"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effector = effector
	e.Effected = effected

	if !e.OnStart(e) {
		t.Fatal("throw-up effect start rejected a valid range")
	}
	// offset = min(100+600,1400) + |effector.Z-effected.Z| = 700+500 = 1200,
	// so raw x before geo correction is 100-1200 = -1100; geo halves it to
	// -550. Z stays the effected's Z at cast time (0), never the
	// geo-returned 999.
	want := location.Location{X: -550, Y: 0, Z: 0}
	if e.landing != want {
		t.Fatalf("landing = %+v, want %+v (geo-corrected X/Y, uncorrected Z)", e.landing, want)
	}
}

func TestThrowUpEffectOutOfRangeAbortsButStillAbortsCurrentAction(t *testing.T) {
	effector := &liveEffectTarget{x: 3000, y: 0, z: 0}
	effected := &liveEffectTarget{x: 0, y: 0, z: 0}

	e, err := New(Skill{ID: 1, FlyRadius: 600}, modelskill.EffectTemplate{Name: "ThrowUp"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effector = effector
	e.Effected = effected

	if e.OnStart(e) {
		t.Fatal("throw-up effect start accepted a distance beyond the 2000-unit range gate")
	}
	if want := []string{"abort:false"}; !reflect.DeepEqual(effected.events, want) {
		t.Fatalf("effected events = %#v, want %#v (abort still runs before the range gate, but no fly)", effected.events, want)
	}
}

// growEffectTarget is a minimal Npc-shaped actor implementing only the
// collision-radius override surface Grow needs.
