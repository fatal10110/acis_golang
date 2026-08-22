package npc

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

var _ skilltarget.AttackRules = (*Hostile)(nil)

var _ skilltarget.SightChecker = (*Hostile)(nil)

// zeroRoll always returns 0, pinning MakeAttackHit's hit/crit/damage-spread
// rolls to a deterministic outcome: with any positive hit rate and
// critical rate, a roll of 0 always hits and always crits.
func zeroRoll(int) int { return 0 }

func newCombatHostile(t testing.TB, id int32, tpl *Template) *Hostile {
	t.Helper()
	h, err := NewHostile(&Instance{ObjectID: id, Template: tpl, Kind: "Monster"}, newHostileLive(t), &hostileMove{}, &hostileAttack{})
	if err != nil {
		t.Fatal(err)
	}
	h.SetFrameBuilder(serverpackets.NpcFrameBuilder{})
	return h
}

func TestHostileMakeAttackHitResolvesDamage(t *testing.T) {
	attacker := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", PAtk: 100, STR: 40, DEX: 30, Level: 1, CritRate: 4})
	attacker.SetRollSource(zeroRoll)
	defender := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster", PDef: 50, STR: 40, DEX: 30, Level: 1, HPMax: 1000})

	hit := attacker.MakeAttackHit(defender, false)

	// Attacker/defender both finalize through the stat calculator: at
	// STR=40 (the reference base-attribute default), STRBonus[40]=1.2 and
	// level 1's LevelMod=0.9, so attacker.PAtk finalizes to
	// 100*1.2*0.9=108 and defender.PDef to 50*0.9=45. An even accuracy/
	// evasion match (same DEX/level on both sides) and a guaranteed
	// critical hit (zeroRoll) then resolve through the physical-attack
	// formula (already verified against the reference implementation).
	// The template sets no BaseRandomDamage, so RandomDamageMultiplier
	// falls back to the weaponless spread `5+sqrt(level)`=6 (level 1);
	// zeroRoll always returns 0, giving randomMul=1+(0-6)/100=0.94:
	//   (108*2 * 1(posMul) * 0.94(randomMul) + 0) * 77/45 = 347 (truncated)
	const wantDamage = 347
	if hit.Miss || hit.Damage != wantDamage {
		t.Fatalf("MakeAttackHit() = %+v, want %d damage", hit, wantDamage)
	}
	if got := defender.CurrentHP(); got != defender.MaxHP() {
		t.Fatalf("defender HP = %d, want unchanged %d", got, defender.MaxHP())
	}
}

// TestHostileMakeAttackHitTruncatesCriticalRateBeforeCap pins the boundary
// from CreatureStatus.java:551-553 (`Math.min((int) calcStat(...), 500)`):
// the finalized critical rate is truncated to an int before the 500 cap and
// before the roll comparison in Formulas.java:705-708. At DEX=26 and
// CritRate=8, AtkCritical finalizes to 8*DEXBonus[26]*10=84.8; truncating
// first yields the int 84, which a roll of exactly 84 must NOT beat
// (CritSucceeds requires rate strictly greater than roll). Without
// truncation the fractional 84.8 would still beat a roll of 84, so this
// pins the previously-missing cast.
func TestHostileMakeAttackHitTruncatesCriticalRateBeforeCap(t *testing.T) {
	attacker := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", PAtk: 100, STR: 40, DEX: 26, Level: 1, CritRate: 8})
	attacker.SetRollSource(func(int) int { return 84 })
	defender := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster", PDef: 50, STR: 40, DEX: 26, Level: 1, HPMax: 1000})

	hit := attacker.MakeAttackHit(defender, false)

	if hit.Miss {
		t.Fatal("MakeAttackHit().Miss = true, want a hit (matched accuracy/evasion beats roll 84)")
	}
	if hit.Crit {
		t.Fatal("MakeAttackHit().Crit = true, want false: truncated critical rate 84 must not beat roll 84")
	}
}

