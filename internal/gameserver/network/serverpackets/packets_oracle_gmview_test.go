package serverpackets

import (
	"strconv"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// oracleGMViewCases replays ProbeGMView: the game master's views of a
// character over the character, skill, quest and item rows of the other
// families, and the raid point record.
func oracleGMViewCases() map[string]func() wire.Frame {
	m := map[string]func() wire.Frame{}
	for r, row := range oracleRows {
		v, n := row.v, "/"+row.name
		m["GMViewCharacterInfo"+n] = func() wire.Frame { return FrameGMViewCharacterInfo(newOraclePlayer(r, v, false).user()) }
		m["GMViewSkillInfo"+n] = func() wire.Frame {
			return FrameGMViewSkillInfo(oracleStr(r), oracleSkillListEntries(v, false, false))
		}
		m["GMViewQuestList"+n] = func() wire.Frame {
			quests := make([]QuestListEntry, 3)
			for k := range quests {
				quests[k] = QuestListEntry{QuestID: rowField(v, k), Flags: rowField(v, k+1)}
			}
			return FrameGMViewQuestList(oracleStr(r+1), quests)
		}
		// The shared item list plus an unaugmented weapon in a player's
		// warehouse, and the next row's list in a clan's, under its leader's
		// name.
		m["GMViewWarehouseWithdrawList/player"+n] = func() wire.Frame {
			items := append(oracleItems(v), oracleItem(v, oracleWeapon, false, false))
			return mustFrame(FrameGMViewWarehouseWithdrawList(oracleStr(r), rowField(v, 2), items, oracleItemTable(v)))
		}
		m["GMViewWarehouseWithdrawList/clan"+n] = func() wire.Frame {
			next := oracleRows[(r+1)%len(oracleRows)].v
			return mustFrame(FrameGMViewWarehouseWithdrawList(oracleStr(r+2), rowField(v, 3), oracleItems(next), oracleItemTable(next)))
		}
		m["ExGetBossRecord"+n] = func() wire.Frame {
			entries := make([]BossRecordEntry, 3)
			for k := range entries {
				entries[k] = BossRecordEntry{BossID: rowField(v, k) + int32(7*k), Points: rowField(v, k+3)}
			}
			return FrameExGetBossRecord(rowField(v, 6), rowField(v, 7), entries, true)
		}
	}

	a := oracleRows[0].v
	variant := func(live bool, apply func(o *oraclePlayer)) func() wire.Frame {
		return func() wire.Frame {
			o := newOraclePlayer(0, a, live)
			apply(o)
			return FrameGMViewCharacterInfo(o.user())
		}
	}
	variants := oracleVariants()
	m["GMViewCharacterInfo/live"] = variant(true, func(*oraclePlayer) {})
	m["GMViewCharacterInfo/live-flying"] = variant(true, func(o *oraclePlayer) { o.c.SetFlying(true) })
	for _, name := range []string{"gm", "hero", "noble"} {
		m["GMViewCharacterInfo/"+name] = variant(false, variants[name])
	}
	for i := range oracleVitals {
		hp, mp, cp := oracleVitals[i], oracleVitals[(i+1)%len(oracleVitals)], oracleVitals[(i+2)%len(oracleVitals)]
		m["GMViewCharacterInfo/vitals"+strconv.Itoa(i)] = variant(false, func(o *oraclePlayer) {
			res := o.c.ResourceValues()
			res.CurrentHP, res.CurrentMP, res.CurrentCP = hp, mp, cp
			o.c.SetResourceValues(res)
		})
	}

	for formal := range 2 {
		for _, clan := range oracleSkillsClans {
			m["GMViewSkillInfo/formal"+strconv.Itoa(formal)+"-clan-"+clan.name] = func() wire.Frame {
				return FrameGMViewSkillInfo(oracleStr(0), oracleSkillListEntries(a, formal == 1, clan.disabled))
			}
		}
	}
	m["GMViewSkillInfo/empty"] = func() wire.Frame { return FrameGMViewSkillInfo("Bo", nil) }
	m["GMViewQuestList/empty"] = func() wire.Frame { return FrameGMViewQuestList("Bo", nil) }
	m["GMViewWarehouseWithdrawList/empty"] = func() wire.Frame {
		return mustFrame(FrameGMViewWarehouseWithdrawList("Bo", 0, nil, item.NewTable(nil)))
	}
	m["ExGetBossRecord/none"] = func() wire.Frame { return FrameExGetBossRecord(3, 7, nil, false) }
	m["ExGetBossRecord/empty"] = func() wire.Frame { return FrameExGetBossRecord(0, 0, nil, true) }
	return m
}
