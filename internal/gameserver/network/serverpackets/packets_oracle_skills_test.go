package serverpackets

import (
	"strconv"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/recipe"
)

// oracleDurations is ProbeSkills.DURATIONS: effect times in ms, whole seconds
// truncated toward zero, and the permanent marker -1.
var oracleDurations = []int32{999, 1000, 1001, 15999, -1, -999, -1000, -1999, -2147483648, 2147483647}

// oracleShortcutKinds are ProbeSkills.SHORTCUTS: the shortcut k of a row is
// the kind k names; the first six are item shortcuts.
var oracleShortcutKinds = []string{
	"item-missing", "item-plain", "item-augmented", "item-etc-no-skill", "item-etc-skill", "item-etc-timer",
	"skill", "action", "macro", "recipe", "none",
}

// oracleSkillsClans are the clan states of ProbeSkills.skillListFlags; a clan
// skill greys out only under a negative reputation.
var oracleSkillsClans = []struct {
	name     string
	disabled bool
}{{"none", false}, {"neg", true}, {"zero", false}, {"pos", false}}

// oracleSkillsCases are the skill, effect, reuse, shortcut, attack, door and
// quest packets and the gauge windows (testdata/oracle/ProbeSkills.java).
// What the stand-ins decide before the encoder is handed to each case as the
// value the reference reads: which reuse timers still run, whether a skill is
// greyed out, an item shortcut's reuse and augmentation.
func oracleSkillsCases() map[string]func() wire.Frame {
	m := map[string]func() wire.Frame{}
	for r, row := range oracleRows {
		v, n := row.v, "/"+row.name
		addSkillEffectCases(m, r, v, n)
		addSkillListCases(m, r, v, n)
		addSkillReuseCases(m, r, v, n)
		addSkillTrainerCases(m, r, v, n)
		addShortcutCases(m, r, v, n)
		addAttackCases(m, r, v, n)
		addDoorCases(m, r, v, n)
		addQuestListCases(m, r, v, n)
		addGaugeWindowCases(m, r, v, n)
	}
	addSkillOrderCases(m)
	addDoorResidenceCases(m)
	addVitalsCases(m)
	m["AcquireSkillDone"] = FrameAcquireSkillDone
	return m
}

func oracleEffects(v [10]int32, count int, toggle func(k int) bool) []AbnormalStatusEffect {
	effects := make([]AbnormalStatusEffect, count)
	for k := range effects {
		effects[k] = AbnormalStatusEffect{
			SkillID: rowField(v, k), Level: rowField(v, k+1), DurationMillis: int(rowField(v, k+2)), Toggle: toggle(k),
		}
	}
	return effects
}

func oracleDurationEffects() []AbnormalStatusEffect {
	var effects []AbnormalStatusEffect
	for _, ms := range oracleDurations {
		effects = append(effects, AbnormalStatusEffect{SkillID: 1000 + ms%100, Level: 3, DurationMillis: int(ms)})
	}
	return effects
}

func addSkillEffectCases(m map[string]func() wire.Frame, r int, v [10]int32, n string) {
	// Skills 2 and 5 are toggles.
	m["AbnormalStatusUpdate"+n] = func() wire.Frame {
		return FrameAbnormalStatusUpdate(oracleEffects(v, 6, func(k int) bool { return k%3 == 2 }))
	}
	// The spectators' list: the fourth entry is a toggle, which gets no special encoding.
	m["ExOlympiadSpelledInfo"+n] = func() wire.Frame {
		return FrameExOlympiadSpelledInfo(rowField(v, 0), oracleEffects(v, 4, func(k int) bool { return k == 3 }))
	}
	m["ExOlympiadSpelledInfo/empty"+n] = func() wire.Frame { return FrameExOlympiadSpelledInfo(rowField(v, 1), nil) }
	if r != 0 {
		return
	}
	m["AbnormalStatusUpdate/empty"] = func() wire.Frame { return FrameAbnormalStatusUpdate(nil) }
	m["AbnormalStatusUpdate/durations"] = func() wire.Frame {
		// A toggle never reads its duration.
		return FrameAbnormalStatusUpdate(append(oracleDurationEffects(), AbnormalStatusEffect{SkillID: 7, Level: 1, DurationMillis: 5000, Toggle: true}))
	}
	m["ExOlympiadSpelledInfo/durations"] = func() wire.Frame {
		return FrameExOlympiadSpelledInfo(rowField(v, 0), oracleDurationEffects())
	}
}

