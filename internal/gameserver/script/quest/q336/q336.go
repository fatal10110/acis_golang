// Package q336 is the quest Coins of Magic: Sorint, president of the coin
// collectors' guild, sends a hunter through Bernard for the Kaldis gold
// dragon coin and makes them a guild member. Members collect coins from
// monsters, trade them up with the guild's nine collectors at a fixed rate
// or in a coin-color guessing game, raise their membership with sets of
// coins and trade coin sets for weapons and armor at Sorint.
package q336

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q336_CoinsOfMagic"

// NPCs.
const (
	pano    = 30078
	collob  = 30092
	ralford = 30165
	hagger  = 30183
	stan    = 30200
	sorint  = 30232
	lorain  = 30673
	duning  = 30688
	pageNPC = 30696
	bernard = 30702
	ferris  = 30847
)

// Monsters.
const (
	bloodyGhost             = 20960
	wailingOfSplendor1      = 21539
	wailingOfSplendor2      = 21540
	clawsOfSplendor1        = 21521
	clawsOfSplendor2        = 21522
	formor                  = 20568
	formorElder             = 20569
	darkGuard               = 20959
	hungeredCorpse          = 20954
	oelMahum                = 20161
	oelMahumWarrior         = 20575
	oelMahumWitchDoctor     = 20576
	tarlkBugbearHighWarrior = 20572
	vanorSilenosShaman      = 20685

	antelope1            = 21278
	antelope2            = 21279
	antelope3            = 21280
	buffalo1             = 21286
	buffalo2             = 21287
	buffalo3             = 21288
	doomKnight           = 20674
	doomServant          = 21006
	headlessKnight1      = 20146
	headlessKnight2      = 20280
	kookaburra1          = 21274
	kookaburra2          = 21275
	kookaburra3          = 21276
	malrukSuccubusTuren1 = 20245
	malrukSuccubusTuren2 = 20284
	royalCaveServant1    = 20240
	royalCaveServant2    = 20276
	shackle1             = 20235
	shackle2             = 20279
	timakOrk             = 20583

	bandersnatch1           = 21282
	bandersnatch2           = 21283
	bandersnatch3           = 21284
	punishmentOfSplendor1   = 21531
	punishmentOfSplendor2   = 21658
	wisdomOfSplendor        = 21526
	doomArcher              = 21008
	graveLich               = 21003
	hatarHanishee           = 20663
	lakin                   = 20604
	nihilInvader            = 20957
	torturedUndead          = 20678
	timakOrkArcher          = 20584
	timakOrkShaman          = 20587
	timakOrkSoldier         = 20585
	haritLizardmanShaman    = 20644
	haritLizardmanMatriarch = 20645
)

// Coins.
const (
	bloodMedusa        = 3472
	bloodWerewolf      = 3473
	bloodBasilisk      = 3474
	bloodDrevanul      = 3475
	bloodSuccubus      = 3476
	bloodDragon        = 3477
	belethBloodDragon  = 3478
	manakBloodWerewolf = 3479
	niaBloodMedusa     = 3480
	goldDragon         = 3481
	goldWyvern         = 3482
	goldKnight         = 3483
	goldGiant          = 3484
	goldDrake          = 3485
	goldWyrm           = 3486
	belethGoldDragon   = 3487
	manakGoldGiant     = 3488
	niaGoldWyvern      = 3489
	silverUnicorn      = 3490
	silverFairy        = 3491
	silverDryad        = 3492
	silverDragon       = 3493
	silverGolem        = 3494
	silverUndine       = 3495
	belethSilverDragon = 3496
	manakSilverDryad   = 3497
	niaSilverFairy     = 3498
)

// Quest items.
const (
	coinDiagram              = 3811
	kaldisGoldDragon         = 3812
	coinCollectorMembership1 = 3813
	coinCollectorMembership2 = 3814
	coinCollectorMembership3 = 3815
)

// Rewards.
const (
	swordOfLimit         = 132
	demonStaff           = 206
	darkScreamer         = 233
	widowMaker           = 303
	demonStockings       = 472
	moonstoneEarring     = 852
	nassenEarring        = 855
	ringOfBinding        = 886
	necklaceOfProtection = 916
	fullPlateHelmet      = 2414
	demonBoots           = 2435
	demonGloves          = 2459
)

// drop is a monster's coin and its chance.
type drop struct{ item, chance int32 }

// firstDrops are the monsters that drop the Kaldis gold dragon, by NPC id.
func firstDrops() map[int32]drop {
	return map[int32]drop{
		haritLizardmanShaman:    {kaldisGoldDragon, 630000},
		haritLizardmanMatriarch: {kaldisGoldDragon, 630000},
	}
}