func TestHostileRechargeShotsUsesTemplateCountersAndBroadcasts(t *testing.T) {
	ai := commons.NewStatSet()
	ai.Set("SoulShot", 1)
	ai.Set("SpiritShot", 1)
	hostile := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", AIParams: ai})
	state := world.New()
	hostile.SetWorld(state)
	state.Spawn(hostile, 0, 0, 0, 0)

	near := &frameReceiver{trackedID: 2}
	far := &frameReceiver{trackedID: 3}
	state.Spawn(near, 600, 0, 0, 0)
	state.Spawn(far, 601, 0, 0, 0)

	hostile.RechargeShots(true, true)
	if !hostile.SoulshotCharged() || !hostile.SpiritshotCharged() {
		t.Fatalf("charged state = soul %v spirit %v, want both true", hostile.SoulshotCharged(), hostile.SpiritshotCharged())
	}
	if got := hostile.CurrentSoulshotCount(); got != 0 {
		t.Fatalf("CurrentSoulshotCount() = %d, want 0", got)
	}
	if got := hostile.CurrentSpiritshotCount(); got != 0 {
		t.Fatalf("CurrentSpiritshotCount() = %d, want 0", got)
	}
	if len(near.frames) != 2 || len(far.frames) != 0 {
		t.Fatalf("recharge frames near/far = %d/%d, want 2/0", len(near.frames), len(far.frames))
	}
	target := newCombatHostile(t, 4, &Template{ID: 4, Type: "Monster", PDef: 1, MDef: 1})
	physical, ok := target.PhysicalSkillInput(hostile, modelskill.Definition{Power: 1})
	if !ok || !physical.SoulShot {
		t.Fatalf("PhysicalSkillInput soulshot = %v, %v; want true, true", physical.SoulShot, ok)
	}
	magic, ok := target.MagicDamageInput(hostile, modelskill.Definition{Power: 1})
	if !ok || !magic.SoulShot || magic.BlessedSoulShot {
		t.Fatalf("MagicDamageInput spirit flags = %v/%v, ok %v; want true/false, true", magic.SoulShot, magic.BlessedSoulShot, ok)
	}

	hostile.RechargeShots(true, true)
	if len(near.frames) != 2 {
		t.Fatalf("duplicate recharge frames = %d, want 2", len(near.frames))
	}
}

// TestHostileStatFuncsBroadcastRunSpeedInfo guards #1597: adding or removing
// an NPC-owned run-speed modifier must refresh every nearby observer's NPCInfo.
func TestHostileStatFuncsBroadcastRunSpeedInfo(t *testing.T) {
	hostile := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", DEX: 30, RunSpeed: 100})
	state := world.New()
	hostile.SetWorld(state)
	state.Spawn(hostile, 0, 0, 0, 0)

	observer := &frameReceiver{trackedID: 2}
	state.Spawn(observer, 600, 0, 0, 0)

	owner := effect.ModOwnerEffect(&effect.Effect{})
	hostile.AddStatFuncs([]effect.Mod{{Stat: stat.RunSpeed, Op: effect.OpAdd, Value: 10, Owner: owner}})
	if len(observer.frames) != 1 {
		t.Fatalf("frames after run-speed buff = %d, want 1 NPCInfo", len(observer.frames))
	}
	if got := observer.frames[0][0]; got != serverpackets.OpcodeNPCInfo {
		t.Fatalf("buff frame opcode = %#x, want NPCInfo %#x", got, serverpackets.OpcodeNPCInfo)
	}
	if got := binary.LittleEndian.Uint32(observer.frames[0][41:45]); got != 120 {
		t.Fatalf("NPCInfo run speed after buff = %d, want 120", got)
	}

	hostile.RemoveStatsByOwner(owner)
	if len(observer.frames) != 2 {
		t.Fatalf("frames after run-speed removal = %d, want 2 NPCInfo frames", len(observer.frames))
	}
	if got := binary.LittleEndian.Uint32(observer.frames[1][41:45]); got != 110 {
		t.Fatalf("NPCInfo run speed after removal = %d, want 110", got)
	}
}

