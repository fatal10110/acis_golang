package serverpackets

import (
	"strconv"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
)

// oracleEngineCases are the packets the reference builds from its item, clan,
// siege, castle, manor and character graphs (testdata/oracle/ProbeEngine.java,
// which runs the reference packet classes over stand-ins for those graphs).
// The inputs are the shared rows of oracleRows.
func oracleEngineCases() map[string]func() wire.Frame {
	m := map[string]func() wire.Frame{}
	addSystemMessageCases(m)
	addConfirmDlgCases(m)
	addDieCases(m)
	addCharSelectedCases(m)
	addCharSelectInfoCases(m)
	addManorCases(m)
	for name, build := range oracleCharacterCases() {
		m[name] = build
	}
	return m
}

func addSystemMessageCases(m map[string]func() wire.Frame) {
	for r, row := range oracleRows {
		v, n, s := row.v, "/"+row.name, oracleStrings[r].s
		id := int(v[0])
		m["SystemMessage/none"+n] = func() wire.Frame { return FrameSystemMessage(id) }
		m["SystemMessage/string"+n] = func() wire.Frame { return FrameSystemMessageString(id, s) }
		m["SystemMessage/itemname"+n] = func() wire.Frame { return FrameSystemMessageItemName(id, v[1]) }
		m["SystemMessage/number"+n] = func() wire.Frame { return FrameSystemMessageNumber(id, v[1]) }
		m["SystemMessage/stringitemname"+n] = func() wire.Frame { return FrameSystemMessageStringItemName(id, s, v[1]) }
		m["SystemMessage/stringnumber"+n] = func() wire.Frame { return FrameSystemMessageStringNumber(id, s, v[1]) }
		m["SystemMessage/twonumbers"+n] = func() wire.Frame { return FrameSystemMessageTwoNumbers(id, v[1], v[2]) }
		m["SystemMessage/numberitemname"+n] = func() wire.Frame { return FrameSystemMessageNumberItemName(id, v[1], v[2]) }
		m["SystemMessage/stringnumberitemname"+n] = func() wire.Frame {
			return FrameSystemMessageStringNumberItemName(id, s, v[1], v[2])
		}
		m["SystemMessage/itemnamenumber"+n] = func() wire.Frame { return FrameSystemMessageItemNameNumber(id, v[1], v[2]) }
		m["SystemMessage/itemnameitemnumber"+n] = func() wire.Frame { return FrameSystemMessageItemNameItemNumber(id, v[1], v[2]) }
		m["SystemMessage/skillname"+n] = func() wire.Frame { return FrameSystemMessageSkillName(id, v[1], v[2]) }
		m["SystemMessage/stringskillname"+n] = func() wire.Frame { return FrameSystemMessageStringSkillName(id, s, v[1], v[2]) }
		m["SystemMessage/params"+n] = func() wire.Frame {
			return FrameSystemMessageParams(id, TextParam(s), NumberParam(v[1]), ItemNameParam(v[2]), ItemNumberParam(v[3]),
				ZoneNameParam(int(v[4]), int(v[5]), int(v[6])))
		}
		m["SystemMessage/fortid"+n] = func() wire.Frame { return FrameSystemMessageParams(id, CastleNameParam(v[1]), TextParam(s)) }
	}
	// The reference's plain-text message is its S1 message (id 109).
	m["SystemMessage/sendString"] = func() wire.Frame { return FrameSystemMessageString(109, "Bo") }
}

func addConfirmDlgCases(m map[string]func() wire.Frame) {
	for r, row := range oracleRows {
		v, n, s := row.v, "/"+row.name, oracleStrings[r].s
		m["ConfirmDlg/summon"+n] = func() wire.Frame {
			return FrameConfirmDlgSummonFriendRequest(s, v[0], v[1], v[2], v[3], time.Duration(v[4])*time.Millisecond)
		}
		m["ConfirmDlg/resurrection"+n] = func() wire.Frame { return FrameConfirmDlgResurrectionRequest(s) }
		m["ConfirmDlg/engage"+n] = func() wire.Frame { return FrameConfirmDlgEngageRequest(s) }
	}
	m["ConfirmDlg/summon/time-only"] = func() wire.Frame {
		return FrameConfirmDlgSummonFriendRequest("Bo", 0, 1, 2, 3, 30*time.Second)
	}
	m["ConfirmDlg/summon/requester-only"] = func() wire.Frame {
		return FrameConfirmDlgSummonFriendRequest("Bo", 0x01020304, 1, 2, 3, 0)
	}
	m["ConfirmDlg/gate/open"] = func() wire.Frame { return FrameConfirmDlgGate(true) }
	m["ConfirmDlg/gate/close"] = func() wire.Frame { return FrameConfirmDlgGate(false) }
}

