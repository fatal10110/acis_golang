package serverpackets

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// oracleDiceHeadings are the roller headings ProbeMisc.DICE_HEADINGS throws a
// die at.
var oracleDiceHeadings = []int{0, 8192, 12345, 16384, 32768, 40000, 49152, 60000, 65535}

// oracleMiscCases replays ProbeMisc: html dialogs and their size limit, the
// community board page cuts, fishing, storage limits, enchant and
// augmentation windows, dice, observer mode, clan hall decoration, fences,
// npc chat, pet status, the sky, and the login and character creation
// packets.
func oracleMiscCases() map[string]func() wire.Frame {
	m := map[string]func() wire.Frame{}
	for r, row := range oracleRows {
		v, n := row.v, "/"+row.name
		s := oracleStrings[r].s
		for _, str := range oracleStrings {
			m["NpcHtmlMessage/"+str.name+n] = func() wire.Frame { return FrameNpcHtmlMessage(v[0], NpcHtmlBody(str.s), v[1]) }
		}
		bait := location.Location{X: int(v[2]), Y: int(v[3]), Z: int(v[4])}
		for _, night := range []bool{false, true} {
			for _, ranked := range []bool{false, true} {
				name := map[bool]string{false: "day", true: "night"}[night] + map[bool]string{false: "", true: "-ranked"}[ranked]
				m["ExFishingStart/"+name+n] = func() wire.Frame { return FrameExFishingStart(v[0], v[1], bait, night, ranked) }
			}
		}
		m["ExFishingStartCombat"+n] = func() wire.Frame {
			return FrameExFishingStartCombat(v[0], v[1], v[2], uint8(v[3]), uint8(v[4]), uint8(v[5]))
		}
		m["ExFishingHpRegen"+n] = func() wire.Frame {
			return FrameExFishingHpRegen(v[0], FishingHPRegen{
				Time: v[1], HP: v[2], Mode: uint8(v[3]), GoodUse: uint8(v[4]), Anim: uint8(v[5]), Penalty: v[6], Deceptive: uint8(v[7]),
			})
		}
		m["ExConfirmCancelItem"+n] = func() wire.Frame { return FrameExConfirmCancelItem(v[0], v[1], v[2], int64(v[3])) }
		// The roller stands at the row's position facing heading 40000; the
		// die lands 30 units ahead of it.
		m["Dice"+n] = func() wire.Frame {
			at := location.OrientedLocation{Location: location.Location{X: int(v[0]), Y: int(v[1]), Z: int(v[2])}, Heading: 40000}
			return FrameDice(v[3], v[4], v[5], at.Ahead(30))
		}
		m["ObserverStart"+n] = func() wire.Frame {
			return FrameObserverStart(location.Location{X: int(v[0]), Y: int(v[1]), Z: int(v[2])}, int(v[3]), int(v[4]))
		}
		m["ObserverEnd"+n] = func() wire.Frame {
			return FrameObserverEnd(location.Location{X: int(v[5]), Y: int(v[6]), Z: int(v[7])})
		}
		m["ClanHallDecoration"+n] = func() wire.Frame {
			return FrameClanHallDecoration(ClanHallDecoration{
				HallID: v[0], RestoreHP: int(v[1]), RestoreMP: int(v[2]), RestoreExp: int(v[3]), Teleport: int(v[4]),
				Curtains: int(v[5]), SupportMagic: int(v[6]), Fixtures: int(v[7]), CreateItem: int(v[8]),
			})
		}
		m["ExColosseumFenceInfo"+n] = func() wire.Frame {
			return FrameExColosseumFenceInfo(FenceInfo{ObjectID: v[0], Type: v[1], X: v[2], Y: v[3], Z: v[4], SizeX: v[5], SizeY: v[6]})
		}
		m["NpcSay/all"+n] = func() wire.Frame { return FrameNpcSay(0, int(v[1]), SayTypeAll, s) }
		m["NpcSay/shout"+n] = func() wire.Frame { return FrameNpcSay(0, int(v[1]), SayTypeShout, s) }
		m["NpcSay/npc"+n] = func() wire.Frame { return FrameNpcSay(v[0], int(v[2]), SayTypeAll, s) }
		m["PetStatusShow"+n] = func() wire.Frame { return FramePetStatusShow(int(v[0])) }
	}

	// A page longer than 8192 UTF-16 code units is replaced, however few
	// bytes or runes it holds.
	a8191 := strings.Repeat("a", 8191)
	for name, html := range map[string]string{
		"limit-kept":      a8191 + "a",
		"limit-cut":       a8191 + "aa",
		"limit-wide-kept": a8191 + "日",
		"limit-pair-cut":  a8191 + "😀",
	} {
		m["NpcHtmlMessage/"+name] = func() wire.Frame { return FrameNpcHtmlMessage(7, NpcHtmlBody(html), 0) }
	}

	for _, h := range oracleDiceHeadings {
		m["Dice/heading-"+strconv.Itoa(h)] = func() wire.Frame {
			at := location.OrientedLocation{Location: location.Location{X: -84318, Y: 244579, Z: -3730}, Heading: h}
			return FrameDice(0x10203040, 4625, 6, at.Ahead(30))
		}
	}

	m["ExStorageMaxCount/human"] = func() wire.Frame { return FrameExStorageMaxCount(oracleStorageCharacter(player.RaceHuman)) }
	m["ExStorageMaxCount/dwarf"] = func() wire.Frame { return FrameExStorageMaxCount(oracleStorageCharacter(player.RaceDwarf)) }

	for name, result := range map[string]EnchantResult{
		"success":            EnchantResultSuccess,
		"broken-crystals":    EnchantResultBrokenWithCrystals,
		"cancelled":          EnchantResultCancelled,
		"unsuccess":          EnchantResultUnsuccess,
		"broken-no-crystals": EnchantResultBrokenNoCrystals,
	} {
		m["EnchantResult/"+name] = func() wire.Frame { return FrameEnchantResult(result) }
	}
	m["ExShowVariationMakeWindow"] = FrameExShowVariationMakeWindow
	m["ExShowVariationCancelWindow"] = FrameExShowVariationCancelWindow
	m["ExShowQuestInfo"] = FrameExShowQuestInfo
	m["SunRise"] = FrameSunRise
	m["SunSet"] = FrameSunSet

	m["AuthLoginFail"] = func() wire.Frame { return FrameAuthLoginFail(LoginFailSystemErrorTryLater) }
	key := make([]byte, 16)
	for i := range key {
		key[i] = byte(0xf1 + i)
	}
	m["VersionCheck/cipher"] = func() wire.Frame { return FrameVersionCheck(key, true) }
	m["VersionCheck/plain"] = func() wire.Frame { return FrameVersionCheck(key, false) }
	m["NewCharacterSuccess"] = func() wire.Frame { return mustFrame(FrameNewCharacterSuccess(oracleRootTemplates())) }

	a, b := strings.Repeat("a", 4090), strings.Repeat("b", 4090)
	for name, html := range map[string]string{
		"short":       "<html/>",
		"one-part":    a,
		"two-parts":   a + strings.Repeat("c", 10),
		"three-parts": a + b + strings.Repeat("c", 10),
		"too-long":    a + b + strings.Repeat("c", 4090),
		"wide":        strings.Repeat("a", 4089) + "é",
		// The cut falls between the two halves of a surrogate pair.
		"straddle": strings.Repeat("a", 4089) + "\U0001F600" + "b",
	} {
		parts := FramesShowBoardPage(html)
		for i, part := range parts {
			part.Release()
			m["ShowBoardPage/"+name+"/"+strconv.Itoa(i)] = func() wire.Frame {
				var picked wire.Frame
				for j, f := range FramesShowBoardPage(html) {
					if j == i {
						picked = f
					} else {
						f.Release()
					}
				}
				return picked
			}
		}
	}
	for name, html := range map[string]string{"fits": strings.Repeat("a", 8179), "too-long": strings.Repeat("a", 8180)} {
		if f, ok := FrameShowBoardEdit(html); ok {
			f.Release()
			m["ShowBoardEdit/"+name+"/0"] = func() wire.Frame {
				f, _ := FrameShowBoardEdit(html)
				return f
			}
		}
	}
	return m
}