// TestHostileStatFuncsBroadcastMaxHPStatus guards #1597: a max-HP modifier
// sends exactly the changed StatusUpdate attribute, not a stale HP pair.
func TestHostileStatFuncsBroadcastMaxHPStatus(t *testing.T) {
	hostile := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", CON: 20, HPMax: 100})
	state := world.New()
	hostile.SetWorld(state)
	state.Spawn(hostile, 0, 0, 0, 0)

	observer := &frameReceiver{trackedID: 2}
	state.Spawn(observer, 600, 0, 0, 0)

	hostile.AddStatFuncs([]effect.Mod{{Stat: stat.MaxHP, Op: effect.OpMul, Value: 2}})
	if len(observer.frames) != 1 {
		t.Fatalf("frames after max-HP buff = %d, want 1 StatusUpdate", len(observer.frames))
	}
	frame := observer.frames[0]
	if got := frame[0]; got != serverpackets.OpcodeStatusUpdate {
		t.Fatalf("buff frame opcode = %#x, want StatusUpdate %#x", got, serverpackets.OpcodeStatusUpdate)
	}
	if got := binary.LittleEndian.Uint32(frame[5:9]); got != 1 {
		t.Fatalf("StatusUpdate attributes = %d, want 1", got)
	}
	if got := binary.LittleEndian.Uint32(frame[9:13]); got != uint32(serverpackets.StatusMaxHP) {
		t.Fatalf("StatusUpdate attribute type = %d, want max HP %d", got, serverpackets.StatusMaxHP)
	}
	if got := binary.LittleEndian.Uint32(frame[13:17]); got != 160 {
		t.Fatalf("StatusUpdate max HP = %d, want 160", got)
	}
}

func TestHostileStatFuncsBroadcastAttackSpeedStatus(t *testing.T) {
	hostile := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", DEX: 30, WIT: 43, AtkSpd: 100})
	state := world.New()
	hostile.SetWorld(state)
	state.Spawn(hostile, 0, 0, 0, 0)

	observer := &frameReceiver{trackedID: 2}
	state.Spawn(observer, 600, 0, 0, 0)

	hostile.AddStatFuncs([]effect.Mod{
		{Stat: stat.PowerAttackSpeed, Op: effect.OpAdd, Value: 10},
		{Stat: stat.MagicAttackSpeed, Op: effect.OpAdd, Value: 10},
	})
	if len(observer.frames) != 1 {
		t.Fatalf("frames after attack-speed buffs = %d, want 1 StatusUpdate", len(observer.frames))
	}
	frame := observer.frames[0]
	if got := binary.LittleEndian.Uint32(frame[5:9]); got != 2 {
		t.Fatalf("StatusUpdate attributes = %d, want 2", got)
	}
	if got := binary.LittleEndian.Uint32(frame[9:13]); got != 18 {
		t.Fatalf("first StatusUpdate attribute type = %d, want attack speed 18", got)
	}
	if got := binary.LittleEndian.Uint32(frame[17:21]); got != 24 {
		t.Fatalf("second StatusUpdate attribute type = %d, want cast speed 24", got)
	}
}

// TestHostileStatFuncsBroadcastZeroRunSpeedObjectInfo guards #1597's
// stationary-NPC branch: a zero move speed uses ServerObjectInfo, not NPCInfo.
func TestHostileStatFuncsBroadcastZeroRunSpeedObjectInfo(t *testing.T) {
	hostile := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", DEX: 30, RunSpeed: 100})
	state := world.New()
	hostile.SetWorld(state)
	state.Spawn(hostile, 0, 0, 0, 0)

	observer := &frameReceiver{trackedID: 2}
	state.Spawn(observer, 600, 0, 0, 0)

	hostile.AddStatFuncs([]effect.Mod{{Stat: stat.RunSpeed, Op: effect.OpSet, Value: 0}})
	if len(observer.frames) != 1 {
		t.Fatalf("frames after zero-speed buff = %d, want 1 ServerObjectInfo", len(observer.frames))
	}
	if got := observer.frames[0][0]; got != 0x8c {
		t.Fatalf("zero-speed frame opcode = %#x, want ServerObjectInfo %#x", got, 0x8c)
	}
}