// secondDrops are the monsters that drop coins for guild members, by NPC id.
func secondDrops() map[int32]drop {
	return map[int32]drop{
		// 3490 itemId - 14 npcIds
		bloodyGhost:             {silverUnicorn, 80000},
		wailingOfSplendor1:      {silverUnicorn, 210000},
		wailingOfSplendor2:      {silverUnicorn, 210000},
		clawsOfSplendor1:        {silverUnicorn, 150000},
		clawsOfSplendor2:        {silverUnicorn, 150000},
		formor:                  {silverUnicorn, 110000},
		formorElder:             {silverUnicorn, 120000},
		darkGuard:               {silverUnicorn, 150000},
		hungeredCorpse:          {silverUnicorn, 100000},
		oelMahum:                {silverUnicorn, 80000},
		oelMahumWarrior:         {silverUnicorn, 90000},
		oelMahumWitchDoctor:     {silverUnicorn, 200000},
		tarlkBugbearHighWarrior: {silverUnicorn, 80000},
		vanorSilenosShaman:      {silverUnicorn, 70000},

		// 3482 itemId - 20 npcIds
		antelope1:            {goldWyvern, 100000},
		antelope2:            {goldWyvern, 100000},
		antelope3:            {goldWyvern, 100000},
		buffalo1:             {goldWyvern, 110000},
		buffalo2:             {goldWyvern, 110000},
		buffalo3:             {goldWyvern, 110000},
		doomKnight:           {goldWyvern, 210000},
		doomServant:          {goldWyvern, 140000},
		headlessKnight1:      {goldWyvern, 80000},
		headlessKnight2:      {goldWyvern, 85000},
		kookaburra1:          {goldWyvern, 110000},
		kookaburra2:          {goldWyvern, 110000},
		kookaburra3:          {goldWyvern, 100000},
		malrukSuccubusTuren1: {goldWyvern, 100000},
		malrukSuccubusTuren2: {goldWyvern, 100000},
		royalCaveServant1:    {goldWyvern, 100000},
		royalCaveServant2:    {goldWyvern, 100000},
		shackle1:             {goldWyvern, 70000},
		shackle2:             {goldWyvern, 70000},
		timakOrk:             {goldWyvern, 80000},

		// 3472 itemId - 15 npcIds
		bandersnatch1:         {bloodMedusa, 90000},
		bandersnatch2:         {bloodMedusa, 90000},
		bandersnatch3:         {bloodMedusa, 90000},
		punishmentOfSplendor1: {bloodMedusa, 160000},
		punishmentOfSplendor2: {bloodMedusa, 160000},
		wisdomOfSplendor:      {bloodMedusa, 160000},
		doomArcher:            {bloodMedusa, 160000},
		graveLich:             {bloodMedusa, 90000},
		hatarHanishee:         {bloodMedusa, 70000},
		lakin:                 {bloodMedusa, 60000},
		nihilInvader:          {bloodMedusa, 110000},
		torturedUndead:        {bloodMedusa, 80000},
		timakOrkArcher:        {bloodMedusa, 140000},
		timakOrkShaman:        {bloodMedusa, 130000},
		timakOrkSoldier:       {bloodMedusa, 140000},
	}
}

// multiplier2nd is the fixed-rate exchange's coin multiplier by collector
// grade, one per reply 10, 11 and 12; nil for no grade.
func multiplier2nd(level int32) []int32 {
	switch level {
	case 1:
		return []int32{5, 25, 100}
	case 2:
		return []int32{5, 25, 50}
	case 3:
		return []int32{10, 50, 100}
	}
	return nil
}

// multiplier3rd is the game's coin multiplier by collector grade, one per
// reply 6, 7 and 8; nil for no grade.
func multiplier3rd(level int32) []int32 {
	switch level {
	case 1:
		return []int32{4, 8, 9}
	case 2:
		return []int32{3, 7, 9}
	case 3:
		return []int32{4, 8, 9}
	}
	return nil
}

// counts is the fixed-rate exchange's reward count, one per reply 10, 11
// and 12.
func counts() []int32 { return []int32{1, 5, 10} }

// dialogues maps a game's hidden coins to the page number that reveals them.
func dialogues() map[int32]int32 {
	return map[int32]int32{
		21: 23, 25: 24, 37: 25, 41: 26, 61: 27, 29: 28, 45: 29, 53: 30, 57: 31,
		22: 32, 26: 33, 38: 34, 42: 35, 62: 36, 30: 37, 46: 38, 54: 39, 58: 40,
		23: 41, 27: 42, 39: 43, 43: 44, 63: 45, 31: 46, 47: 47, 55: 48, 59: 49,
	}
}

