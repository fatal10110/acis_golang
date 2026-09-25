package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/handler/skill/skilltest"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestSendSkillHandlerResultDeliversTargetMessagesWithNilCaster pins the
// nil-live-safe split (issue #2350): a caster with no live connection (a
// hostile NPC's AIController.OnHitResult) still delivers target-addressed
// messages resolved by ID lookup, while caster-addressed messages, which
// have nowhere to go, are silently skipped instead of panicking on the nil
// receiver.
func TestSendSkillHandlerResultDeliversTargetMessagesWithNilCaster(t *testing.T) {
	frames := &testsupport.FrameCapture{}
	target := newTestLivePlayer(t, 42, frames)
	state := world.New()
	state.AddPlayer(target)
	l := &GameClientLink{world: state}

	l.sendSkillHandlerResult(nil, actorcast.EffectResult{
		Messages: []any{
			handlerskill.MagicResist{TargetID: 42, AttackerName: "Orc"},
			handlerskill.ManaDrain{TargetID: 42, CasterName: "Orc", MP: 30},
			handlerskill.Resisted{TargetName: "Orc", SkillID: 1, SkillLevel: 1},
			handlerskill.AttackFailedMessage{},
			handlerskill.ManaDamageMissedMessage{},
			handlerskill.OpponentMPReducedMessage{MP: 5},
		},
	})

	got := frames.Frames()
	if len(got) != 2 {
		t.Fatalf("frame count = %d, want 2 (MagicResist, ManaDrain) and nothing else", len(got))
	}
	assertSystemMessageStringFrame(t, got[0], serverpackets.SystemMessageResistedS1Magic, "Orc")
	assertSystemMessageStringNumberFrame(t, got[1], serverpackets.SystemMessageS2MPHasBeenDrainedByS1, "Orc", 30)
}

func TestSendSkillHandlerResultKeepsTargetMessageOrder(t *testing.T) {
	resist := handlerskill.Resisted{TargetName: "First", SkillID: 1, SkillLevel: 1}
	counter := handlerskill.Counterattack{AttackerID: 1, DefenderID: 3, DefenderName: "Second"}
	for _, tc := range []struct {
		name     string
		messages []any
		want     []int
	}{
		{"blow resist before counter", []any{resist, counter}, []int{serverpackets.SystemMessageS1ResistedYourS2, serverpackets.SystemMessageS1PerformingCounterattack}},
		{"pdam dodge before later counter", []any{handlerskill.Dodge{AttackerID: 1, DefenderID: 2}, counter}, []int{serverpackets.SystemMessageS1DodgesAttack, serverpackets.SystemMessageS1PerformingCounterattack}},
		{"pdam failure before later lethal", []any{handlerskill.AttackFailedMessage{}, handlerskill.Lethal{AttackerID: 1, TargetID: 3}}, []int{serverpackets.SystemMessageAttackFailed, serverpackets.SystemMessageLethalStrikeSuccessful}},
		{"manadam miss before later resist", []any{handlerskill.ManaDamageMissedMessage{}, resist}, []int{serverpackets.SystemMessageMissedTarget, serverpackets.SystemMessageS1ResistedYourS2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			frames := &testsupport.FrameCapture{}
			caster := newTestLivePlayer(t, 1, frames)
			state := world.New()
			state.AddPlayer(caster)
			state.AddPlayer(newTestLivePlayer(t, 2, &testsupport.FrameCapture{}))
			state.AddPlayer(newTestLivePlayer(t, 3, &testsupport.FrameCapture{}))
			l := &GameClientLink{world: state}
			l.sendSkillHandlerResult(caster, actorcast.EffectResult{Messages: tc.messages})
			got := frames.Frames()
			if len(got) != len(tc.want) {
				t.Fatalf("caster frame count = %d, want %d", len(got), len(tc.want))
			}
			for i, want := range tc.want {
				assertSystemMessageIDFrame(t, got[i], want)
			}
		})
	}
}

type orderedSkillActor struct {
	skilltest.Creature
	world.Presence
	id      int32
	kind    actor.Kind
	effects *effect.List
	counter float64
	blow    formulas.BlowInput
}