// TestHostileTakeDamageRollsAttackedShotRecharge ports MonsterBehavior/
// WarriorBase/WizardBase.onAttacked (aCis Java generic monster AI): a
// landed hit rolls the defender's SoulShotRate/SpiritShotRate AI parameters
// and recharges the matching shot type on success. A forced roll of 0
// always beats a 100% rate and never beats a 0% rate, so this pins the
// trigger deterministically instead of relying on real RNG.
func TestHostileTakeDamageRollsAttackedShotRecharge(t *testing.T) {
	ai := commons.NewStatSet()
	ai.Set("SoulShot", 1)
	ai.Set("SoulShotRate", 100)
	ai.Set("SpiritShot", 1)
	ai.Set("SpiritShotRate", 0)
	defender := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", HPMax: 1000, AIParams: ai})
	defender.SetRollSource(zeroRoll)
	attacker := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster"})

	defender.TakeDamage(10, attacker)

	if !defender.SoulshotCharged() {
		t.Fatal("SoulshotCharged() = false, want true: a 100% SoulShotRate must trigger on a landed hit")
	}
	if defender.SpiritshotCharged() {
		t.Fatal("SpiritshotCharged() = true, want false: a 0% SpiritShotRate must never trigger")
	}
	if got := defender.CurrentSoulshotCount(); got != 0 {
		t.Fatalf("CurrentSoulshotCount() = %d, want 0", got)
	}
}

// TestHostileTakeDamageQueuesFlatAttackDesireWeight guards against the
// ATTACK desire's weight scaling with damage dealt: DefaultNpc.tryToAttack
// always queues a flat 200 weight per hit (accumulated across hits by
// DesireQueue.AddOrUpdate, same as any other addAttackDesire caller), and
// Npc.reduceCurrentHp never derives desire weight from damage at all. A
// 10-damage hit and a 500-damage hit must add the same 200 increment, even
// though the threat table's hate keeps accumulating with the damage dealt.
func TestHostileTakeDamageQueuesFlatAttackDesireWeight(t *testing.T) {
	defender := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", HPMax: 1000})
	attacker := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster"})

	defender.TakeDamage(10, attacker)
	defender.TakeDamage(500, attacker)

	desire, ok := defender.AI().Desires().Peek()
	if !ok {
		t.Fatal("Desires().Peek() ok = false, want a queued attack desire")
	}
	if desire.Weight != 400 {
		t.Fatalf("queued attack desire weight = %v, want 400 (two flat-200 hits, "+
			"independent of the 10 vs. 500 damage dealt)", desire.Weight)
	}
	if got := defender.AI().Threats().Hate(attacker); got != 510 {
		t.Fatalf("threat table hate = %v, want 510 (damage still accumulates there)", got)
	}
}

func TestHostileSetChargedShotDischargesIndependentlyPerKind(t *testing.T) {
	hostile := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster"})

	hostile.SetChargedShot(item.ShotSoul, true)
	hostile.SetChargedShot(item.ShotSpirit, true)
	if !hostile.SoulshotCharged() || !hostile.SpiritshotCharged() {
		t.Fatalf("charged state = soul %v spirit %v, want both true", hostile.SoulshotCharged(), hostile.SpiritshotCharged())
	}

	hostile.SetChargedShot(item.ShotSoul, false)
	if hostile.SoulshotCharged() {
		t.Fatal("SoulshotCharged() = true after SetChargedShot(ShotSoul, false)")
	}
	if !hostile.SpiritshotCharged() {
		t.Fatal("discharging ShotSoul must not discharge ShotSpirit")
	}
}

func TestHostileStatFuncsAdjustFinalizedCombatStats(t *testing.T) {
	tpl := &Template{ID: 1, Type: "Monster", PAtk: 100, PDef: 50, STR: 40, DEX: 30, Level: 1}
	h := newCombatHostile(t, 1, tpl)

	basePAtk := h.calcStat(stat.PowerAttack, tpl.PAtk)
	basePDef := h.PDef()

	owner := effect.ModOwnerEffect(&effect.Effect{})
	h.AddStatFuncs([]effect.Mod{{Stat: stat.PowerAttack, Op: effect.OpMul, Value: 1.5, Owner: owner}})

	if got, want := h.calcStat(stat.PowerAttack, tpl.PAtk), basePAtk*1.5; got != want {
		t.Fatalf("P.Atk after buff = %v, want %v", got, want)
	}
	if got := h.PDef(); got != basePDef {
		t.Fatalf("P.Def changed by an unrelated P.Atk buff: got %v, want unchanged %v", got, basePDef)
	}

	h.RemoveStatsByOwner(owner)

	if got := h.calcStat(stat.PowerAttack, tpl.PAtk); got != basePAtk {
		t.Fatalf("P.Atk after RemoveStatsByOwner = %v, want reverted %v", got, basePAtk)
	}
}