// exchange1st is the grade 1 collectors' trades, by NPC and trade: two
// coins taken, one given.
func exchange1st() map[int32]map[int32][]int32 {
	return map[int32]map[int32][]int32{
		collob: {
			1: {bloodBasilisk, bloodSuccubus, bloodDragon},
			2: {silverDryad, silverUndine, silverDragon},
			3: {goldGiant, goldWyrm, goldDragon},
			4: {goldGiant, goldWyrm, belethGoldDragon},
		},
		ferris: {
			1: {bloodBasilisk, bloodSuccubus, bloodDragon},
			2: {silverDryad, silverUndine, silverDragon},
			3: {goldGiant, goldWyrm, goldDragon},
			4: {bloodBasilisk, bloodSuccubus, belethBloodDragon},
		},
		pano: {
			1: {bloodBasilisk, bloodSuccubus, bloodDragon},
			2: {silverUndine, silverDryad, silverDragon},
			3: {goldGiant, goldWyrm, goldDragon},
			4: {silverDryad, silverUndine, belethSilverDragon},
		},
	}
}

// request1st is the grade 1 collectors' game stakes, by NPC and trade.
func request1st() map[int32]map[int32][]int32 {
	return map[int32]map[int32][]int32{
		collob: {1: {bloodSuccubus}, 2: {silverUndine}, 3: {goldWyrm}, 4: {goldWyrm, goldGiant}},
		ferris: {1: {bloodBasilisk}, 2: {silverDryad}, 3: {goldGiant}, 4: {bloodBasilisk, bloodSuccubus}},
		pano:   {1: {bloodBasilisk}, 2: {silverDryad}, 3: {goldGiant}, 4: {silverDryad, silverUndine}},
	}
}

// exchange2nd is the grade 2 collectors' trades, by NPC and trade: two
// coins taken, one given.
func exchange2nd() map[int32]map[int32][]int32 {
	return map[int32]map[int32][]int32{
		ralford: {
			1: {goldDrake, goldKnight, goldWyrm},
			2: {silverGolem, silverFairy, silverDryad},
			3: {silverGolem, silverFairy, silverUndine},
			4: {bloodWerewolf, bloodDrevanul, manakBloodWerewolf},
		},
		stan: {
			1: {goldKnight, goldDrake, goldGiant},
			2: {bloodWerewolf, bloodDrevanul, bloodBasilisk},
			3: {silverFairy, silverGolem, silverDryad},
			4: {silverFairy, silverGolem, manakSilverDryad},
		},
		duning: {
			1: {goldDrake, goldKnight, goldGiant},
			2: {bloodDrevanul, bloodWerewolf, bloodSuccubus},
			3: {silverGolem, silverFairy, silverUndine},
			4: {goldKnight, goldDrake, manakGoldGiant},
		},
	}
}

// request2nd is the grade 2 collectors' game stakes, by NPC and trade.
func request2nd() map[int32]map[int32][]int32 {
	return map[int32]map[int32][]int32{
		ralford: {1: {goldDrake}, 2: {silverFairy}, 3: {silverGolem}, 4: {bloodWerewolf, bloodDrevanul}},
		stan:    {1: {goldKnight}, 2: {bloodWerewolf}, 3: {silverFairy}, 4: {silverFairy, silverGolem}},
		duning:  {1: {goldDrake}, 2: {bloodDrevanul}, 3: {silverGolem}, 4: {goldDrake, goldKnight}},
	}
}

// exchange3rd is the grade 3 collectors' trades, by NPC and trade: the
// coin taken, the coin given.
func exchange3rd() map[int32]map[int32][]int32 {
	return map[int32]map[int32][]int32{
		lorain: {
			1: {goldWyvern, goldKnight},
			2: {silverUnicorn, silverGolem},
			3: {bloodMedusa, bloodDrevanul},
			4: {goldWyvern, niaGoldWyvern},
		},
		pageNPC: {
			1: {goldWyvern, goldKnight},
			2: {silverUnicorn, silverFairy},
			3: {bloodMedusa, bloodWerewolf},
			4: {bloodMedusa, niaBloodMedusa},
		},
		hagger: {
			1: {goldWyvern, goldDrake},
			2: {silverUnicorn, silverGolem},
			3: {bloodMedusa, bloodWerewolf},
			4: {silverUnicorn, niaSilverFairy},
		},
	}
}