// addSkillOrderCases is ProbeSkills.effectOrders: the toggles come after the
// other effects, sorted by id, and a second toggle of an id is dropped.
func addSkillOrderCases(m map[string]func() wire.Frame) {
	m["AbnormalStatusUpdate/orders"] = func() wire.Frame {
		e := func(id, level int32, ms int, toggle bool) AbnormalStatusEffect {
			return AbnormalStatusEffect{SkillID: id, Level: level, DurationMillis: ms, Toggle: toggle}
		}
		return FrameAbnormalStatusUpdate([]AbnormalStatusEffect{
			e(30, 1, 1000, true), e(7, 2, -1, false), e(10, 3, 2000, true), e(8, 4, 5000, false), e(20, 5, 3000, true),
			e(10, 6, 4000, true), e(-5, 7, 5000, true), e(30, 8, 6000, true), e(7, 9, 7000, false),
		})
	}
}

// oracleSkillListEntries is ProbeSkills.skillListPlayer: four skills, the odd
// ones passive, the last two clan skills.
func oracleSkillListEntries(v [10]int32, formalWear, clanDisabled bool) []SkillListEntry {
	entries := make([]SkillListEntry, 4)
	for k := range entries {
		entries[k] = SkillListEntry{
			ID: rowField(v, k), Level: rowField(v, k+1), Passive: k%2 == 1,
			Disabled: formalWear || (k >= 2 && clanDisabled),
		}
	}
	return entries
}

func addSkillListCases(m map[string]func() wire.Frame, r int, v [10]int32, n string) {
	m["SkillList"+n] = func() wire.Frame { return FrameSkillList(oracleSkillListEntries(v, false, false)) }
	if r != 0 {
		return
	}
	m["SkillList/empty"] = func() wire.Frame { return FrameSkillList(nil) }
	for formal := range 2 {
		for _, clan := range oracleSkillsClans {
			name := "SkillList/formal" + strconv.Itoa(formal) + "-clan-" + clan.name
			m[name] = func() wire.Frame { return FrameSkillList(oracleSkillListEntries(v, formal == 1, clan.disabled)) }
		}
	}
}

func addSkillReuseCases(m map[string]func() wire.Frame, r int, v [10]int32, n string) {
	// Four timers, reuse f(v,k+2) ms of which f(v,k+3) ms are left; a timer
	// with nothing left has run out and is not sent.
	m["SkillCoolTime"+n] = func() wire.Frame {
		var entries []SkillCoolTimeEntry
		for k := range 4 {
			if left := rowField(v, k+3); left > 0 {
				entries = append(entries, SkillCoolTimeEntry{
					SkillID: rowField(v, k), Level: rowField(v, k+1),
					ReuseSeconds: rowField(v, k+2) / 1000, RemainingSeconds: left / 1000,
				})
			}
		}
		return FrameSkillCoolTime(entries)
	}
	for _, count := range []int{0, 1, 3} {
		m["MagicSkillLaunched/t"+strconv.Itoa(count)+n] = func() wire.Frame {
			targets := make([]int32, count)
			for i := range targets {
				targets[i] = rowField(v, 3+i)
			}
			return FrameMagicSkillLaunched(rowField(v, 0), rowField(v, 1), rowField(v, 2), targets)
		}
	}
	if r != 0 {
		return
	}
	m["SkillCoolTime/empty"] = func() wire.Frame { return FrameSkillCoolTime(nil) }
	m["SkillCoolTime/units"] = func() wire.Frame {
		var entries []SkillCoolTimeEntry
		id := int32(1)
		for _, reuse := range []int32{999, 1000, 1001, 15999, -1, -999, -1999, 600000, 2147483647} {
			for _, left := range []int32{1, 999, 1000, 1001, 600000, 2147483647} {
				entries = append(entries, SkillCoolTimeEntry{SkillID: id, Level: 1, ReuseSeconds: reuse / 1000, RemainingSeconds: left / 1000})
				id++
			}
		}
		return FrameSkillCoolTime(entries)
	}
}