func TestHostileMakeAttackHitMissesUnknownTargetType(t *testing.T) {
	attacker := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", PAtk: 100})
	attacker.SetRollSource(zeroRoll)

	hit := attacker.MakeAttackHit(&hostileTarget{id: 99}, false)
	if !hit.Miss {
		t.Fatal("MakeAttackHit().Miss = false, want true for a target with no physical-damage surface")
	}
}

func TestHostileAttackTypeAndWeaponReuseDelay(t *testing.T) {
	const bowItemID = 500
	const bowReuseMillis = 1500

	bow := &item.Template{
		ID:      bowItemID,
		Kind:    item.KindWeapon,
		Crystal: item.CrystalB,
		Weapon:  &item.WeaponDetail{Type: item.WeaponBow, ReuseDelay: bowReuseMillis},
	}
	notAWeapon := &item.Template{ID: 501, Kind: item.KindEtcItem}
	items := item.NewTable([]*item.Template{bow, notAWeapon})

	tests := []struct {
		name           string
		rightHand      int
		items          *item.Table
		wantAttackType item.WeaponType
		wantReuseDelay time.Duration
		wantGrade      int
	}{
		{
			name:           "no right-hand item id stays unarmed",
			rightHand:      0,
			items:          items,
			wantAttackType: item.WeaponFist,
		},
		{
			name:           "unknown right-hand item id stays unarmed",
			rightHand:      999,
			items:          items,
			wantAttackType: item.WeaponFist,
		},
		{
			name:           "right-hand item that isn't a weapon stays unarmed",
			rightHand:      int(notAWeapon.ID),
			items:          items,
			wantAttackType: item.WeaponFist,
		},
		{
			name:           "nil item table stays unarmed",
			rightHand:      bowItemID,
			items:          nil,
			wantAttackType: item.WeaponFist,
		},
		{
			name:           "right-hand weapon item resolves its type, reuse delay and crystal grade",
			rightHand:      bowItemID,
			items:          items,
			wantAttackType: item.WeaponBow,
			wantReuseDelay: bowReuseMillis * time.Millisecond,
			wantGrade:      int(item.CrystalB),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", RightHand: tc.rightHand})
			h.SetWeapon(tc.items)

			if got := h.AttackType(); got != tc.wantAttackType {
				t.Fatalf("AttackType() = %v, want %v", got, tc.wantAttackType)
			}
			if got := h.WeaponReuseDelay(); got != tc.wantReuseDelay {
				t.Fatalf("WeaponReuseDelay() = %v, want %v", got, tc.wantReuseDelay)
			}
			if got := h.WeaponGrade(); got != tc.wantGrade {
				t.Fatalf("WeaponGrade() = %v, want %v", got, tc.wantGrade)
			}
		})
	}
}

func TestHostileAttackableByLiveAttacker(t *testing.T) {
	target := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster", PDef: 50, DEX: 30, Level: 1, HPMax: 100})
	attacker := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", PAtk: 100, AtkSpd: 300, DEX: 30, Level: 1, HPMax: 100})

	if !target.AttackableBy(attacker) {
		t.Fatal("live hostile target is not attackable")
	}
	if !target.AttackableWithoutForceBy(attacker) {
		t.Fatal("live hostile target is not attackable without force")
	}
	if target.AttackableBy(target) {
		t.Fatal("hostile target is attackable by itself")
	}
	if target.AttackableWithoutForceBy(target) {
		t.Fatal("hostile target is attackable by itself without force")
	}
	target.MarkDead()
	if target.AttackableBy(attacker) {
		t.Fatal("dead hostile target is attackable")
	}
	if target.AttackableWithoutForceBy(attacker) {
		t.Fatal("dead hostile target is attackable without force")
	}
}