// request3rd is the grade 3 collectors' game stake, by NPC, for a trade
// parameter above 40, 30, 20 and the rest.
func request3rd() map[int32][]int32 {
	return map[int32][]int32{
		lorain:  {goldWyvern, bloodMedusa, silverUnicorn, goldWyvern},
		pageNPC: {bloodMedusa, bloodMedusa, silverUnicorn, goldWyvern},
		hagger:  {silverUnicorn, bloodMedusa, silverUnicorn, goldWyvern},
	}
}

// New returns the quest.
func New() script.Script {
	dying := append(slices.Sorted(maps.Keys(firstDrops())), slices.Sorted(maps.Keys(secondDrops()))...)
	return script.Script{
		Title:   "Coins of Magic",
		QuestID: 336,
		Items:   []int32{coinDiagram, kaldisGoldDragon, coinCollectorMembership1, coinCollectorMembership2, coinCollectorMembership3},
		Bind: script.Bindings{
			script.EventQuestStart: {sorint},
			script.EventTalked:     {pano, collob, ralford, hagger, stan, sorint, lorain, duning, pageNPC, bernard, ferris},
			script.EventMyDying:    dying,
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	flag := st.GetInt("flag")
	npcID := e.NPC.NpcID()
	event := e.Name
	switch {
	case strings.EqualFold(event, "30232-05.htm"):
		if !p.HasItem(coinDiagram) {
			s.GiveItems(p, coinDiagram, 1)
		}
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		st.Set("state", itoa(1))
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(event, "upgrade"):
		htmltext = upgrade(s, st, p, htmltext)
	case strings.EqualFold(event, "30232-22.htm"):
		st.SetCond(4)
	case strings.EqualFold(event, "30232-23.htm"):
		st.SetCond(5)
	case strings.HasPrefix(event, "exchange "):
		htmltext = exchangeReward(s, p, number(event, "exchange "))
	case strings.EqualFold(event, "30232-18a.htm"):
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
	case strings.EqualFold(event, "30702-03.htm") || strings.EqualFold(event, "30702-04.htm"):
		st.SetCond(2)
		st.Set("state", itoa(2))
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(event, "30702-06.htm"):
		st.SetCond(7)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(event, "reply_1"):
		htmltext = itoa(npcID) + "-02.htm"
		chooseTrade(st, npcID, 11)
	case strings.EqualFold(event, "reply_2"):
		htmltext = itoa(npcID) + "-03.htm"
		chooseTrade(st, npcID, 21)
	case strings.EqualFold(event, "reply_3"):
		htmltext = itoa(npcID) + "-04.htm"
		chooseTrade(st, npcID, 31)
	case strings.EqualFold(event, "reply_4"):
		htmltext = itoa(npcID) + "-05.htm"
		chooseTrade(st, npcID, 42)
	case strings.EqualFold(event, "reply_6") || strings.EqualFold(event, "reply_7") || strings.EqualFold(event, "reply_8"):
		htmltext = startGame(s, st, p, npcID, (number(event, "reply_")+4)%10, htmltext)
	case strings.EqualFold(event, "reply_10") || strings.EqualFold(event, "reply_11") || strings.EqualFold(event, "reply_12"):
		htmltext = exchangeCoins(s, st, p, npcID, number(event, "reply_")%10, htmltext)
	case strings.EqualFold(event, "reply_13") && flag == 1:
		st.Set("flag", itoa(16))
		htmltext = itoa(npcID) + "-14.htm"
	case strings.EqualFold(event, "reply_14") && flag == 1:
		st.Set("flag", itoa(32))
		htmltext = itoa(npcID) + "-15.htm"
	case strings.EqualFold(event, "reply_15") && flag == 1:
		st.Set("flag", itoa(48))
		htmltext = itoa(npcID) + "-16.htm"
	case strings.EqualFold(event, "reply_16"):
		st.Set("flag", itoa(flag+4))
		htmltext = itoa(npcID) + "-17.htm"
	case strings.EqualFold(event, "reply_17"):
		st.Set("flag", itoa(flag+8))
		htmltext = itoa(npcID) + "-18.htm"
	case strings.EqualFold(event, "reply_18"):
		st.Set("flag", itoa(flag+12))
		htmltext = itoa(npcID) + "-19.htm"
	case strings.EqualFold(event, "reply_19") || strings.EqualFold(event, "reply_20") || strings.EqualFold(event, "reply_21"):
		htmltext = guessLastCoin(s, st, p, npcID, flag, (number(event, "reply_")+2)%20, htmltext)
	}
	return htmltext
}

// number reads what is left of event once every prefix is removed as a
// decimal 32-bit integer; anything else panics, so the invocation aborts.
func number(event, prefix string) int32 {
	n, err := strconv.ParseInt(strings.ReplaceAll(event, prefix, ""), 10, 32)
	if err != nil {
		panic(err)
	}
	return int32(n)
}

// upgrade raises a member's grade at Sorint when the member holds the
// grade's coin set.
func upgrade(s *script.Script, st *script.QuestState, p *script.Player, htmltext string) string {
	switch {
	case p.HasItem(coinCollectorMembership3):
		if p.HasItems(bloodWerewolf, bloodDrevanul, goldKnight, goldDrake, silverFairy, silverGolem) {
			htmltext = "30232-16.htm"
			st.SetCond(9)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, coinCollectorMembership3, -1)
			s.TakeItems(p, bloodWerewolf, 1)
			s.TakeItems(p, bloodDrevanul, 1)
			s.TakeItems(p, goldKnight, 1)
			s.TakeItems(p, goldDrake, 1)
			s.TakeItems(p, silverFairy, 1)
			s.TakeItems(p, silverGolem, 1)
			s.GiveItems(p, coinCollectorMembership2, 1)
		} else {
			htmltext = "30232-13.htm"
			st.SetCond(8)
		}
	case p.HasItem(coinCollectorMembership2):
		if p.HasItems(bloodBasilisk, bloodSuccubus, goldGiant, goldWyrm, silverDryad, silverUndine) {
			htmltext = "30232-17.htm"
			st.SetCond(11)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, coinCollectorMembership2, -1)
			s.TakeItems(p, bloodBasilisk, 1)
			s.TakeItems(p, bloodSuccubus, 1)
			s.TakeItems(p, goldGiant, 1)
			s.TakeItems(p, goldWyrm, 1)
			s.TakeItems(p, silverDryad, 1)
			s.TakeItems(p, silverUndine, 1)
			s.GiveItems(p, coinCollectorMembership1, 1)
		} else {
			htmltext = "30232-14.htm"
			st.SetCond(10)
		}
	case p.HasItem(coinCollectorMembership1):
		htmltext = "30232-15.htm"
	}
	return htmltext
}