func addDieCases(m map[string]func() wire.Frame) {
	for _, row := range oracleRows {
		id := row.v[0]
		m["Die/plain/"+row.name] = func() wire.Frame { return FrameDie(id, DieOptions{}) }
	}
	const id = 0x01020304
	for name, opts := range map[string]DieOptions{
		"fixedres":                    {FixedRes: true},
		"clanhall":                    {ClanHall: true},
		"castle":                      {Castle: true},
		"clanhall-castle":             {ClanHall: true, Castle: true},
		"clan-flag-no-siege":          {},
		"siege-owner":                 {Castle: true},
		"siege-defender":              {Castle: true},
		"siege-attacker-flag":         {SiegeHQ: true},
		"siege-attacker-no-flag":      {},
		"siege-pending":               {},
		"siege-pending-castle":        {Castle: true},
		"siege-attacker-clanhall":     {ClanHall: true, SiegeHQ: true},
		"hall-siege-attacker-flag":    {SiegeHQ: true},
		"hall-siege-attacker-no-flag": {},
		"hall-siege-defender-flag":    {},
		"hall-siege-residences":       {ClanHall: true, Castle: true, SiegeHQ: true},
		"siege-over-hall":             {Castle: true},
		"no-clan-siege":               {},
		"everything":                  {ClanHall: true, Castle: true, SiegeHQ: true, FixedRes: true},
		"monster-sweep":               {Sweep: true},
		"monster-plain":               {},
	} {
		m["Die/"+name] = func() wire.Frame { return FrameDie(id, opts) }
	}
}

// oracleClassIDs are the class ids of the reference's class table entries the
// probe picks per row (ProbeEngine.CLASS_INDEX; an entry's id is its index).
var oracleClassIDs = [...]int{0, 18, 53, 88}

func addCharSelectedCases(m map[string]func() wire.Frame) {
	for r, row := range oracleRows {
		v := row.v
		m["CharSelected/"+row.name] = func() wire.Frame {
			c := &player.Character{
				ID:          v[0],
				Race:        player.Race(r),
				Location:    loc(v[3], v[4], v[5]),
				SP:          int(v[8]),
				Exp:         int64(v[9])*1000003 + int64(v[0]),
				CharLevel:   int(v[1] ^ v[2]),
				KarmaPoints: int(v[3] ^ v[4]),
				PKKills:     int(v[5] ^ v[6]),
			}
			c.SetName(oracleStrings[r].s)
			c.SetTitle(oracleStrings[(r+1)%len(oracleStrings)].s)
			c.SetClanID(v[1])
			c.SetSex(player.Sex(v[2] & 1))
			c.SetClassID(oracleClassIDs[r])
			c.SetResourceValues(player.Resources{CurrentHP: float64(v[6]) + 0.5, CurrentMP: float64(v[7]) + 0.25})
			tmpl := &player.Template{
				INT: int(v[0] + 1), STR: int(v[1] + 2), CON: int(v[2] + 3),
				MEN: int(v[3] + 4), DEX: int(v[4] + 5), WIT: int(v[5] + 6),
			}
			return FrameCharSelected(CharSelectedSnapshot{Character: c, Template: tmpl, SessionID: v[2], GameTime: v[6] ^ v[7]})
		}
	}
}

// oracleSlotObjectID and oracleSlotItemID are ProbeEngine.paperdollObjectId
// and paperdollItemId: the equipped item of a paperdoll slot, from a row.
func oracleSlotObjectID(v [10]int32, slot int) int32 { return v[slot%10] + int32(slot)*0x101 }
func oracleSlotItemID(v [10]int32, slot int) int32   { return v[(slot+3)%10] - int32(slot)*7 }