func TestHostileTakeDamageReachingZeroTriggersDieAndDecayChain(t *testing.T) {
	attacker := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", PAtk: 100, STR: 40, DEX: 30, Level: 1, CritRate: 4})
	attacker.SetRollSource(zeroRoll)
	defender := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster", PDef: 50, STR: 40, DEX: 30, Level: 1, HPMax: 100, CorpseTime: 7})

	state := world.New()
	state.Spawn(defender, 100, 100, 0, 0)

	if defender.Dead() {
		t.Fatal("defender.Dead() = true before any damage, want false")
	}

	hit := attacker.MakeAttackHit(defender, false) // 369 damage vs 100 HP: lethal in one hit.
	defender.TakeDamage(hit.Damage, attacker)

	if !defender.Dead() {
		t.Fatal("defender.Dead() = false after a lethal hit, want true")
	}

	// The kill itself only latches the dead state (Die, exercised above via
	// TakeDamage); registering the corpse with the decay task is the
	// orchestration layer's job per Hostile.Die's own doc comment. Exercise
	// that same handoff here with the existing corpse-decay task.
	respawned := false
	effects := decayEffectsFunc(func(actor task.DecayActor) {
		h, ok := actor.(*Hostile)
		if !ok {
			t.Fatalf("decay actor = %T, want *Hostile", actor)
		}
		h.Decay(state, func() { respawned = true })
	})
	decay, err := task.NewDecay(effects, func() time.Time { return time.Unix(0, 0) })
	if err != nil {
		t.Fatal(err)
	}
	decay.Add(defender, 0)
	decay.Tick()

	if !defender.Decayed() {
		t.Fatal("defender.Decayed() = false after the decay task fired, want true")
	}
	if !respawned {
		t.Fatal("respawn hook was not called through the decay chain")
	}
	if _, ok := state.Object(defender.ObjectID()); ok {
		t.Fatal("defender is still tracked in the world after decay")
	}
}

type decayEffectsFunc func(task.DecayActor)

func (f decayEffectsFunc) Decay(actor task.DecayActor) { f(actor) }

func TestHostileBroadcastAttackSendsFrameToKnownReceivers(t *testing.T) {
	state := world.New()
	attacker := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster"})
	attacker.SetWorld(state)
	state.Spawn(attacker, 100, 100, 0, 0)

	observer := &frameReceiver{trackedID: 55}
	state.Spawn(observer, 100, 100, 0, 0)

	nonReceiver := &hostileTarget{id: 56}
	state.Spawn(nonReceiver, 100, 100, 0, 0)

	attacker.BroadcastAttack(serverpackets.AttackSnapshot{
		AttackerID: attacker.ObjectID(),
		Hits:       []serverpackets.AttackHit{{TargetID: 2, Damage: 10}},
	})

	if len(observer.frames) != 1 {
		t.Fatalf("observer received %d frames, want 1", len(observer.frames))
	}
	if observer.frames[0][0] != serverpackets.OpcodeAttack {
		t.Fatalf("frame opcode = %#x, want %#x", observer.frames[0][0], serverpackets.OpcodeAttack)
	}
}

func TestHostileBroadcastAttackNoopsWithoutWorld(t *testing.T) {
	attacker := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster"})
	// SetWorld was never called; this must not panic.
	attacker.BroadcastAttack(serverpackets.AttackSnapshot{AttackerID: 1})
}

func TestHostileBroadcastSkillUseSendsFrameToKnownReceivers(t *testing.T) {
	state := world.New()
	caster := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster"})
	caster.SetWorld(state)
	state.Spawn(caster, 100, 100, 0, 0)

	observer := &frameReceiver{trackedID: 55}
	state.Spawn(observer, 100, 100, 0, 0)

	caster.BroadcastSkillUse(2, 200, 100, 0, 4, 1, 1200, 5000)

	if len(observer.frames) != 1 {
		t.Fatalf("observer received %d frames, want 1", len(observer.frames))
	}
	if observer.frames[0][0] != serverpackets.OpcodeMagicSkillUse {
		t.Fatalf("frame opcode = %#x, want %#x", observer.frames[0][0], serverpackets.OpcodeMagicSkillUse)
	}
}

func TestHostileBroadcastSkillUseNoopsWithoutWorld(t *testing.T) {
	caster := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster"})
	// SetWorld was never called; this must not panic.
	caster.BroadcastSkillUse(2, 200, 100, 0, 4, 1, 1200, 5000)
}