// exchangeReward trades a coin set for the reward item at Sorint.
func exchangeReward(s *script.Script, p *script.Player, item int32) string {
	switch {
	case item == 206 && p.HasItems(belethBloodDragon, silverDragon) && p.ItemCount(goldWyrm) > 12:
		s.TakeItems(p, belethBloodDragon, 1)
		s.TakeItems(p, silverDragon, 1)
		s.TakeItems(p, goldWyrm, 13)
		s.GiveItems(p, demonStaff, 1)
		s.PlaySound(p, script.SoundMiddle)
		return "30232-24a.htm"
	case item == 233 && p.HasItems(bloodDragon, goldGiant, belethGoldDragon, silverDryad):
		s.TakeItems(p, bloodDragon, 1)
		s.TakeItems(p, goldGiant, 1)
		s.TakeItems(p, belethGoldDragon, 1)
		s.TakeItems(p, silverDryad, 1)
		s.GiveItems(p, darkScreamer, 1)
		s.PlaySound(p, script.SoundMiddle)
		return "30232-24b.htm"
	case item == 303 && p.ItemCount(bloodBasilisk) > 1 && p.HasItems(bloodSuccubus, goldDragon, belethSilverDragon):
		s.TakeItems(p, bloodBasilisk, 2)
		s.TakeItems(p, bloodSuccubus, 1)
		s.TakeItems(p, goldDragon, 1)
		s.TakeItems(p, belethSilverDragon, 1)
		s.GiveItems(p, widowMaker, 1)
		s.PlaySound(p, script.SoundMiddle)
		return "30232-24c.htm"
	case item == 132 && p.HasItems(bloodDragon, goldDragon, silverDragon, silverUndine):
		s.TakeItems(p, bloodDragon, 1)
		s.TakeItems(p, goldDragon, 1)
		s.TakeItems(p, silverDragon, 1)
		s.TakeItems(p, silverUndine, 1)
		s.GiveItems(p, swordOfLimit, 1)
		s.PlaySound(p, script.SoundMiddle)
		return "30232-24d.htm"
	case item == 2435 && p.HasItem(manakGoldGiant):
		s.TakeItems(p, manakGoldGiant, 1)
		s.GiveItems(p, demonBoots, 1)
		s.PlaySound(p, script.SoundMiddle)
		return "30232-24e.htm"
	case item == 472 && p.HasItems(silverDryad, manakSilverDryad):
		s.TakeItems(p, silverDryad, 1)
		s.TakeItems(p, manakSilverDryad, 1)
		s.GiveItems(p, demonStockings, 1)
		s.PlaySound(p, script.SoundMiddle)
		return "30232-24f.htm"
	case item == 2459 && p.HasItem(manakGoldGiant):
		s.TakeItems(p, manakGoldGiant, 1)
		s.GiveItems(p, demonGloves, 1)
		s.PlaySound(p, script.SoundMiddle)
		return "30232-24g.htm"
	case item == 2414 && p.HasItems(manakBloodWerewolf, goldGiant, goldWyrm):
		s.TakeItems(p, manakBloodWerewolf, 1)
		s.TakeItems(p, goldGiant, 1)
		s.TakeItems(p, goldWyrm, 1)
		s.GiveItems(p, fullPlateHelmet, 1)
		s.PlaySound(p, script.SoundMiddle)
		return "30232-24h.htm"
	case item == 852 && p.ItemCount(niaBloodMedusa) > 1 && p.ItemCount(goldDrake) > 1 && p.ItemCount(bloodDrevanul) > 1 && p.ItemCount(goldKnight) > 2:
		s.TakeItems(p, niaBloodMedusa, 2)
		s.TakeItems(p, goldDrake, 2)
		s.TakeItems(p, bloodDrevanul, 2)
		s.TakeItems(p, goldKnight, 3)
		s.GiveItems(p, moonstoneEarring, 1)
		s.PlaySound(p, script.SoundMiddle)
		// The datapack ships this page as 30232-24I.htm; page names are
		// case-sensitive, so the client gets the missing-page notice.
		return "30232-24i.htm"
	case item == 855 && p.ItemCount(niaBloodMedusa) > 6 && p.ItemCount(goldKnight) > 4 && p.ItemCount(bloodDrevanul) > 4 && p.ItemCount(silverGolem) > 4:
		s.TakeItems(p, niaBloodMedusa, 7)
		s.TakeItems(p, goldKnight, 5)
		s.TakeItems(p, bloodDrevanul, 5)
		s.TakeItems(p, silverGolem, 5)
		s.GiveItems(p, nassenEarring, 1)
		s.PlaySound(p, script.SoundMiddle)
		return "30232-24j.htm"
	case item == 886 && p.ItemCount(niaGoldWyvern) > 4 && p.ItemCount(silverGolem) > 3 && p.ItemCount(goldDrake) > 3 && p.ItemCount(bloodDrevanul) > 3:
		s.TakeItems(p, niaGoldWyvern, 5)
		s.TakeItems(p, silverGolem, 4)
		s.TakeItems(p, goldDrake, 4)
		s.TakeItems(p, bloodDrevanul, 4)
		s.GiveItems(p, ringOfBinding, 1)
		s.PlaySound(p, script.SoundMiddle)
		return "30232-24k.htm"
	case item == 916 && p.ItemCount(niaSilverFairy) > 4 && p.ItemCount(silverFairy) > 2 && p.ItemCount(goldKnight) > 2 && p.ItemCount(bloodDrevanul) > 2:
		s.TakeItems(p, niaSilverFairy, 5)
		s.TakeItems(p, silverFairy, 3)
		s.TakeItems(p, goldKnight, 3)
		s.TakeItems(p, bloodDrevanul, 3)
		s.GiveItems(p, necklaceOfProtection, 1)
		s.PlaySound(p, script.SoundMiddle)
		return "30232-24l.htm"
	}
	return "30232-24.htm"
}