func addSkillTrainerCases(m map[string]func() wire.Frame, r int, v [10]int32, n string) {
	m["ExEnchantSkillList"+n] = func() wire.Frame {
		entries := make([]EnchantSkillEntry, 3)
		for k := range entries {
			entries[k] = EnchantSkillEntry{ID: rowField(v, k), Level: rowField(v, k+1), SPCost: rowField(v, k+7), XPCost: int64(rowField(v, k+6))}
		}
		return FrameExEnchantSkillList(entries)
	}
	grants := func(entry func(k int) AcquireSkillListEntry) []AcquireSkillListEntry {
		entries := make([]AcquireSkillListEntry, 3)
		for k := range entries {
			entries[k] = entry(k)
		}
		return entries
	}
	base := func(k int) AcquireSkillListEntry {
		return AcquireSkillListEntry{ID: rowField(v, k), Level: rowField(v, k+1)}
	}
	m["AcquireSkillList/usual"+n] = func() wire.Frame {
		// The class trainer shows the cost clamped to zero.
		return FrameAcquireSkillList(AcquireSkillTypeUsual, grants(func(k int) AcquireSkillListEntry {
			e := base(k)
			e.Cost = max(rowField(v, k+3), 0)
			return e
		}))
	}
	m["AcquireSkillList/fishing"+n] = func() wire.Frame {
		return FrameAcquireSkillList(AcquireSkillTypeFishing, grants(func(k int) AcquireSkillListEntry {
			e := base(k)
			e.Unknown = 1
			return e
		}))
	}
	m["AcquireSkillList/clan"+n] = func() wire.Frame {
		return FrameAcquireSkillList(AcquireSkillTypeClan, grants(func(k int) AcquireSkillListEntry {
			e := base(k)
			e.Cost = rowField(v, k+3)
			return e
		}))
	}
	if r == 0 {
		m["ExEnchantSkillList/empty"] = func() wire.Frame { return FrameExEnchantSkillList(nil) }
		m["AcquireSkillList/usual-empty"] = func() wire.Frame { return FrameAcquireSkillList(AcquireSkillTypeUsual, nil) }
	}
}

// oracleShortcut is ProbeSkills.shortcut: shortcut k of a row. The item ones
// read the owner's inventory and reuse timers (ProbeSkills.shortcutOwner),
// which the entry carries as the reference reads them: an augmentation id on
// the augmented item, the seconds left and the item skill's reuse delay on
// the item with a running timer.
func oracleShortcut(v [10]int32, k int) Shortcut {
	id := rowField(v, k+2)
	if k < 6 {
		id += int32(k) * 0x101
	}
	sc := Shortcut{
		Slot: rowField(v, k), Page: rowField(v, k+1), ID: id, CharacterType: rowField(v, k+4),
		Level: rowField(v, k+3), SharedReuseGroup: rowField(v, k+5),
	}
	sc.Type = []ShortcutType{
		ShortcutItem, ShortcutItem, ShortcutItem, ShortcutItem, ShortcutItem, ShortcutItem,
		ShortcutSkill, ShortcutAction, ShortcutMacro, ShortcutRecipe, ShortcutNone,
	}[k]
	switch k {
	case 2:
		sc.AugmentationID = rowField(v, 7)
	case 5:
		sc.RemainingSeconds = int32(max(int64(rowField(v, 9)), 0) / 1000)
		sc.ReuseSeconds = rowField(v, 8) / 1000
	}
	return sc
}