func TestHostileBroadcastSkillLaunchedSendsFrameToKnownReceivers(t *testing.T) {
	state := world.New()
	caster := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster"})
	caster.SetWorld(state)
	state.Spawn(caster, 100, 100, 0, 0)

	observer := &frameReceiver{trackedID: 55}
	state.Spawn(observer, 100, 100, 0, 0)

	caster.BroadcastSkillLaunched(4, 1, []int32{2})

	if len(observer.frames) != 1 {
		t.Fatalf("observer received %d frames, want 1", len(observer.frames))
	}
	if observer.frames[0][0] != serverpackets.OpcodeMagicSkillLaunched {
		t.Fatalf("frame opcode = %#x, want %#x", observer.frames[0][0], serverpackets.OpcodeMagicSkillLaunched)
	}
}

func TestHostileBroadcastSkillLaunchedNoopsWithoutWorld(t *testing.T) {
	caster := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster"})
	// SetWorld was never called; this must not panic.
	caster.BroadcastSkillLaunched(4, 1, []int32{2})
}

func TestHostileBroadcastMoveToPawnSendsFrameToKnownReceivers(t *testing.T) {
	state := world.New()
	caster := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster"})
	caster.SetWorld(state)
	state.Spawn(caster, 100, 100, 0, 0)

	observer := &frameReceiver{trackedID: 55}
	state.Spawn(observer, 100, 100, 0, 0)

	target := &hostileTarget{id: 2}
	state.Spawn(target, 200, 100, 0, 0)

	caster.BroadcastMoveToPawn(target)

	if len(observer.frames) != 1 {
		t.Fatalf("observer received %d frames, want 1", len(observer.frames))
	}
	if observer.frames[0][0] != serverpackets.OpcodeMoveToPawn {
		t.Fatalf("frame opcode = %#x, want %#x", observer.frames[0][0], serverpackets.OpcodeMoveToPawn)
	}
}

func TestHostileBroadcastMoveToPawnNoopsWithoutWorld(t *testing.T) {
	caster := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster"})
	// SetWorld was never called; this must not panic.
	caster.BroadcastMoveToPawn(&hostileTarget{id: 2})
}

func TestHostileDieBroadcastsDieToKnownReceivers(t *testing.T) {
	state := world.New()
	victim := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", HPMax: 10})
	victim.SetWorld(state)
	state.Spawn(victim, 100, 100, 0, 0)

	observer := &frameReceiver{trackedID: 55}
	state.Spawn(observer, 100, 100, 0, 0)

	if !victim.Die(&hostileTarget{id: 2}, nil) {
		t.Fatal("Die() = false on a live target, want true")
	}

	if len(observer.frames) != 1 {
		t.Fatalf("observer received %d frames, want 1", len(observer.frames))
	}
	if observer.frames[0][0] != serverpackets.OpcodeDie {
		t.Fatalf("frame opcode = %#x, want %#x", observer.frames[0][0], serverpackets.OpcodeDie)
	}
	if got := binary.LittleEndian.Uint32(observer.frames[0][21:25]); got != 0 {
		t.Fatalf("Die sweep field = %d, want 0", got)
	}

	// A repeated kill is a no-op per Die's once-only contract: no second
	// Die packet.
	if victim.Die(&hostileTarget{id: 2}, nil) {
		t.Fatal("Die() = true on an already-dead target, want false")
	}
	if len(observer.frames) != 1 {
		t.Fatalf("observer received %d frames after a repeat kill, want still 1", len(observer.frames))
	}
}

func TestHostileDieSetsSweepFlagFromSpoilPool(t *testing.T) {
	state := world.New()
	victim := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", HPMax: 10})
	victim.SetWorld(state)
	state.Spawn(victim, 100, 100, 0, 0)

	observer := &frameReceiver{trackedID: 55}
	state.Spawn(observer, 100, 100, 0, 0)
	victim.SpoilPool().Add(57, 1)

	if !victim.Die(&hostileTarget{id: 2}, nil) {
		t.Fatal("Die() = false on a live target, want true")
	}
	if got := binary.LittleEndian.Uint32(observer.frames[0][21:25]); got != 1 {
		t.Fatalf("Die sweep field = %d, want 1", got)
	}
}