// chooseTrade records the trade a collector's reply picked and the
// collector's grade.
func chooseTrade(st *script.QuestState, npcID, param2 int32) {
	st.Set("param2", itoa(param2))
	st.Set("level", itoa(exchangerLevel(npcID)))
}

// startGame takes the stake of the picked trade's game and hides three
// coins; index is the game picked (reply 6, 7 or 8).
func startGame(s *script.Script, st *script.QuestState, p *script.Player, npcID, index int32, htmltext string) string {
	param2 := st.GetInt("param2")
	level := st.GetInt("level")
	point := index + 1
	multiplier := multiplier3rd(level)[index]
	page := int32(11)
	trigger := false

	switch level {
	case 3:
		if npcID == pageNPC {
			if index == 1 {
				page = 12
			} else if index == 2 {
				page = 13
			}
		}
		request := request3rd()[npcID]
		var itemID int32
		switch {
		case param2 > 40:
			itemID = request[0]
		case param2 > 30:
			itemID = request[1]
		case param2 > 20:
			itemID = request[2]
		default:
			itemID = request[3]
		}
		requere := multiplier * (param2 % 10)
		if p.ItemCount(itemID) >= requere {
			trigger = true
			s.TakeItems(p, itemID, requere)
		} else {
			htmltext = itoa(npcID) + "-10.htm"
		}
	case 2, 1:
		var table map[int32]map[int32][]int32
		if level == 2 {
			table = request2nd()
		} else {
			table = request1st()
		}
		items, ok := table[npcID][param2/10]
		if !ok {
			panic("q336: no stake for the trade")
		}
		requere := multiplier * (param2 % 10)
		if param2 > 40 {
			requere = multiplier
		}
		exists := true
		for _, item := range items {
			if p.ItemCount(item) < requere {
				exists = false
			}
		}
		if exists {
			for _, item := range items {
				s.TakeItems(p, item, requere)
			}
			trigger = true
		} else {
			htmltext = itoa(npcID) + "-10.htm"
		}
	}

	if trigger {
		param1 := int32(s.Rnd(3) + 1)
		param1 += int32(s.Rnd(3)+1) * 4
		param1 += int32(s.Rnd(3)+1) * 16
		st.Set("param1", itoa(param1))
		st.Set("flag", itoa(1))
		st.Set("point", itoa(point))
		htmltext = itoa(npcID) + "-" + itoa(page) + ".htm"
	}
	return htmltext
}