// oracleSlot is ProbeEngine.character and equip: a character row of the shared
// input and its seventeen equipped items, as the list entry the reference
// builds from them.
func oracleSlot(v [10]int32, objectID int32, name string, accessLevel int32, lastAccess int64, baseClass, classID int32, rhandEnchant int, augmentation *int32) CharacterSlot {
	if baseClass == 0 && classID > 0 {
		baseClass = classID
	}
	slot := CharacterSlot{
		Name: name, ObjectID: objectID, ClanID: v[2],
		Sex: player.Sex(v[3] & 1), Race: player.Race(v[3] & 7), BaseClassID: baseClass, ClassID: classID,
		X: v[5], Y: v[6], Z: v[7],
		CurHP: float64(v[3]) + 0.5, CurMP: float64(v[5]) + 0.25, MaxHP: float64(v[2]), MaxMP: float64(v[4]),
		SP: v[4] ^ v[5], Exp: int64(v[0])*1000003 + int64(v[1]), Level: v[1],
		Karma: v[8], PKKills: v[9], PvPKills: v[0] ^ v[9],
		HairStyle: v[7], HairColor: v[8] ^ v[7], Face: v[6],
		AccessLevel: accessLevel, LastAccess: lastAccess,
	}
	if accessLevel < 0 {
		slot.DeleteTimerSeconds = -1
	}
	for s := 0; s <= 16; s++ {
		enchant := s
		if s == rhandPaperdollIndex {
			enchant = rhandEnchant
		}
		slot.Paperdoll[s] = item.PaperdollEntry{ObjectID: oracleSlotObjectID(v, s), TemplateID: oracleSlotItemID(v, s), EnchantLevel: enchant}
	}
	if augmentation != nil {
		slot.Paperdoll[rhandPaperdollIndex].AugmentationID = *augmentation
	}
	return slot
}

// oracleBareSlot is a character with no equipped items.
func oracleBareSlot(v [10]int32, objectID int32, name string, accessLevel int32, lastAccess int64, baseClass, classID int32) CharacterSlot {
	slot := oracleSlot(v, objectID, name, accessLevel, lastAccess, baseClass, classID, 0, nil)
	slot.Paperdoll = [item.PaperdollSlots]item.PaperdollEntry{}
	return slot
}

func addCharSelectInfoCases(m map[string]func() wire.Frame) {
	for r, row := range oracleRows {
		v := row.v
		m["CharSelectInfo/one/"+row.name] = func() wire.Frame {
			var aug *int32
			if oracleSlotObjectID(v, rhandPaperdollIndex) > 0 {
				aug = &v[8]
			}
			slot := oracleSlot(v, v[0], oracleStrings[r].s, 0, 0, 10, 10, int(v[9]), aug)
			return FrameCharSelectInfo("acct", v[2], []CharacterSlot{slot}, 0)
		}
	}
	a, b, c := oracleRows[0].v, oracleRows[1].v, oracleRows[3].v
	three := func() []CharacterSlot {
		return []CharacterSlot{
			oracleBareSlot(a, 11, "One", 0, 50, 10, 10),
			oracleBareSlot(b, 12, "Two", 0, 90, 18, 18),
			oracleBareSlot(c, 13, "Three", 0, 90, 53, 53),
		}
	}
	m["CharSelectInfo/empty"] = func() wire.Frame { return FrameCharSelectInfo("acct", a[0], nil, -1) }
	m["CharSelectInfo/auto-recent"] = func() wire.Frame { return FrameCharSelectInfo("acct", 7, three(), -1) }
	m["CharSelectInfo/explicit-last"] = func() wire.Frame { return FrameCharSelectInfo("acct", 7, three(), 2) }
	m["CharSelectInfo/explicit-none"] = func() wire.Frame { return FrameCharSelectInfo("acct", 7, three(), 5) }
	m["CharSelectInfo/auto-never-played"] = func() wire.Frame {
		slots := []CharacterSlot{oracleBareSlot(a, 11, "One", 0, 0, 10, 10), oracleBareSlot(b, 12, "Two", 0, 0, 18, 18)}
		return FrameCharSelectInfo("acct", 7, slots, -1)
	}
	m["CharSelectInfo/banned"] = func() wire.Frame {
		return FrameCharSelectInfo("acct", 7, []CharacterSlot{oracleBareSlot(a, 11, "Banned", -1, 0, 10, 10)}, 0)
	}
	m["CharSelectInfo/subclass"] = func() wire.Frame {
		slot := oracleBareSlot(a, 11, "Sub", 0, 0, 10, 18)
		slot.Exp, slot.SP, slot.Level = 0x0102030405060708, 777, 44
		return FrameCharSelectInfo("acct", 7, []CharacterSlot{slot}, 0)
	}
	m["CharSelectInfo/base-unset"] = func() wire.Frame {
		return FrameCharSelectInfo("acct", 7, []CharacterSlot{oracleBareSlot(a, 11, "Base", 0, 0, 0, 53)}, 0)
	}
	for _, augmentation := range []int32{8191<<16 | 3, -1} {
		m["CharSelectInfo/augment/"+strconv.Itoa(int(augmentation))] = func() wire.Frame {
			return FrameCharSelectInfo("acct", 7, []CharacterSlot{oracleSlot(a, 11, "Aug", 0, 0, 10, 10, 5, &augmentation)}, 0)
		}
	}
	for _, enchant := range []int{126, 127, 128, 200} {
		m["CharSelectInfo/enchant/"+strconv.Itoa(enchant)] = func() wire.Frame {
			return FrameCharSelectInfo("acct", 7, []CharacterSlot{oracleSlot(a, 11, "Ench", 0, 0, 10, 10, enchant, nil)}, 0)
		}
	}
	m["CharSelectInfo/own-items"] = func() wire.Frame {
		return FrameCharSelectInfo("acct", 7, []CharacterSlot{oracleSlot(a, 11, "Own", 0, 0, 10, 10, 3, nil)}, 0)
	}
}

