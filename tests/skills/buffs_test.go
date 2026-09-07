package skills

import (
	"slices"
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestBuffIconPersistsUntilExpiry casts a self-buff and walks its visible
// lifetime: the icon appears in AbnormalStatusUpdate with the definition's
// remaining duration, and the production effect sweep retires it on time,
// clearing the icon list.
func TestBuffIconPersistsUntilExpiry(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{
				ID: 4, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
				MPInitialConsume: 2, MPConsume: 3, SkillType: "BUFF",
				Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 2, Icon: true}},
			},
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 4, 1)
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(4, false, false))
	readCastStartFrames(t, c, objID, 4, 1, 500, 60_000, objID)
	icons := readStatusUpdateSkippingAbnormal(t, c, objID, []serverpackets.StatusAttribute{{Type: serverpackets.StatusCurrentMP, Value: 25}})
	found := false
	for _, e := range icons {
		if e.SkillID == 4 && int32(e.Level) == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("AbnormalStatusUpdate icons after buff cast = %+v, want skill 4", icons)
	}
	drainUntilQuiet(t, c)

	time.Sleep(2200 * time.Millisecond)
	srv.TickEffects()
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageS1HasWornOff, 4, 1)
	if entries := readAbnormalStatusUpdateEntries(t, c); len(entries) != 0 {
		t.Fatalf("AbnormalStatusUpdate entries after expiry = %+v, want none", entries)
	}
	drainUntilQuiet(t, c)
}

// TestDebuffThatFailsToLandSendsAttackFailed verifies a debuff with no land
// chance still plays the cast (ack, use message, launch report) but answers
// with the attack-failed message and applies nothing.
func TestDebuffThatFailsToLandSendsAttackFailed(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{
				ID: 5, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				SkillType: "DEBUFF", EffectType: "DEBUFF", Debuff: true,
				BaseLandRate: 0, IgnoreResists: true,
				Effects: []modelskill.EffectTemplate{{Name: "Debuff", Time: 60}},
			},
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 5, 1)
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(5, false, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillUse, "MagicSkillUse")
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageUseS1, 5, 1)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillLaunched, "MagicSkillLaunched")
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageAttackFailed)
	drainUntilQuiet(t, c)
}

// TestDebuffDoesNotLandOnInvulnerableNPC casts a guaranteed-land debuff at
// an invulnerable monster: the cast still plays, AttackFailed stays silent
// (the land roll succeeded), and the NPC receives no effect.
func TestDebuffDoesNotLandOnInvulnerableNPC(t *testing.T) {
	srv, c, _ := bootOneShotDebuff(t, 6)
	hostile := srv.SpawnHostileNPC(t)
	hostile.SetInvul(true)
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(6, false, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillUse, "MagicSkillUse")
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageUseS1, 6, 1)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillLaunched, "MagicSkillLaunched")
	drainAssertingNoAttackFailed(t, c)

	if got := len(hostile.EffectList().All()); got != 0 {
		t.Fatalf("invulnerable NPC effects = %d, want 0", got)
	}
}

// TestDebuffDoesNotLandWhenCasterCannotGiveDamage is the access-level
// sibling of invulnerability: a caster forbidden to deal damage still
// completes the cast without AttackFailed, but the debuff does not apply.
func TestDebuffDoesNotLandWhenCasterCannotGiveDamage(t *testing.T) {
	srv, c, objID := bootOneShotDebuff(t, 7)
	denyCasterDamage(t, srv, objID)
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(7, false, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillUse, "MagicSkillUse")
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageUseS1, 7, 1)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillLaunched, "MagicSkillLaunched")
	drainAssertingNoAttackFailed(t, c)

	if got := len(hostile.EffectList().All()); got != 0 {
		t.Fatalf("NPC effects after denied-damage caster = %d, want 0", got)
	}
}

func bootOneShotDebuff(t *testing.T, skillID modelskill.ID) (*gameservertest.Server, *testsupport.ScriptedClient, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{
				ID: skillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
				CastRange: 900, HitTime: 0, StaticHitTime: true,
				SkillType: "DEBUFF", EffectType: "DEBUFF", Debuff: true,
				BaseLandRate: 100, IgnoreResists: true,
				Effects: []modelskill.EffectTemplate{{Name: "Debuff", Time: 60}},
			},
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, int(skillID), 1)
	startInWorld(t, c)
	return srv, c, objID
}

func denyCasterDamage(t *testing.T, srv *gameservertest.Server, objID int32) {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	denied, ok := obj.(interface{ SetCanGiveDamage(bool) })
	if !ok {
		t.Fatalf("world.Player(%d) = %T has no SetCanGiveDamage", objID, obj)
	}
	denied.SetCanGiveDamage(false)
}

func drainAssertingNoAttackFailed(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	for i := 0; i < 100; i++ {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			return
		}
		if frame[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		if id := wireReader(frame[1:]).ReadInt32(); id == int32(serverpackets.SystemMessageAttackFailed) {
			t.Fatal("got AttackFailed; invul/permission refuse is silent")
		}
	}
	t.Fatal("client kept receiving frames after 100 drains")
}