// exchangeCoins runs the picked trade at the fixed rate; index is the rate
// picked (reply 10, 11 or 12).
func exchangeCoins(s *script.Script, st *script.QuestState, p *script.Player, npcID, index int32, htmltext string) string {
	param2 := st.GetInt("param2")
	level := st.GetInt("level")
	multiplier := multiplier2nd(level)[index]
	count := counts()[index]
	trigger := param2 / 10
	if trigger <= 0 {
		return htmltext
	}
	neededAmount := multiplier * (param2 % 10)
	switch level {
	case 3:
		data := exchange3rd()[npcID][trigger]
		if p.ItemCount(data[0]) >= neededAmount {
			htmltext = itoa(npcID) + "-07.htm"
			s.TakeItems(p, data[0], neededAmount)
			s.GiveItems(p, data[1], count)
			s.PlaySound(p, script.SoundMiddle)
		} else {
			htmltext = itoa(npcID) + "-10.htm"
		}
	case 2, 1:
		var data []int32
		if level == 2 {
			data = exchange2nd()[npcID][trigger]
		} else {
			data = exchange1st()[npcID][trigger]
		}
		if p.ItemCount(data[0]) >= neededAmount && p.ItemCount(data[1]) >= neededAmount {
			htmltext = itoa(npcID) + "-07.htm"
			s.TakeItems(p, data[0], neededAmount)
			s.TakeItems(p, data[1], neededAmount)
			s.GiveItems(p, data[2], count)
			s.PlaySound(p, script.SoundMiddle)
		} else {
			htmltext = itoa(npcID) + "-10.htm"
		}
	}
	return htmltext
}

// guessLastCoin adds the last coin's guess to the game's guesses and
// answers: the trade's coin when every coin is right, the hidden coins
// when no guess is left, else how many coins are right.
func guessLastCoin(s *script.Script, st *script.QuestState, p *script.Player, npcID, flag, multiplier int32, htmltext string) string {
	level := st.GetInt("level")
	flag += multiplier
	point := st.GetInt("point")
	param1 := st.GetInt("param1")
	param2 := st.GetInt("param2")
	param3 := int32(0)

	switch {
	case param1 == flag && point >= 0:
		point = 0
		param1 = 0
		switch level {
		case 3:
			s.GiveItems(p, exchange3rd()[npcID][param2/10][1], 1)
		case 2:
			s.GiveItems(p, exchange2nd()[npcID][param2/10][2], 1)
		case 1:
			s.GiveItems(p, exchange1st()[npcID][param2/10][2], 1)
		}
		htmltext = itoa(npcID) + "-20.htm"
	case point == 0:
		htmltext = itoa(npcID) + "-" + dialoguePage(param1) + ".htm"
		param1 = 0
	default:
		i0 := param1 % 4
		i1 := param1 / 4
		i2 := i1 / 4
		i1 %= 4
		i3 := flag % 4
		i4 := flag / 4
		i5 := i4 / 4
		i4 %= 4
		if i0 == i3 {
			param3++
		}
		if i1 == i4 {
			param3++
		}
		if i2 == i5 {
			param3++
		}
		switch param3 {
		case 1:
			htmltext = itoa(npcID) + "-50.htm"
		case 2:
			htmltext = itoa(npcID) + "-51.htm"
		case 0:
			htmltext = itoa(npcID) + "-52.htm"
		}
		flag = 1
		point--
	}

	st.Set("flag", itoa(flag))
	st.Set("point", itoa(point))
	st.Set("param1", itoa(param1))
	st.Set("param2", itoa(param2))
	return htmltext
}