func addShortcutCases(m map[string]func() wire.Frame, r int, v [10]int32, n string) {
	all := make([]Shortcut, len(oracleShortcutKinds))
	for k, kind := range oracleShortcutKinds {
		all[k] = oracleShortcut(v, k)
		m["ShortCutRegister/"+kind+n] = func() wire.Frame { return FrameShortCutRegister(all[k]) }
	}
	m["ShortCutInit"+n] = func() wire.Frame { return FrameShortCutInit(all) }
	if r == 0 {
		m["ShortCutInit/empty"] = func() wire.Frame { return FrameShortCutInit(nil) }
	}
}

func addAttackCases(m map[string]func() wire.Frame, r int, v [10]int32, n string) {
	for _, count := range []int{1, 2, 5} {
		m["Attack/hits"+strconv.Itoa(count)+n] = func() wire.Frame {
			hits := make([]event.AttackHit, count)
			for i := range hits {
				hits[i] = event.AttackHit{TargetID: rowField(v, 3+i), Damage: int(rowField(v, 4+i)), Flags: uint8(rowField(v, 5+i) & 0xff)}
			}
			return FrameAttack(event.Attack{
				AttackerID: rowField(v, 0), X: int(rowField(v, 1)), Y: int(rowField(v, 2)), Z: int(rowField(v, 3)), Hits: hits,
			})
		}
	}
	if r != 0 {
		return
	}
	// Every flag the swing sets, alone and together.
	m["Attack/flags"] = func() wire.Frame {
		flags := []uint8{0, 0x10, 0x20, 0x40, 0x80, 0x30, 0xf0}
		hits := make([]event.AttackHit, len(flags))
		for i, f := range flags {
			hits[i] = event.AttackHit{TargetID: int32(100 + i), Damage: 10 * i, Flags: f}
		}
		return FrameAttack(event.Attack{AttackerID: 1, Hits: hits})
	}
}

// oracleDoor is ProbeSkills.door; the hit points are the double
// f(v,4)+0.5, which the door packets cast to an int.
type oracleDoor struct {
	v [10]int32
}

func (d oracleDoor) ObjectID() int32 { return rowField(d.v, 0) }
func (d oracleDoor) DoorID() int     { return int(rowField(d.v, 1)) }
func (d oracleDoor) Opened() bool    { return rowField(d.v, 2)&1 == 1 }
func (d oracleDoor) MaxHP() int      { return int(rowField(d.v, 3)) }
func (d oracleDoor) HP() int         { return int(float64(rowField(d.v, 4)) + 0.5) }
func (d oracleDoor) Damage() int     { return int(rowField(d.v, 5)) }

// oracleStatic is ProbeSkills.doors' static object.
type oracleStatic struct {
	v [10]int32
}

func (s oracleStatic) StaticObjectID() int { return int(rowField(s.v, 0)) }
func (s oracleStatic) ObjectID() int32     { return rowField(s.v, 1) }

func addDoorCases(m map[string]func() wire.Frame, r int, v [10]int32, n string) {
	d := oracleDoor{v}
	m["DoorInfo"+n] = func() wire.Frame { return FrameDoorInfo(d, false) }
	m["DoorInfo/attackable"+n] = func() wire.Frame { return FrameDoorInfo(d, true) }
	// The door of a castle under siege.
	m["DoorStatusUpdate"+n] = func() wire.Frame { return FrameDoorStatusUpdate(d, true) }
	m["StaticObjectInfo"+n] = func() wire.Frame { return FrameStaticObjectInfo(oracleStatic{v}) }
}

// addDoorResidenceCases is ProbeSkills.doorResidences: the status packet shows
// the hit points only for a door of a castle under siege.
func addDoorResidenceCases(m map[string]func() wire.Frame) {
	d := oracleDoor{oracleRows[0].v}
	for _, name := range []string{"no-residence", "castle-idle", "other-residence"} {
		m["DoorStatusUpdate/"+name] = func() wire.Frame { return FrameDoorStatusUpdate(d, false) }
	}
}

func addQuestListCases(m map[string]func() wire.Frame, r int, v [10]int32, n string) {
	m["QuestList"+n] = func() wire.Frame {
		quests := make([]QuestListEntry, 3)
		for k := range quests {
			quests[k] = QuestListEntry{QuestID: rowField(v, k), Flags: rowField(v, k+1)}
		}
		return FrameQuestList(quests)
	}
	if r == 0 {
		m["QuestList/empty"] = func() wire.Frame { return FrameQuestList(nil) }
	}
}