func (a *orderedSkillActor) ObjectID() int32                                 { return a.id }
func (a *orderedSkillActor) Kind() actor.Kind                                { return a.kind }
func (a *orderedSkillActor) CharacterName() string                           { return "Target" }
func (a *orderedSkillActor) AttackableBy(skilltarget.Actor) bool             { return true }
func (a *orderedSkillActor) AttackableWithoutForceBy(skilltarget.Actor) bool { return true }
func (a *orderedSkillActor) TestCursesOnSkillSee(modelskill.Definition, []skilltarget.Actor) bool {
	return false
}
func (a *orderedSkillActor) NotePvPSkillTargets([]attackable.Combatant, bool, string) {}
func (a *orderedSkillActor) EffectList() *effect.List                                 { return a.effects }
func (a *orderedSkillActor) CounterSkillPhysical() float64                            { return a.counter }
func (a *orderedSkillActor) BlowInput(creature.FormulaActor, modelskill.Definition) (formulas.BlowInput, bool) {
	return a.blow, true
}
func (a *orderedSkillActor) SkillSuccessInput(creature.FormulaActor, modelskill.Definition, bool, formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	return formulas.SkillSuccessInput{IgnoreResists: true, BaseChance: 0}, true
}

func TestSkillMessageOrderThroughCastAdapters(t *testing.T) {
	for _, cubic := range []bool{false, true} {
		name := "resolved cast"
		if cubic {
			name = "cubic cast"
		}
		t.Run(name, func(t *testing.T) {
			frames := &testsupport.FrameCapture{}
			live := newTestLivePlayer(t, 1, frames)
			state := world.New()
			state.AddPlayer(live)
			state.AddPlayer(newTestLivePlayer(t, 2, &testsupport.FrameCapture{}))
			link := &GameClientLink{world: state}
			caster := &orderedSkillActor{id: 1, kind: actor.KindPlayer}
			target := &orderedSkillActor{id: 2, kind: actor.KindPlayer, effects: effect.NewList(nil), counter: 100,
				blow: formulas.BlowInput{Landed: true, AttackPower: 100, SkillPower: 50, Defence: 50, RandomMul: 1, PosMul: 1}}
			def := modelskill.Definition{ID: 7, Level: 20, SkillType: "BLOW", Target: modelskill.TargetOne, Offensive: true,
				CastRange: 40, CanBeReflected: true, Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 10}}}
			skills := handlerskill.NewDefaultRegistry()
			var result actorcast.EffectResult
			if cubic {
				result = actorcast.ApplyCubicEffect(skills, caster, def, target)
			} else {
				result = actorcast.ApplyEffectsResult(actorcast.EffectHandlers{Targets: skilltarget.NewRegistry(nil), Skills: skills}, caster, target, def)
			}
			if !result.Handled {
				t.Fatal("cast was not handled")
			}
			link.sendSkillHandlerResult(live, result)
			got := frames.Frames()
			if len(got) != 2 {
				t.Fatalf("caster frame count = %d, want 2", len(got))
			}
			assertSystemMessageStringSkillNameFrame(t, got[0], serverpackets.SystemMessageS1ResistedYourS2, "Target", 7, 20)
			assertSystemMessageStringFrame(t, got[1], serverpackets.SystemMessageS1PerformingCounterattack, "Player")
		})
	}
}

// TestDeliverHitResultForwardsToSendSkillHandlerResult pins the exported
// wrapper boot wiring uses to give a hostile NPC's OnHitResult a delivery
// path with no live caster connection (issue #2350).
func TestDeliverHitResultForwardsToSendSkillHandlerResult(t *testing.T) {
	frames := &testsupport.FrameCapture{}
	target := newTestLivePlayer(t, 43, frames)
	state := world.New()
	state.AddPlayer(target)
	l := &GameClientLink{world: state}

	l.DeliverHitResult(actorcast.EffectResult{
		Messages: []any{handlerskill.ManaDrain{TargetID: 43, CasterName: "Orc", MP: 12}},
	})

	got := frames.Frames()
	if len(got) != 1 {
		t.Fatalf("frame count = %d, want 1 (ManaDrain)", len(got))
	}
	assertSystemMessageStringNumberFrame(t, got[0], serverpackets.SystemMessageS2MPHasBeenDrainedByS1, "Orc", 12)
}