// TestHostileBroadcastFailuresAreLoggedWithoutWorld covers #1241: Hostile's
// status/die broadcasts, unlike the AI-driven broadcast paths #1236 wired
// through task.AI.Tick's a.log.Warn(), previously discarded ErrNoWorld
// silently. TakeDamage on a lethal hit exercises both BroadcastStatus (via
// ReduceHP's callers) and BroadcastDie in one call.
func TestHostileBroadcastFailuresAreLoggedWithoutWorld(t *testing.T) {
	victim := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", HPMax: 10})
	buf := &bytes.Buffer{}
	victim.SetLogger(zerolog.New(buf))

	// SetWorld was never called: both broadcasts must fail with
	// ErrNoWorld, and both failures must be logged instead of discarded.
	victim.TakeDamage(10, &hostileTarget{id: 2})

	if !victim.Dead() {
		t.Fatal("victim.Dead() = false after a lethal hit, want true")
	}
	got := buf.String()
	if !strings.Contains(got, "npc: status broadcast") {
		t.Fatalf("log = %q, want it to contain %q", got, "npc: status broadcast")
	}
	if !strings.Contains(got, "npc: die broadcast") {
		t.Fatalf("log = %q, want it to contain %q", got, "npc: die broadcast")
	}
}

type frameReceiver struct {
	world.Presence
	trackedID int32
	frames    [][]byte
}

func (f *frameReceiver) ObjectID() int32 { return f.trackedID }

func (f *frameReceiver) SendFrame(frame wire.Frame) bool {
	defer frame.Release()
	raw := frame.Bytes()
	payload := make([]byte, len(raw)-2)
	copy(payload, raw[2:])
	f.frames = append(f.frames, payload)
	return true
}

func (f *frameReceiver) BroadcastFrame(frame wire.Frame) bool { return f.SendFrame(frame) }

var _ creature.DeathActor = (*Hostile)(nil)

// fakeHostileLineOfSight is a LineOfSight double that records the query it
// received and returns a fixed result.
type fakeHostileLineOfSight struct {
	result bool
	got    struct {
		ox, oy, oz       int
		oCollisionHeight float64
		tx, ty, tz       int
		tCollisionHeight float64
	}
}

func (f *fakeHostileLineOfSight) CanSeeActor(ox, oy, oz int, oCollisionHeight float64, tx, ty, tz int, tCollisionHeight float64) bool {
	f.got.ox, f.got.oy, f.got.oz, f.got.oCollisionHeight = ox, oy, oz, oCollisionHeight
	f.got.tx, f.got.ty, f.got.tz, f.got.tCollisionHeight = tx, ty, tz, tCollisionHeight
	return f.result
}

func TestHostileCanSeeDefaultsToVisibleWithoutLineOfSight(t *testing.T) {
	h := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", CollisionHeight: 30})
	target := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster", CollisionHeight: 30})

	if !h.CanSee(target) {
		t.Fatal("CanSee() = false with no line-of-sight query attached, want true")
	}
}

func TestHostileCanSeeQueriesLineOfSightWithActorHeights(t *testing.T) {
	h := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", CollisionHeight: 30})
	target := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster", CollisionHeight: 40})

	los := &fakeHostileLineOfSight{result: false}
	h.SetLineOfSight(los)

	if got := h.CanSeeTarget(target); got != false {
		t.Fatalf("CanSeeTarget() = %v, want false (from line-of-sight query result)", got)
	}

	ox, oy, oz := h.Position()
	tx, ty, tz := target.Position()
	if los.got.ox != ox || los.got.oy != oy || los.got.oz != oz {
		t.Fatalf("CanSeeActor() origin = (%d,%d,%d), want (%d,%d,%d)", los.got.ox, los.got.oy, los.got.oz, ox, oy, oz)
	}
	if los.got.tx != tx || los.got.ty != ty || los.got.tz != tz {
		t.Fatalf("CanSeeActor() target = (%d,%d,%d), want (%d,%d,%d)", los.got.tx, los.got.ty, los.got.tz, tx, ty, tz)
	}
	if los.got.oCollisionHeight != h.CollisionHeight() {
		t.Fatalf("CanSeeActor() origin collision height = %v, want %v", los.got.oCollisionHeight, h.CollisionHeight())
	}
	if los.got.tCollisionHeight != target.CollisionHeight() {
		t.Fatalf("CanSeeActor() target collision height = %v, want %v", los.got.tCollisionHeight, target.CollisionHeight())
	}
}
