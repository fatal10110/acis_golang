package player

import (
	"reflect"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// ---- from character_charges_test.go ----
func TestCharacterIncreaseChargesClampsToMax(t *testing.T) {
	c := chargingCharacter(t)

	if ok := c.IncreaseCharges(2, 5); !ok || c.Charges() != 2 {
		t.Fatalf("after +2: Charges() = %d, ok = %v, want 2, true", c.Charges(), ok)
	}
	if ok := c.IncreaseCharges(2, 5); !ok || c.Charges() != 4 {
		t.Fatalf("after +2: Charges() = %d, ok = %v, want 4, true", c.Charges(), ok)
	}
	if ok := c.IncreaseCharges(3, 5); !ok || c.Charges() != 5 {
		t.Fatalf("after +3 clamped: Charges() = %d, ok = %v, want 5, true", c.Charges(), ok)
	}
	if ok := c.IncreaseCharges(1, 5); ok || c.Charges() != 5 {
		t.Fatalf("at max: Charges() = %d, ok = %v, want 5, false", c.Charges(), ok)
	}
}

func TestCharacterIncreaseChargesNotifiesStatusOnlyAfterSuccessfulAdd(t *testing.T) {
	c := chargingCharacter(t)
	rec := recordEvents(c)

	if !c.IncreaseCharges(5, 5) {
		t.Fatal("IncreaseCharges() = false, want true")
	}
	if updates := event.Count[event.ChargesChanged](rec); updates != 1 {
		t.Fatalf("updates after clamped add = %d, want 1", updates)
	}
	if c.IncreaseCharges(1, 5) {
		t.Fatal("IncreaseCharges() = true at max, want false")
	}
	if updates := event.Count[event.ChargesChanged](rec); updates != 1 {
		t.Fatalf("updates after at-max no-op = %d, want 1", updates)
	}
}

func TestCharacterIncreaseChargesNotifiesForceMessageBeforeStatus(t *testing.T) {
	c := chargingCharacter(t)
	rec := recordEvents(c)

	c.IncreaseCharges(2, 5)
	c.IncreaseCharges(3, 5)
	c.IncreaseCharges(1, 5)

	events := rec.Events()
	want := []event.Event{
		event.ChargeMessage{Charges: 2},
		event.ChargesChanged{},
		event.ChargeMessage{Charges: 5, Maxed: true},
		event.ChargesChanged{},
		event.ChargeMessage{Charges: 5, Maxed: true},
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("charge notifications = %v, want %v", events, want)
	}
}

func TestCharacterDecreaseChargesReportsInsufficientCharges(t *testing.T) {
	c := chargingCharacter(t)
	c.IncreaseCharges(2, 5)

	if ok := c.DecreaseCharges(3); ok || c.Charges() != 2 {
		t.Fatalf("DecreaseCharges(3) over available = ok %v, Charges() %d, want false, 2", ok, c.Charges())
	}
	if ok := c.DecreaseCharges(2); !ok || c.Charges() != 0 {
		t.Fatalf("DecreaseCharges(2) = ok %v, Charges() %d, want true, 0", ok, c.Charges())
	}
}

func TestCharacterDecreaseChargesNotifiesStatusOnlyAfterSuccessfulRemoval(t *testing.T) {
	c := chargingCharacter(t)
	c.IncreaseCharges(2, 5)
	rec := recordEvents(c)

	if !c.DecreaseCharges(1) {
		t.Fatal("DecreaseCharges() = false, want true")
	}
	if updates := event.Count[event.ChargesChanged](rec); updates != 1 {
		t.Fatalf("updates after successful removal = %d, want 1", updates)
	}
	if c.DecreaseCharges(2) {
		t.Fatal("DecreaseCharges() = true with insufficient charges, want false")
	}
	if updates := event.Count[event.ChargesChanged](rec); updates != 1 {
		t.Fatalf("updates after failed removal = %d, want 1", updates)
	}
}

func TestCharacterClearChargesResetsToZero(t *testing.T) {
	c := chargingCharacter(t)
	c.IncreaseCharges(4, 5)

	c.ClearCharges()

	if got := c.Charges(); got != 0 {
		t.Fatalf("Charges() after ClearCharges = %d, want 0", got)
	}
}

func TestCharacterClearChargesNotifiesStatusOnlyWhenChargesChange(t *testing.T) {
	c := chargingCharacter(t)
	c.IncreaseCharges(4, 5)
	rec := recordEvents(c)

	c.ClearCharges()
	if updates := event.Count[event.ChargesChanged](rec); updates != 1 {
		t.Fatalf("updates after clearing charges = %d, want 1", updates)
	}
	c.ClearCharges()
	if updates := event.Count[event.ChargesChanged](rec); updates != 1 {
		t.Fatalf("updates after clearing empty charges = %d, want 1", updates)
	}
}

func TestCharacterDieClearsCharges(t *testing.T) {
	c := attachIdleLive(t, liveCharacter(1, combatTemplate(), combatItems()))
	c.SetHP(1)
	c.IncreaseCharges(3, 5)

	c.Die(nil)

	if got := c.Charges(); got != 0 {
		t.Fatalf("Charges() after Die = %d, want 0", got)
	}
}

// ---- from character_dot_test.go ----
var (
	_ interface {
		Dead() bool
		HP() float64
		ReduceHPByDOT(float64, effect.Actor, bool)
	} = (*Character)(nil)
	_ interface {
		Dead() bool
		MPValue() float64
		ReduceMP(float64) float64
	} = (*Character)(nil)
)

func TestDamageOverTimeEffectTargetsCharacterAndBroadcastsStatus(t *testing.T) {
	c, err := NewCharacter(1, humanFighterTemplate(), "acct", "dot", 0, 0, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() error: %v", err)
	}
	rec := recordEvents(c)
	before := c.HP()

	e, err := effect.New(effect.Skill{ID: 1}, modelskill.EffectTemplate{Name: "DamOverTime", Value: 4})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	e.Effected = c
	if !e.ActionTime() {
		t.Fatal("ActionTime() = false, want true")
	}
	if got, want := c.HP(), before-4; got != want {
		t.Fatalf("HP() = %v, want %v", got, want)
	}
	if statusUpdates := countVitals(rec); statusUpdates != 1 {
		t.Fatalf("status updates = %d, want 1", statusUpdates)
	}
}

// TestManaDamageOverTimeEffectTargetsCharacter pins Finding 2 of the #1088
// closed-PR review: a mana-DOT tick must report a status change
// (EffectManaDamOverTime.java:35 -> CreatureStatus.reduceMp/setMp,
// CreatureStatus.java:338-355, 274-306 -> the Player override at
// PlayerStatus.java:408-416, which sends CUR_HP+CUR_MP+CUR_CP+MAX_CP on
// every call).
func TestManaDamageOverTimeEffectTargetsCharacter(t *testing.T) {
	c, err := NewCharacter(1, humanFighterTemplate(), "acct", "dot", 0, 0, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() error: %v", err)
	}
	rec := recordEvents(c)
	before := c.MPValue()
	e, err := effect.New(effect.Skill{ID: 1}, modelskill.EffectTemplate{Name: "ManaDamOverTime", Value: 4})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	e.Effected = c
	if !e.ActionTime() {
		t.Fatal("ActionTime() = false, want true")
	}
	if got, want := c.MPValue(), before-4; got != want {
		t.Fatalf("MPValue() = %v, want %v", got, want)
	}
	if mpStatusUpdates := countVitals(rec); mpStatusUpdates != 1 {
		t.Fatalf("MP status updates = %d, want 1", mpStatusUpdates)
	}
}

// ---- from character_dotnotice_test.go ----
func TestNotifyEffectRemovedDueLackHPAndMP(t *testing.T) {
	c, err := NewCharacter(1, humanFighterTemplate(), "acct", "dot", 0, 0, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() error: %v", err)
	}

	// No sink attached must not panic.
	c.NotifyEffectRemovedDueLackHP(nil)
	c.NotifyEffectRemovedDueLackMP(nil)

	rec := recordEvents(c)
	c.NotifyEffectRemovedDueLackHP(nil)
	c.NotifyEffectRemovedDueLackMP(nil)
	c.NotifyRelaxDeactivatedHPFull(nil)
	if hpNotices := event.Count[event.EffectRemovedLackHP](rec); hpNotices != 1 {
		t.Fatalf("hp notices = %d, want 1", hpNotices)
	}
	if mpNotices := event.Count[event.EffectRemovedLackMP](rec); mpNotices != 1 {
		t.Fatalf("mp notices = %d, want 1", mpNotices)
	}
	if relaxNotices := event.Count[event.RelaxHPFull](rec); relaxNotices != 1 {
		t.Fatalf("relax notices = %d, want 1", relaxNotices)
	}
}

func TestNotifyHealRestoredHooks(t *testing.T) {
	c, err := NewCharacter(1, humanFighterTemplate(), "acct", "heal", 0, 0, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() error: %v", err)
	}

	rec := recordEvents(c)
	c.NotifyHPRestored("Healer", 4, true)
	c.NotifyMPRestored("", 8, false)
	want := []event.Event{
		event.Restored{Resource: event.ResourceHP, HealerName: "Healer", Amount: 4, ByOther: true},
		event.Restored{Resource: event.ResourceMP, Amount: 8},
	}
	if got := rec.Events(); !reflect.DeepEqual(got, want) {
		t.Fatalf("restored notices = %+v, want %+v", got, want)
	}
}

func TestNotifyMagicFailureHooks(t *testing.T) {
	c, err := NewCharacter(1, humanFighterTemplate(), "acct", "mage", 0, 0, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() error: %v", err)
	}

	// No sink attached must not panic.
	c.NotifyAttackFailed()

	rec := recordEvents(c)
	c.NotifyAttackFailed()
	c.NotifyResistedSkill("Victim", 1419, 1)
	c.NotifyResistedMagic("Mage")
	want := []event.Event{
		event.AttackFailed{},
		event.SkillResisted{TargetName: "Victim", SkillID: 1419, Level: 1},
		event.MagicResisted{AttackerName: "Mage"},
	}
	if got := rec.Events(); !reflect.DeepEqual(got, want) {
		t.Fatalf("magic failure notices = %+v, want %+v", got, want)
	}
}

func TestCharacterSeedPowerReadsActiveSeedEffectLevel(t *testing.T) {
	c := withEffectList(t, liveCharacter(1, combatTemplate(), combatItems()))

	if got := c.SeedPower(1285); got != 0 {
		t.Fatalf("SeedPower(1285) uncharged = %d, want 0", got)
	}

	c.EffectList().Add(&effect.Effect{Skill: effect.Skill{ID: 1285}, Level: 4, Type: effect.TypeBuff})

	if got := c.SeedPower(1285); got != 4 {
		t.Fatalf("SeedPower(1285) charged = %d, want 4", got)
	}
}

func TestCharacterForceLevelReadsActiveForceEffectLevel(t *testing.T) {
	c := withEffectList(t, liveCharacter(1, combatTemplate(), combatItems()))

	if level, ok := c.ForceLevel(5104); ok || level != 0 {
		t.Fatalf("ForceLevel(5104) inactive = (%d, %v), want (0, false)", level, ok)
	}

	c.EffectList().Add(&effect.Effect{Skill: effect.Skill{ID: 5104}, Level: 2, Type: effect.TypeBuff})

	if level, ok := c.ForceLevel(5104); !ok || level != 2 {
		t.Fatalf("ForceLevel(5104) active = (%d, %v), want (2, true)", level, ok)
	}
}

// ---- from character_herb_test.go ----
// TestConsumeHerbReportsWhetherAConsumerTookIt pins the result a herb
// deliverer needs: a detached character consumes nothing, and saying so lets
// the caller deliver the herb another way instead of discarding it.
func TestConsumeHerbReportsWhetherAConsumerTookIt(t *testing.T) {
	c := &Character{ID: 1}

	if c.ConsumeHerb(8600) {
		t.Fatal("ConsumeHerb() = true with no sink attached")
	}

	rec := recordEvents(c)
	if !c.ConsumeHerb(8600) {
		t.Fatal("ConsumeHerb() = false with a sink attached")
	}
	if got := event.Of[event.HerbConsumed](rec); len(got) != 1 || got[0].ItemID != 8600 {
		t.Fatalf("consumed = %v, want [8600]", got)
	}

	c.DetachSession()
	if c.ConsumeHerb(8600) {
		t.Fatal("ConsumeHerb() = true after detach")
	}
	if got := event.Count[event.HerbConsumed](rec); got != 1 {
		t.Fatalf("consumed = %d, want no further consumption after detach", got)
	}
}

type rewardInventoryDelivery struct{ calls int }

func (d *rewardInventoryDelivery) QueueInventoryUpdate(*itemcontainer.Inventory) { d.calls++ }

func (*rewardInventoryDelivery) UpdateInventoryWeight(*itemcontainer.Inventory) {}

// TestAddRewardItemNotifiesTheUpdateHook pins the delivery half of an
// auto-looted kill reward: the mutation methods stay silent because they also
// serve client requests, so this server-driven caller is the one that has to
// register the inventory with the batching task.
func TestAddRewardItemNotifiesTheUpdateHook(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 57, Name: "adena", Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
	})
	c := &Character{ID: 1}
	delivery := &rewardInventoryDelivery{}
	inv := itemcontainer.RestorePlayerInventoryWithDelivery(c.ID, templates, nil, delivery, nil)
	c.AttachRuntime(&Template{}, inv)
	rec := recordEvents(c)

	if !c.AddRewardItem(57, 10, 0x30000001) {
		t.Fatal("AddRewardItem() = false for a known stackable template")
	}
	if delivery.calls != 1 {
		t.Fatalf("update deliveries = %d, want 1", delivery.calls)
	}
	want := []event.ItemObtained{{ItemID: 57, Count: 10, Notice: event.ObtainAdena}}
	if got := event.Of[event.ItemObtained](rec); !slices.Equal(got, want) {
		t.Fatalf("ItemObtained events = %+v, want %+v", got, want)
	}

	if c.AddRewardItem(9999, 1, 0x30000002) {
		t.Fatal("AddRewardItem() = true for an unknown template")
	}
	if delivery.calls != 1 {
		t.Fatalf("update deliveries after a rejected add = %d, want 1", delivery.calls)
	}
	if got := event.Count[event.ItemObtained](rec); got != 1 {
		t.Fatalf("ItemObtained events after a rejected add = %d, want 1", got)
	}
}

// chargingCharacter is an in-world character whose charge auto-clear timer
// arms on a queue nothing advances.
func chargingCharacter(t *testing.T) *Character {
	t.Helper()
	return attachIdleLive(t, &Character{ID: 1})
}