func TestTargetCastRejectionsSendMessageBeforeActionFailed(t *testing.T) {
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, frames)

	for _, tt := range []struct {
		name      string
		rejection skilltarget.CastRejection
		def       modelskill.Definition
		message   int
	}{
		{"aura", skilltarget.CastRejectCantAttackPeaceZone, modelskill.Definition{}, serverpackets.SystemMessageCantAtkPeacezone},
		{"front aura", skilltarget.CastRejectCantAttackPeaceZone, modelskill.Definition{}, serverpackets.SystemMessageCantAtkPeacezone},
		{"behind aura", skilltarget.CastRejectCantAttackPeaceZone, modelskill.Definition{}, serverpackets.SystemMessageCantAtkPeacezone},
		{"one invalid", skilltarget.CastRejectInvalidTarget, modelskill.Definition{}, serverpackets.SystemMessageInvalidTarget},
		{"one target in peace", skilltarget.CastRejectTargetInPeaceZone, modelskill.Definition{}, serverpackets.SystemMessageTargetInPeacezone},
		{"corpse pet non-pet", skilltarget.CastRejectCannotUseSkill, modelskill.Definition{ID: 2179, Level: 1}, serverpackets.SystemMessageS1CannotBeUsed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			testsupport.ResetCapture(frames)
			sendTargetCastRejection(live, tt.rejection, tt.def)
			sendMagicActionFailed(live)

			got := frames.Frames()
			if opcodes := testsupport.FrameOpcodes(got); len(opcodes) != 2 || opcodes[0] != serverpackets.OpcodeSystemMessage || opcodes[1] != serverpackets.OpcodeActionFailed {
				t.Fatalf("opcodes = %x, want [SystemMessage, ActionFailed]", opcodes)
			}
			if tt.def.ID != 0 {
				assertSystemMessageSkillFrame(t, got[0], tt.message, int32(tt.def.ID), int32(tt.def.Level))
				return
			}
			assertStaticSystemMessageFrame(t, got[0], tt.message)
		})
	}
}

type magicSkillDoorShape struct{}

func (magicSkillDoorShape) GeoX() int               { return 0 }
func (magicSkillDoorShape) GeoY() int               { return 0 }
func (magicSkillDoorShape) GeoZ() int               { return 0 }
func (magicSkillDoorShape) Height() int             { return 0 }
func (magicSkillDoorShape) GeoData() [][]block.NSWE { return [][]block.NSWE{{block.AllDirections}} }

func TestResolveMagicSkillTargetKeepsLockedDoorForSilentRejection(t *testing.T) {
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	locked, err := door.NewObject(2, &door.Template{ID: 2, OpenKind: door.OpenClick}, magicSkillDoorShape{})
	if err != nil {
		t.Fatal(err)
	}
	l := &GameClientLink{targets: skilltarget.NewRegistry(nil)}
	target, rejection := l.resolveMagicSkillTarget(live.Character, locked, modelskill.Definition{Target: modelskill.TargetUnlockable}, false)
	if target != locked {
		t.Fatalf("target = %T, want locked door", target)
	}
	if rejection != skilltarget.CastRejectSilent {
		t.Fatalf("rejection = %v, want silent", rejection)
	}
	testsupport.ResetCapture(frames)
	sendTargetCastRejection(live, rejection, modelskill.Definition{})
	if got := testsupport.FrameOpcodes(frames.Frames()); len(got) != 0 {
		t.Fatalf("silent rejection opcodes = %x, want none", got)
	}
	testsupport.ResetCapture(frames)
	l.rejectMagicCast(live, modelskill.Definition{HitTime: 500}, locked)
	if got := testsupport.FrameOpcodes(frames.Frames()); len(got) != 1 || got[0] != serverpackets.OpcodeMoveToPawn {
		t.Fatalf("silent rejection opcodes = %x, want [MoveToPawn]", got)
	}
}

// TestFusionTargetClearsOnlyTheChannelThatSetIt pins the clear-if-still-mine
// rule the fusion target accessors carry: handleMagicSkillUse installs the
// channel's target (magic_skill.go:129) and its finishFusion clears it by the
// same id (magic_skill.go:132). When an earlier channel finishes after the
// caster has started a new one, clearing must be a no-op, or
// abortFusionTargeting (magic_skill.go:344) stops recognizing the caster as
// fusing its current target.
func TestFusionTargetClearsOnlyTheChannelThatSetIt(t *testing.T) {
	const (
		first  = int32(11)
		second = int32(22)
	)

	var live livePlayer
	live.setFusionTarget(first)
	live.setFusionTarget(second)

	live.clearFusionTarget(first)
	if !live.fusesTarget(second) {
		t.Fatal("fusesTarget(second) = false after the superseded channel cleared its own target, want true")
	}
	if live.fusesTarget(first) {
		t.Fatal("fusesTarget(first) = true, want the superseded target gone")
	}

	live.clearFusionTarget(second)
	if live.fusesTarget(second) {
		t.Fatal("fusesTarget(second) = true after its own channel cleared it, want false")
	}
}