// TestStunBlocksCastingAndMovement lands a stun through a self-cast skill
// and verifies both blanket locks the reference applies to a CC'd caster:
// further casts get ActionFailed only — no reason message — and walk
// requests are released with ActionFailed too.
func TestStunBlocksCastingAndMovement(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{
				ID: 20, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				HitTime: 0, StaticHitTime: true,
				SkillType: "DEBUFF", EffectType: "DEBUFF", Debuff: true,
				BaseLandRate: 100, IgnoreResists: true,
				Effects: []modelskill.EffectTemplate{{Name: "Stun", Time: 30}},
			},
			{
				ID: 21, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				HitTime: 0, StaticHitTime: true, StaticReuse: true, SkillType: "DUMMY",
			},
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 20, 1)
	seedKnownSkill(t, srv, objID, 21, 1)
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(20, false, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillUse, "stun cast ack")
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageUseS1, 20, 1)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillLaunched, "stun launch")
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(21, false, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "cast while stunned")
	drainUntilQuiet(t, c)

	c.Send(encodeMoveBackwardToLocation(200, 200, 30))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "move while stunned")
	drainUntilQuiet(t, c)
}

func slotBuffDef(id modelskill.ID) modelskill.Definition {
	return modelskill.Definition{
		ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		HitTime: 0, StaticHitTime: true, StaticReuse: true, SkillType: "BUFF",
		Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true}},
	}
}

func buffSlotIDs(entries []abnormalStatusEntry) []int32 {
	ids := make([]int32, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.SkillID)
	}
	return ids
}

func castSlotBuff(t *testing.T, c *testsupport.ScriptedClient, objID, skillID int32) []abnormalStatusEntry {
	t.Helper()
	c.Send(encodeRequestMagicSkillUse(skillID, false, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillUse, "MagicSkillUse")
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageUseS1, skillID, 1)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillLaunched, "MagicSkillLaunched")
	return drainCollectingAbnormal(t, c)
}

func drainCollectingAbnormal(t *testing.T, c *testsupport.ScriptedClient) []abnormalStatusEntry {
	t.Helper()
	var last []abnormalStatusEntry
	for i := 0; i < 100; i++ {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			return last
		}
		if frame[0] == serverpackets.OpcodeAbnormalStatusUpdate {
			last = readAbnormalStatusUpdateEntriesFromFrame(t, frame)
		}
	}
	t.Fatal("client kept receiving frames after 100 drains")
	return last
}

func liveMaxBuffCount(t *testing.T, srv *gameservertest.Server, objID int32) int {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	owner, ok := obj.(interface{ MaxBuffCount() int })
	if !ok {
		t.Fatalf("world.Player(%d) = %T has no MaxBuffCount", objID, obj)
	}
	return owner.MaxBuffCount()
}

// TestBuffSlotCapUsesMaxBuffsAmount pins that the players.properties
// MaxBuffsAmount value is the live buff-slot cap when Divine Inspiration
// is unknown: a third slot-family buff evicts the oldest at cap 2.
func TestBuffSlotCapUsesMaxBuffsAmount(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithMaxBuffsAmount(2),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			slotBuffDef(101), slotBuffDef(102), slotBuffDef(103),
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 101, 1)
	seedKnownSkill(t, srv, objID, 102, 1)
	seedKnownSkill(t, srv, objID, 103, 1)
	startInWorld(t, c)

	if got := liveMaxBuffCount(t, srv, objID); got != 2 {
		t.Fatalf("MaxBuffCount() = %d, want 2", got)
	}

	if icons := buffSlotIDs(castSlotBuff(t, c, objID, 101)); !slices.Equal(icons, []int32{101}) {
		t.Fatalf("icons after first buff = %v, want [101]", icons)
	}
	if icons := buffSlotIDs(castSlotBuff(t, c, objID, 102)); !slices.Equal(icons, []int32{101, 102}) {
		t.Fatalf("icons after second buff = %v, want [101 102]", icons)
	}
	if icons := buffSlotIDs(castSlotBuff(t, c, objID, 103)); !slices.Equal(icons, []int32{102, 103}) {
		t.Fatalf("icons after third buff at cap 2 = %v, want oldest evicted [102 103]", icons)
	}
}