// oracleStorageCharacter is a character of race whose limits are the
// probe's: human 90, 12, 15, 17, 18, 21, 23 and dwarf 117, 13, 15, 18, 19,
// 21, 23, each a configured base plus its limit stat.
func oracleStorageCharacter(race player.Race) *player.Character {
	c := &player.Character{Race: race}
	c.SetName("S")
	c.Configure(player.Runtime{Rules: player.NewRuleBook(player.Rules{
		InventorySlots: player.InventorySlots{NoDwarf: 90, Dwarf: 117, Configured: true},
		StorageSlots: player.StorageSlots{
			WarehouseNoDwarf: 11, WarehouseDwarf: 12, Freight: 13,
			PrivateStoreNoDwarf: 14, PrivateStoreDwarf: 15,
			DwarfRecipe: 16, CommonRecipe: 17, Configured: true,
		},
	})})
	c.AddStatFuncs([]effect.Mod{
		{Stat: stat.WhLim, Op: effect.OpAdd, Value: 1.9},
		{Stat: stat.FreightLim, Op: effect.OpAdd, Value: 2},
		{Stat: stat.PSellLim, Op: effect.OpAdd, Value: 3},
		{Stat: stat.PBuyLim, Op: effect.OpAdd, Value: 4},
		{Stat: stat.RecDLim, Op: effect.OpAdd, Value: 5},
		{Stat: stat.RecCLim, Op: effect.OpAdd, Value: 6},
	})
	return c
}

// oracleRootTemplates holds the nine root professions with the probe's base
// stats: the k-th root (in creation screen order) has STR 10+k, DEX 20+k,
// CON 30+k, INT 40+k, WIT 50+k and MEN 60+k.
func oracleRootTemplates() *player.TemplateTable {
	templates := map[int]*player.Template{}
	for k, id := range []int{0, 10, 18, 25, 31, 38, 44, 49, 53} {
		templates[id] = &player.Template{
			ID: id, BaseLevel: 1,
			STR: 10 + k, DEX: 20 + k, CON: 30 + k, INT: 40 + k, WIT: 50 + k, MEN: 60 + k,
		}
	}
	table, err := player.NewTemplateTable(templates)
	if err != nil {
		panic(err)
	}
	return table
}