// The probe's three manor seeds (ProbeEngine.manor), one per castle, with the
// reference prices of their items; the last has no price, which reads as 1.
var (
	oracleSeedA = manor.Seed{
		CropID: 5073, SeedID: 5016, MatureID: 5103, Level: 10, Reward1: 1864, Reward2: 1878,
		CastleID: 1, SeedsLimit: 1000, CropsLimit: 2000, SeedReferencePrice: 7, CropReferencePrice: 51,
	}
	oracleSeedB = manor.Seed{
		CropID: 5068, SeedID: 5012, MatureID: 5098, Level: 20, Reward1: 1865, Reward2: 1879,
		CastleID: 2, Alternative: true, SeedsLimit: 500, CropsLimit: 1500, SeedReferencePrice: 13, CropReferencePrice: 96,
	}
	oracleSeedC = manor.Seed{
		CropID: 5100, SeedID: 5090, MatureID: 5110, Level: 30, Reward1: 1866, Reward2: 1880,
		CastleID: 3, SeedsLimit: 300, CropsLimit: 700, SeedReferencePrice: 1, CropReferencePrice: 1,
	}
)

// oracleManorRate is the probe's drop-rate multiplier on the seed limits.
const oracleManorRate = 2

func addManorCases(m map[string]func() wire.Frame) {
	A, B, C := oracleSeedA, oracleSeedB, oracleSeedC
	for _, hide := range oracleBools {
		h, hidden := "/"+map[bool]string{true: "hidden", false: "shown"}[hide.b], hide.b
		m["ExShowSeedInfo/current"+h] = func() wire.Frame {
			return FrameExShowSeedInfo(1, hidden, []ManorListing{
				{ID: 5016, Amount: 7, StartAmount: 10, Price: 30, Seed: A},
				{ID: 9999, Amount: 1, StartAmount: 2, Price: 3},
			})
		}
		m["ExShowCropInfo/current"+h] = func() wire.Frame {
			return FrameExShowCropInfo(1, hidden, []ManorListing{
				{ID: 5073, Amount: 40, StartAmount: 45, Price: 50, Reward: 2, Seed: A},
				{ID: 7777, Amount: 1, StartAmount: 2, Price: 3, Reward: 1},
			})
		}
		m["ExShowManorDefaultInfo"+h] = func() wire.Frame { return FrameExShowManorDefaultInfo(hidden, []manor.Seed{C, B, A}) }
	}
	m["ExShowSeedInfo/empty"] = func() wire.Frame { return FrameExShowSeedInfo(3, false, nil) }
	m["ExShowCropInfo/empty"] = func() wire.Frame { return FrameExShowCropInfo(4, true, nil) }
	// The next period of a manor that is not approved is shown as no rows.
	m["ExShowSeedInfo/next-unapproved"] = func() wire.Frame { return FrameExShowSeedInfo(1, false, nil) }
	m["ExShowCropInfo/next-unapproved"] = func() wire.Frame { return FrameExShowCropInfo(1, false, nil) }
	m["ExShowSeedInfo/next-approved"] = func() wire.Frame {
		return FrameExShowSeedInfo(1, true, []ManorListing{{ID: 5016, Amount: 12, StartAmount: 15, Price: 33, Seed: A}})
	}
	m["ExShowCropInfo/next-approved"] = func() wire.Frame {
		return FrameExShowCropInfo(1, true, []ManorListing{{ID: 5073, Amount: 22, StartAmount: 30, Price: 55, Reward: 1, Seed: A}})
	}

	seedRows := map[int32][]ManorSetting{
		1: {{Seed: A, Limit: int32(A.SeedsLimit * oracleManorRate), Current: ManorPeriodSetting{StartAmount: 10, Price: 30}, Next: ManorPeriodSetting{StartAmount: 15, Price: 33}}},
		2: {{Seed: B, Limit: int32(B.SeedsLimit * oracleManorRate), Next: ManorPeriodSetting{StartAmount: 9, Price: 20}}},
		3: {{Seed: C, Limit: int32(C.SeedsLimit * oracleManorRate)}},
		4: nil,
	}
	cropRows := map[int32][]ManorSetting{
		1: {{Seed: A, Limit: int32(A.CropsLimit * oracleManorRate), Current: ManorPeriodSetting{StartAmount: 45, Price: 50, Reward: 2}, Next: ManorPeriodSetting{StartAmount: 30, Price: 55, Reward: 1}}},
		2: {{Seed: B, Limit: int32(B.CropsLimit * oracleManorRate), Current: ManorPeriodSetting{StartAmount: 5, Price: 70, Reward: 2}}},
		3: {{Seed: C, Limit: int32(C.CropsLimit * oracleManorRate)}},
		4: nil,
	}
	for castle := int32(1); castle <= 4; castle++ {
		name := "/castle" + strconv.Itoa(int(castle))
		m["ExShowSeedSetting"+name] = func() wire.Frame { return FrameExShowSeedSetting(castle, seedRows[castle]) }
		m["ExShowCropSetting"+name] = func() wire.Frame { return FrameExShowCropSetting(castle, cropRows[castle]) }
	}

	// Only castles that still want the crop are listed.
	m["ExShowProcureCropDetail/5073"] = func() wire.Frame {
		return FrameExShowProcureCropDetail(5073, []ManorCastleProcure{
			{CastleID: 1, Amount: 40, Price: 50, Reward: 2},
			{CastleID: 2, Amount: 3, Price: 31, Reward: 1},
		})
	}
	m["ExShowProcureCropDetail/5068"] = func() wire.Frame { return FrameExShowProcureCropDetail(5068, nil) }
	m["ExShowProcureCropDetail/5100"] = func() wire.Frame { return FrameExShowProcureCropDetail(5100, nil) }

	m["ExShowSellCropList/one"] = func() wire.Frame {
		return FrameExShowSellCropList(1, []ManorHeldCrop{
			{ObjectID: 268, CropID: 5073, Count: 12, Seed: A, Procure: &ManorListing{Amount: 40, Price: 50, Reward: 2}},
		})
	}
	m["ExShowSellCropList/not-wanted"] = func() wire.Frame {
		return FrameExShowSellCropList(3, []ManorHeldCrop{{ObjectID: 268, CropID: 5073, Count: 12, Seed: A}})
	}
	m["ExShowSellCropList/three"] = func() wire.Frame {
		return FrameExShowSellCropList(2, []ManorHeldCrop{
			{ObjectID: 268, CropID: 5073, Count: 12, Seed: A, Procure: &ManorListing{Amount: 3, Price: 31, Reward: 1}},
			{ObjectID: 270, CropID: 5100, Count: 0x7fffffff, Seed: C},
			{ObjectID: 269, CropID: 5068, Count: 1, Seed: B},
		})
	}

	templates := item.NewTable([]*item.Template{{ID: 5073, Kind: item.KindEtcItem, Slot: item.SlotNone}})
	sell := func(rows []ProcureSale) wire.Frame {
		frame, err := FrameSellListProcure(900, rows, templates)
		if err != nil {
			panic(err)
		}
		return frame
	}
	m["SellListProcure/one"] = func() wire.Frame {
		return sell([]ProcureSale{{Item: &item.Instance{ObjectID: 268, TemplateID: 5073, Count: 12}, Amount: 40}})
	}
	m["SellListProcure/none"] = func() wire.Frame { return sell(nil) }

	m["BuyListSeed/two"] = func() wire.Frame {
		return FrameBuyListSeed(1500, 1, []ManorListing{{ID: 5016, Amount: 7, Price: 30}, {ID: 9999, Amount: 1, Price: 3}})
	}
	m["BuyListSeed/filtered"] = func() wire.Frame { return FrameBuyListSeed(-5, 4, []ManorListing{{ID: 5090, Amount: 2, Price: 8}}) }
	m["BuyListSeed/none"] = func() wire.Frame { return FrameBuyListSeed(0, 3, nil) }
	m["ExSendManorList"] = func() wire.Frame { return FrameExSendManorList() }
}