// dialoguePage is the page number that reveals the hidden coins param1;
// coins with no page give "null", so the page is missing.
func dialoguePage(param1 int32) string {
	if n, ok := dialogues()[param1]; ok {
		return itoa(n)
	}
	return "null"
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		if p.Level() < 40 {
			htmltext = "30232-01.htm"
		} else {
			htmltext = "30232-02.htm"
		}
	case questlog.StatusStarted:
		state := st.GetInt("state")
		npcID := e.NPC.NpcID()
		switch npcID {
		case sorint:
			htmltext = talkSorint(s, st, p, state, htmltext)
		case bernard:
			switch state {
			case 1:
				htmltext = "30702-01.htm"
			case 2:
				htmltext = "30702-02a.htm"
			default:
				htmltext = "30702-05.htm"
			}
		case lorain, pageNPC, hagger:
			if isCollector(p, 3) {
				htmltext = itoa(npcID) + "-01.htm"
				unset(st)
			}
		case ralford, stan, duning:
			if isCollector(p, 2) {
				htmltext = itoa(npcID) + "-01.htm"
				unset(st)
			} else if p.HasItem(coinCollectorMembership3) {
				htmltext = itoa(npcID) + "-54.htm"
			}
		case collob, ferris, pano:
			if isCollector(p, 1) {
				htmltext = itoa(npcID) + "-01.htm"
				unset(st)
			} else if p.HasItem(coinCollectorMembership3) || p.HasItem(coinCollectorMembership2) {
				htmltext = itoa(npcID) + "-54.htm"
			}
		}
	}
	return htmltext
}

func talkSorint(s *script.Script, st *script.QuestState, p *script.Player, state int32, htmltext string) string {
	switch state {
	case 1, 2:
		if !p.HasItem(kaldisGoldDragon) {
			return "30232-06.htm"
		}
		st.SetCond(4)
		st.Set("state", itoa(3))
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, coinDiagram, -1)
		s.TakeItems(p, kaldisGoldDragon, -1)
		s.GiveItems(p, coinCollectorMembership3, 1)
		return "30232-07.htm"
	case 3:
		switch {
		case p.HasItem(coinCollectorMembership3):
			return "30232-10.htm"
		case p.HasItem(coinCollectorMembership2):
			return "30232-11.htm"
		case p.HasItem(coinCollectorMembership1):
			return "30232-12.htm"
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if p == nil {
		return
	}
	npcID := e.NPC.NpcID()
	if d, ok := firstDrops()[npcID]; ok {
		st := s.RandomPartyMember(p, e.NPC, script.ByCond(2))
		if st == nil {
			return
		}
		if s.DropItems(st.Player(), d.item, 1, 1, d.chance) {
			st.SetCond(3)
		}
	} else if d, ok := secondDrops()[npcID]; ok {
		st := s.RandomPartyMember(p, e.NPC, script.ByVar("state", "3"))
		if st == nil {
			return
		}
		s.DropItems(st.Player(), d.item, 1, 0, d.chance)
	}
}

// isCollector reports whether p holds a membership of grade level or a
// better one (grade 1 is the best).
func isCollector(p *script.Player, level int32) bool {
	switch level {
	case 1:
		return p.HasItem(coinCollectorMembership1)
	case 2:
		return p.HasAtLeastOneItem(coinCollectorMembership1, coinCollectorMembership2)
	default:
		return p.HasAtLeastOneItem(coinCollectorMembership1, coinCollectorMembership2, coinCollectorMembership3)
	}
}

// unset clears the trade and game variables.
func unset(st *script.QuestState) {
	st.Unset("flag")
	st.Unset("level")
	st.Unset("point")
	st.Unset("param1")
	st.Unset("param2")
}

// exchangerLevel is the collector grade of an NPC, 0 for a non-collector.
func exchangerLevel(npcID int32) int32 {
	switch npcID {
	case lorain, pageNPC, hagger:
		return 3
	case ralford, stan, duning:
		return 2
	case collob, ferris, pano:
		return 1
	}
	return 0
}

func itoa(n int32) string { return strconv.Itoa(int(n)) }