// oracleGauges is ProbeSkills.gauges: the row's competitor with the given
// current gauges.
func oracleOlympiad(r int, v [10]int32, hp, cp float64) OlympiadUserInfo {
	return OlympiadUserInfo{
		Side: int(rowField(v, 0)), ObjectID: rowField(v, 1), Name: oracleStrings[r].s, ClassID: int32(oracleClassIDs[r]),
		HP: hp, MaxHP: float64(rowField(v, 3)), CP: cp, MaxCP: float64(rowField(v, 5)),
	}
}

func oracleDuelist(r int, v [10]int32, hp, mp, cp float64) DuelUserInfo {
	return DuelUserInfo{
		Name: oracleStrings[r].s, ObjectID: rowField(v, 1), ClassID: int32(oracleClassIDs[r]), Level: rowField(v, 2),
		HP: hp, MaxHP: float64(rowField(v, 3)), MP: mp, MaxMP: float64(rowField(v, 4)), CP: cp, MaxCP: float64(rowField(v, 5)),
	}
}

func addGaugeWindowCases(m map[string]func() wire.Frame, r int, v [10]int32, n string) {
	hp, mp, cp := rowDouble(r, 0), rowDouble(r, 1), rowDouble(r, 2)
	m["ExOlympiadUserInfo"+n] = func() wire.Frame { return FrameExOlympiadUserInfo(oracleOlympiad(r, v, hp, cp)) }
	m["ExDuelUpdateUserInfo"+n] = func() wire.Frame { return FrameExDuelUpdateUserInfo(oracleDuelist(r, v, hp, mp, cp)) }
}

// addVitalsCases is ProbeSkills.vitals: each current gauge in every position
// of the shared vitals, on the windows that cast it to an int.
func addVitalsCases(m map[string]func() wire.Frame) {
	v := oracleRows[0].v
	crafter := rowField(v, 0)
	dwarvenBook := []recipe.Recipe{{ID: int(rowField(v, 4)), Dwarven: true}, {ID: int(rowField(v, 5)), Dwarven: true}}
	maxMP := float64(rowField(v, 3))
	for i := range oracleVitals {
		name := "/vitals" + strconv.Itoa(i)
		hp, mp, cp := oracleVitals[i], oracleVitals[(i+1)%len(oracleVitals)], oracleVitals[(i+2)%len(oracleVitals)]
		m["ExOlympiadUserInfo"+name] = func() wire.Frame { return FrameExOlympiadUserInfo(oracleOlympiad(0, v, hp, cp)) }
		m["ExDuelUpdateUserInfo"+name] = func() wire.Frame { return FrameExDuelUpdateUserInfo(oracleDuelist(0, v, hp, mp, cp)) }
		self := func() *oraclePlayer {
			o := newOraclePlayer(0, v, false)
			res := o.c.ResourceValues()
			res.CurrentHP, res.CurrentMP, res.CurrentCP = hp, mp, cp
			o.c.SetResourceValues(res)
			return o
		}
		m["UserInfo"+name] = func() wire.Frame { return FrameUserInfo(self().user()) }
		m["CharInfo"+name] = func() wire.Frame { return FrameCharInfo(self().char()) }

		m["RecipeShopSellList"+name] = func() wire.Frame {
			return FrameRecipeShopSellList(crafter, mp, maxMP, int(rowField(v, 2)), nil)
		}
		m["RecipeShopItemInfo"+name] = func() wire.Frame { return FrameRecipeShopItemInfo(crafter, rowField(v, 4), mp, maxMP) }
		m["RecipeBookItemList"+name] = func() wire.Frame { return FrameRecipeBookItemList(true, maxMP, dwarvenBook) }
		m["RecipeItemMakeInfo"+name] = func() wire.Frame { return FrameRecipeItemMakeInfo(dwarvenBook[0], mp, maxMP, -1) }
	}
}