// TestDivineInspirationAddsBuffSlots pins that known Divine Inspiration
// (skill 1405) raises the live cap by its skill level: at MaxBuffsAmount 2
// plus level 1, three slot-family buffs fit and the fourth evicts the oldest.
func TestDivineInspirationAddsBuffSlots(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithMaxBuffsAmount(2),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			slotBuffDef(101), slotBuffDef(102), slotBuffDef(103), slotBuffDef(104),
			{
				ID: modelskill.DivineInspirationSkillID, Level: 1,
				Activation: modelskill.ActivationPassive, SkillType: "COREDONE",
			},
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, int(modelskill.DivineInspirationSkillID), 1)
	seedKnownSkill(t, srv, objID, 101, 1)
	seedKnownSkill(t, srv, objID, 102, 1)
	seedKnownSkill(t, srv, objID, 103, 1)
	seedKnownSkill(t, srv, objID, 104, 1)
	startInWorld(t, c)

	if got := liveMaxBuffCount(t, srv, objID); got != 3 {
		t.Fatalf("MaxBuffCount() with Divine Inspiration 1 = %d, want 3", got)
	}

	if icons := buffSlotIDs(castSlotBuff(t, c, objID, 101)); !slices.Equal(icons, []int32{101}) {
		t.Fatalf("icons after first buff = %v, want [101]", icons)
	}
	if icons := buffSlotIDs(castSlotBuff(t, c, objID, 102)); !slices.Equal(icons, []int32{101, 102}) {
		t.Fatalf("icons after second buff = %v, want [101 102]", icons)
	}
	if icons := buffSlotIDs(castSlotBuff(t, c, objID, 103)); !slices.Equal(icons, []int32{101, 102, 103}) {
		t.Fatalf("icons after third buff with extra slot = %v, want [101 102 103]", icons)
	}
	if icons := buffSlotIDs(castSlotBuff(t, c, objID, 104)); !slices.Equal(icons, []int32{102, 103, 104}) {
		t.Fatalf("icons after fourth buff at cap 3 = %v, want oldest evicted [102 103 104]", icons)
	}
}

func stackedBuffDef(id modelskill.ID, order float64, duration int) modelskill.Definition {
	d := slotBuffDef(id)
	d.Effects = []modelskill.EffectTemplate{{Name: "Buff", Time: duration, Icon: true, StackType: "speed_up", StackOrder: order}}
	return d
}

func liveHeldSkillIDs(t *testing.T, srv *gameservertest.Server, objID int32) []int32 {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	holder, ok := obj.(interface{ EffectList() *effect.List })
	if !ok {
		t.Fatalf("world.Player(%d) = %T has no EffectList", objID, obj)
	}
	var ids []int32
	for _, e := range holder.EffectList().All() {
		if e != nil {
			ids = append(ids, int32(e.Skill.ID))
		}
	}
	return ids
}

// TestStackedStrongerBuffCancelsLesserByDefault pins the shipped
// CancelLesserEffect=True behavior: a stronger same-stack buff removes the
// weaker one from the held list, so only the stronger icon remains.
func TestStackedStrongerBuffCancelsLesserByDefault(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			stackedBuffDef(201, 1, 30), stackedBuffDef(202, 2, 30),
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 201, 1)
	seedKnownSkill(t, srv, objID, 202, 1)
	startInWorld(t, c)

	if icons := buffSlotIDs(castSlotBuff(t, c, objID, 201)); !slices.Equal(icons, []int32{201}) {
		t.Fatalf("icons after weaker buff = %v, want [201]", icons)
	}
	if icons := buffSlotIDs(castSlotBuff(t, c, objID, 202)); !slices.Equal(icons, []int32{202}) {
		t.Fatalf("icons after stronger stacked buff = %v, want lesser canceled [202]", icons)
	}
	if ids := liveHeldSkillIDs(t, srv, objID); !slices.Equal(ids, []int32{202}) {
		t.Fatalf("held effects after stronger stacked buff = %v, want [202]", ids)
	}
}

// TestStackedLesserSurvivesWhenCancelLesserDisabled pins CancelLesserEffect=False:
// the weaker same-stack buff stays queued, so when the stronger expires the
// weaker icon returns.
func TestStackedLesserSurvivesWhenCancelLesserDisabled(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCancelLesserEffect(false),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			stackedBuffDef(201, 1, 30), stackedBuffDef(202, 2, 2),
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 201, 1)
	seedKnownSkill(t, srv, objID, 202, 1)
	startInWorld(t, c)

	if icons := buffSlotIDs(castSlotBuff(t, c, objID, 201)); !slices.Equal(icons, []int32{201}) {
		t.Fatalf("icons after weaker buff = %v, want [201]", icons)
	}
	if icons := buffSlotIDs(castSlotBuff(t, c, objID, 202)); !slices.Equal(icons, []int32{202}) {
		t.Fatalf("icons after stronger stacked buff = %v, want only active [202]", icons)
	}
	if ids := liveHeldSkillIDs(t, srv, objID); !slices.Equal(ids, []int32{201, 202}) {
		t.Fatalf("held effects with cancel-lesser off = %v, want queued lesser [201 202]", ids)
	}

	time.Sleep(2200 * time.Millisecond)
	srv.TickEffects()
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageS1HasWornOff, 202, 1)
	if entries := readAbnormalStatusUpdateEntries(t, c); !slices.Equal(buffSlotIDs(entries), []int32{201}) {
		t.Fatalf("icons after stronger expiry = %v, want lesser restored [201]", buffSlotIDs(entries))
	}
}
