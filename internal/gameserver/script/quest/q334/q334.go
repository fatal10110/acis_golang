// Package q334 is the quest The Wishing Potion: the alchemist Matild has the
// player gather the ingredients of a wishing potion, then trades it for one
// of four wishes, some of which call out fairies, demons and a chest.
package q334

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q334_TheWishingPotion"

// NPCs.
const (
	torai           = 30557
	alchemistMatild = 30738
	fairyRupina     = 30742
	chestOfWisdom   = 30743
)

// Monsters.
const (
	whisperingWind      = 20078
	antSoldier          = 20087
	antWarriorCaptain   = 20088
	silenos             = 20168
	tyrant              = 20192
	tyrantKingpin       = 20193
	amberBasilisk       = 20199
	horrorMistRipper    = 20227
	turakBugbear        = 20248
	turakBugbearWarrior = 20249
	glassJaguar         = 20250
	grima               = 27135
	succubusOfSeduction = 27136
	greatDemonKing      = 27138
	secretKeeperTree    = 27139
	alexandroSanches    = 27153
	bonaparterius       = 27154
	ramsebalius         = 27155
)

// Quest items.
const (
	alchemyText         = 3678
	secretBook          = 3679
	firstPotionRecipe   = 3680
	secondPotionRecipe  = 3681
	matildOrb           = 3682
	forbiddenLoveScroll = 3683
	amberScale          = 3684
	windSoulstone       = 3685
	glassEye            = 3686
	horrorEctoplasm     = 3687
	silenosHorn         = 3688
	antSoldierAphid     = 3689
	tyrantsChitin       = 3690
	bugbearBlood        = 3691
)

// Rewards.
const (
	adena                     = 57
	necklaceOfGrace           = 931
	wishPotion                = 3467
	ancientCrown              = 3468
	certificateOfRoyalty      = 3469
	heartOfPaagrio            = 3943
	musicalScoreThemeOfLove   = 4408
	musicalScoreThemeOfBattle = 4409
)

// The wishes' say lines.
const (
	sayDemonKingFarewell1 = 33409
	sayDemonKingFarewell2 = 33410
	sayFarewell           = 33411
	sayRamsebaliusDies    = 33412
	sayBonaparteriusDies  = 33413
	sayAlexandroDies      = 33414
	sayWishFirst          = 33415
	sayWishSecond         = 33416
	sayWishThird          = 33417
	sayDemonKingSpawn     = 33418
	sayRupinaSpawn        = 33420
	sayChestSpawn         = 33421
	sayGrimaSpawn         = 33422
	saySuccubusSpawn      = 33423
	sayAlexandroSpawn     = 33424
	sayBonaparteriusSpawn = 33425
	sayRamsebaliusSpawn   = 33426
)

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "The Wishing Potion",
		QuestID: 334,
		Items: []int32{
			alchemyText, secretBook, firstPotionRecipe, secondPotionRecipe, matildOrb, forbiddenLoveScroll,
			amberScale, windSoulstone, antSoldierAphid, glassEye, horrorEctoplasm, silenosHorn,
			bugbearBlood, tyrantsChitin,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {alchemistMatild},
			script.EventTalked:     {torai, alchemistMatild, fairyRupina, chestOfWisdom},
			script.EventCreated: {
				grima, succubusOfSeduction, greatDemonKing, alexandroSanches, bonaparterius, ramsebalius,
				fairyRupina, chestOfWisdom,
			},
			script.EventDecayed: {greatDemonKing, alexandroSanches, bonaparterius, ramsebalius},
			script.EventMyDying: {
				whisperingWind, antSoldier, antWarriorCaptain, silenos, tyrant, tyrantKingpin, amberBasilisk,
				horrorMistRipper, turakBugbear, turakBugbearWarrior, glassJaguar, grima, succubusOfSeduction,
				greatDemonKing, secretKeeperTree, alexandroSanches, bonaparterius, ramsebalius,
			},
		},
		Hooks: script.Hooks{
			OnEvent: onAdvEvent, OnTalk: onTalk, OnTimer: onTimer,
			OnCreated: onCreated, OnDecayed: onDecayed, OnMyDying: onMyDying,
		},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "30738-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, alchemyText, 1)
	case strings.EqualFold(e.Name, "30738-07.htm"):
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, alchemyText, -1)
		s.TakeItems(p, secretBook, -1)
		s.GiveItems(p, firstPotionRecipe, 1)
		s.GiveItems(p, secondPotionRecipe, 1)
	case strings.EqualFold(e.Name, "30738-11.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundItemGet)
		s.TakeItems(p, firstPotionRecipe, -1)
		s.TakeItems(p, secondPotionRecipe, -1)
		s.TakeItems(p, amberScale, -1)
		s.TakeItems(p, windSoulstone, -1)
		s.TakeItems(p, glassEye, -1)
		s.TakeItems(p, horrorEctoplasm, -1)
		s.TakeItems(p, silenosHorn, -1)
		s.TakeItems(p, antSoldierAphid, -1)
		s.TakeItems(p, tyrantsChitin, -1)
		s.TakeItems(p, bugbearBlood, -1)
		s.GiveItems(p, wishPotion, 1)
		if !p.HasItem(matildOrb) {
			s.GiveItems(p, matildOrb, 1)
		}
	case strings.EqualFold(e.Name, "30738-14.htm"):
		if p.HasItem(wishPotion) {
			htmltext = "30738-13.htm"
			if st.GetInt("wish") != 1 {
				setInt(st, "wish", 0)
			}
		}
	case strings.EqualFold(e.Name, "30738-15a.htm"):
		if !p.HasItem(wishPotion) {
			htmltext = "30738-15.htm"
			s.GiveItems(p, firstPotionRecipe, 1)
			s.GiveItems(p, secondPotionRecipe, 1)
		}
	case strings.EqualFold(e.Name, "30738-16.htm"):
		exchangeWishPotion(s, st, e, 1)
	case strings.EqualFold(e.Name, "30738-17.htm"):
		exchangeWishPotion(s, st, e, 2)
	case strings.EqualFold(e.Name, "30738-18.htm"):
		exchangeWishPotion(s, st, e, 3)
	case strings.EqualFold(e.Name, "30738-19.htm"):
		exchangeWishPotion(s, st, e, 4)
	}
	return htmltext
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
		if p.Level() < 30 {
			htmltext = "30738-01.htm"
		} else {
			htmltext = "30738-02.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case alchemistMatild:
			switch {
			case p.HasItem(alchemyText):
				if p.HasItem(secretBook) {
					htmltext = "30738-06.htm"
				} else {
					htmltext = "30738-05.htm"
				}
			case p.HasItems(firstPotionRecipe, secondPotionRecipe):
				if p.HasItems(amberScale, windSoulstone, glassEye, horrorEctoplasm, silenosHorn, antSoldierAphid, tyrantsChitin, bugbearBlood) {
					htmltext = "30738-09.htm"
				} else {
					htmltext = "30738-08.htm"
				}
			case p.HasItem(matildOrb):
				htmltext = "30738-12.htm"
			}
		case fairyRupina:
			if st.GetInt("wish") == 1 && st.GetInt("choice") == 0 {
				if s.Rnd(100) < 4 {
					htmltext = "30742-01.htm"
					s.GiveItems(p, necklaceOfGrace, 1)
				} else {
					htmltext = "30742-02.htm"
					parts := demonSetPartsRewardList()
					s.GiveItems(p, parts[s.Rnd(len(parts))], 1)
				}
				setInt(st, "wish", 0)
				s.PlaySound(p, script.SoundItemGet)
				e.NPC.DeleteMe()
			}
		case torai:
			if p.HasItem(forbiddenLoveScroll) {
				htmltext = "30557-01.htm"
				s.PlaySound(p, script.SoundItemGet)
				s.TakeItems(p, forbiddenLoveScroll, 1)
				s.GiveItems(p, adena, 500000)
			}
		case chestOfWisdom:
			if st.GetInt("wish") == 4 {
				rand := s.Rnd(10)
				switch {
				case rand < 2:
					htmltext = "30743-01.htm"
				case rand < 4:
					htmltext = "30743-02.htm"
				case rand < 6:
					htmltext = "30743-03.htm"
				case rand == 6:
					htmltext = "30743-04.htm"
				case rand < 9:
					htmltext = "30743-05.htm"
				default:
					htmltext = "30743-06.htm"
				}
				s.PlaySound(p, script.SoundItemGet)
				giveSpellbooks(s, p)
				s.GiveItems(p, musicalScoreThemeOfLove, 1)
				s.GiveItems(p, musicalScoreThemeOfBattle, 1)
				setInt(st, "wish", 0)
				e.NPC.DeleteMe()
			}
		}
	}
	return htmltext
}

// onTimer plays out the three lines Matild says over a wish, then the wish
// itself: an item, adena, or a fairy, demons or a chest at the player's feet.
func onTimer(s *script.Script, e script.Timer) string {
	switch {
	case e.Name == "timer_1":
		e.NPC.Say(script.NpcString(sayWishFirst))
		s.StartTimer("timer_2", e.NPC, e.Player, 4000*time.Millisecond)
	case e.Name == "timer_2":
		e.NPC.Say(script.NpcString(sayWishSecond))
		s.StartTimer("timer_3", e.NPC, e.Player, 4000*time.Millisecond)
	case strings.EqualFold(e.Name, "timer_3"):
		e.NPC.Say(script.NpcString(sayWishThird))
		st := s.QuestState(e.Player, questName)
		if st == nil {
			return ""
		}
		p := e.Player
		wish := st.GetInt("wish")
		var choice int
		if wish == 1 {
			choice = s.Rnd(2)
		} else {
			choice = s.Rnd(3)
		}
		spawner := s.Spawner()
		switch choice {
		case 0:
			switch wish {
			case 1:
				setInt(st, "choice", 0)
				spawner.AddSpawn(fairyRupina, p, true, 120000*time.Millisecond)
			case 2:
				setInt(st, "choice", 0)
				for range 3 {
					spawner.AddSpawn(grima, p, true, 200000*time.Millisecond)
				}
			case 3:
				setInt(st, "wish", 0)
				setInt(st, "choice", 0)
				s.PlaySound(p, script.SoundItemGet)
				s.GiveItems(p, certificateOfRoyalty, 1)
			case 4:
				setInt(st, "wish", 0)
				setInt(st, "choice", 0)
				s.PlaySound(p, script.SoundItemGet)
				giveSpellbooks(s, p)
			}
		case 1:
			switch wish {
			case 1:
				setInt(st, "choice", 0)
				for range 3 {
					spawner.AddSpawn(succubusOfSeduction, p, true, 200000*time.Millisecond)
				}
			case 2:
				setInt(st, "wish", 0)
				setInt(st, "choice", 0)
				s.GiveItems(p, adena, 10000)
			case 3:
				setInt(st, "choice", 0)
				spawner.AddSpawn(alexandroSanches, p, true, 200000*time.Millisecond)
			case 4:
				setInt(st, "choice", 1)
				spawner.AddSpawn(chestOfWisdom, p, true, 120000*time.Millisecond)
			}
		case 2:
			switch wish {
			case 2:
				setInt(st, "wish", 0)
				setInt(st, "choice", 0)
				s.GiveItems(p, adena, 10000)
			case 3:
				setInt(st, "wish", 0)
				setInt(st, "choice", 0)
				s.PlaySound(p, script.SoundItemGet)
				s.GiveItems(p, ancientCrown, 1)
			case 4:
				setInt(st, "choice", 2)
				spawner.AddSpawn(chestOfWisdom, p, true, 120000*time.Millisecond)
			}
		}
		setInt(st, "exchange", 0)
	}
	return ""
}

// onCreated has each NPC the wishes call out announce itself.
func onCreated(_ *script.Script, e script.Created) {
	switch e.NPC.NpcID() {
	case fairyRupina:
		e.NPC.Say(script.NpcString(sayRupinaSpawn))
	case grima:
		e.NPC.Say(script.NpcString(sayGrimaSpawn))
	case succubusOfSeduction:
		e.NPC.Say(script.NpcString(saySuccubusSpawn))
	case alexandroSanches:
		e.NPC.Say(script.NpcString(sayAlexandroSpawn))
	case bonaparterius:
		e.NPC.Say(script.NpcString(sayBonaparteriusSpawn))
	case ramsebalius:
		e.NPC.Say(script.NpcString(sayRamsebaliusSpawn))
	case greatDemonKing:
		e.NPC.Say(script.NpcString(sayDemonKingSpawn))
	case chestOfWisdom:
		e.NPC.Say(script.NpcString(sayChestSpawn))
	}
}

// onDecayed has a demon that leaves alive, its time up, say goodbye.
func onDecayed(_ *script.Script, e script.Decayed) {
	if e.NPC.Dead() {
		return
	}
	switch e.NPC.NpcID() {
	case alexandroSanches, bonaparterius, ramsebalius:
		e.NPC.Say(script.NpcString(sayFarewell))
	case greatDemonKing:
		e.NPC.Say(script.NpcString(sayDemonKingFarewell1))
		e.NPC.Say(script.NpcString(sayDemonKingFarewell2))
	}
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	switch e.NPC.NpcID() {
	case secretKeeperTree:
		if st.Cond() == 1 {
			st.SetCond(2)
			s.DropItemsAlways(p, secretBook, 1, 1)
		}
	case amberBasilisk:
		checkDropAndReward(s, p, st, st.Cond(), amberScale)
	case antSoldier, antWarriorCaptain:
		checkDropAndReward(s, p, st, st.Cond(), antSoldierAphid)
	case glassJaguar:
		checkDropAndReward(s, p, st, st.Cond(), glassEye)
	case horrorMistRipper:
		checkDropAndReward(s, p, st, st.Cond(), horrorEctoplasm)
	case silenos:
		checkDropAndReward(s, p, st, st.Cond(), silenosHorn)
	case turakBugbear, turakBugbearWarrior:
		checkDropAndReward(s, p, st, st.Cond(), bugbearBlood)
	case tyrant, tyrantKingpin:
		checkDropAndReward(s, p, st, st.Cond(), tyrantsChitin)
	case whisperingWind:
		checkDropAndReward(s, p, st, st.Cond(), windSoulstone)
	case succubusOfSeduction:
		if st.Cond() == 5 && !p.HasItem(forbiddenLoveScroll) && st.GetInt("wish") == 1 {
			if s.DropItems(p, forbiddenLoveScroll, 1, 0, 28000) {
				setInt(st, "wish", 0)
			}
		}
	case grima:
		if st.Cond() == 5 && st.GetInt("wish") == 2 && s.Rnd(1000) < 33 {
			setInt(st, "wish", 0)
			s.PlaySound(p, script.SoundItemGet)
			if s.Rnd(1000) == 0 {
				s.GiveItems(p, adena, 100000000)
			} else {
				s.GiveItems(p, adena, 900000)
			}
		}
	case alexandroSanches:
		checkRewardOrSpawn(s, p, e.NPC, bonaparterius, sayAlexandroDies)
	case bonaparterius:
		checkRewardOrSpawn(s, p, e.NPC, ramsebalius, sayBonaparteriusDies)
	case ramsebalius:
		checkRewardOrSpawn(s, p, e.NPC, greatDemonKing, sayRamsebaliusDies)
	case greatDemonKing:
		setInt(st, "wish", 0)
		s.PlaySound(p, script.SoundItemGet)
		s.GiveItems(p, adena, 1406956)
	}
}

// checkDropAndReward drops itemID while the player gathers the ingredients
// of the potion, and moves to cond 4 once the last one is in.
func checkDropAndReward(s *script.Script, p *script.Player, st *script.QuestState, cond, itemID int32) {
	if (cond == 3 || (cond == 5 && p.HasItem(matildOrb))) && p.HasItems(firstPotionRecipe, secondPotionRecipe) && !p.HasItem(itemID) {
		if isLastIngredient(p, itemID) {
			if s.DropItems(p, itemID, 1, 1, 100000) {
				st.SetCond(4)
			}
		} else {
			s.DropItems(p, itemID, 1, 0, 100000)
		}
	}
}

// checkRewardOrSpawn has a demon of the third wish say its last word, then
// either call out the next one or give a piece of the demon set.
func checkRewardOrSpawn(s *script.Script, p *script.Player, npc *script.NPC, npcID, message int32) {
	st := s.CheckPlayerState(p, npc, questlog.StatusStarted)
	if st == nil {
		return
	}
	if st.Cond() == 5 && st.GetInt("wish") == 3 {
		npc.Say(script.NpcString(message))
		if s.RndBool() {
			s.Spawner().AddSpawn(npcID, p, true, 200000*time.Millisecond)
		} else {
			setInt(st, "wish", 0)
			s.PlaySound(p, script.SoundItemGet)
			parts := demonSetPartsRewardList()
			s.GiveItems(p, parts[s.Rnd(len(parts))], 1)
		}
	}
}

// isLastIngredient reports whether every ingredient but current is in the
// bag.
func isLastIngredient(p *script.Player, current int32) bool {
	for _, item := range []int32{amberScale, windSoulstone, glassEye, horrorEctoplasm, silenosHorn, antSoldierAphid, tyrantsChitin, bugbearBlood} {
		if current != item && !p.HasItem(item) {
			return false
		}
	}
	return true
}

// giveSpellbooks gives one item of each of the three reward lists, and a
// heart of Paagrio one time in three.
func giveSpellbooks(s *script.Script, p *script.Player) {
	list1, list2, list3 := rewardList1(), rewardList2(), rewardList3()
	s.GiveItems(p, list1[s.Rnd(len(list1))], 1)
	s.GiveItems(p, list2[s.Rnd(len(list2))], 1)
	s.GiveItems(p, list3[s.Rnd(len(list3))], 1)
	if s.Rnd(3) == 0 {
		s.GiveItems(p, heartOfPaagrio, 1)
	}
}

// exchangeWishPotion trades the wish potion for wish and starts the lines
// Matild says over it. It answers the page that tells why it did not: no
// potion, or an earlier wish still pending. Its caller shows Matild's page
// for the event whatever it answers.
func exchangeWishPotion(s *script.Script, st *script.QuestState, e script.Event, wish int32) string {
	if !e.Player.HasItem(wishPotion) {
		return "30738-14.htm"
	}
	if st.GetInt("exchange") != 0 {
		return "30738-20.htm"
	}
	s.TakeItems(e.Player, wishPotion, 1)
	setInt(st, "wish", wish)
	setInt(st, "exchange", wish)
	s.StartTimer("timer_1", e.NPC, e.Player, 3000*time.Millisecond)
	return e.Name
}

// setInt sets the quest variable key to the number n.
func setInt(st *script.QuestState, key string, n int32) {
	st.Set(key, strconv.Itoa(int(n)))
}

// Items the wishes give: demon set patterns, skill books and spellbooks.
const (
	demonTunicFabric       = 1979
	demonHosePattern       = 1980
	demonBootsFabric       = 2952
	demonGlovesFabric      = 2953
	summonStormCubic       = 3039
	summonPhantomCubic     = 3041
	summonLifeCubic        = 3045
	summonReanimatedMan    = 3057
	summonCorruptedMan     = 3058
	corpseBurst            = 3059
	forget                 = 3060
	curseDiscord           = 3061
	curseFear              = 3062
	anchor                 = 3063
	silence                = 3064
	deathSpike             = 3065
	curseDeathLink         = 3066
	vampiricClaw           = 3067
	vitalize               = 3068
	repose                 = 3069
	holdUndead             = 3070
	requiem                = 3071
	purify                 = 3072
	mightOfHeaven          = 3073
	surrenderToWind        = 3074
	blazingCircle          = 3075
	prominence             = 3076
	blazingSkin            = 3077
	decay                  = 3078
	cancel                 = 3079
	sleepingCloud          = 3080
	auraFlare              = 3081
	surrenderToWater       = 3082
	frostWall              = 3083
	freezingShackle        = 3084
	hydroBlast             = 3085
	frostBolt              = 3086
	iceDagger              = 3087
	freezingSkin           = 3088
	tempest                = 3089
	hurricane              = 3090
	servitorMagicShield    = 3091
	servitorPhysicalShield = 3092
	servitorHaste          = 3093
	invigor                = 3094
	magicBarrier           = 3095
	blessedBody            = 3096
	blessedSoul            = 3097
	returnSkill            = 3098
	haste                  = 3099
	guidance               = 3100
	deathWhisper           = 3101
	blessShield            = 3102
	paagrioWisdom          = 3103
	paagrioGlory           = 3104
	sealOfWinter           = 3105
	sealOfFlame            = 3106
	sealOfGloom            = 3107
	sealOfMirage           = 3108
	sealOfSilence          = 3109
	sealOfScourge          = 3110
	sealOfSuspension       = 3111
	paagrioVision          = 3112
	paagrioProtection      = 3113
	stealEssence           = 3114
	freezingFlame          = 3115
	chantOfFury            = 3116
	chantOfEvasion         = 3117
	chantOfRage            = 3118
	greaterHeal            = 3429
	greaterBattleHeal      = 3430
	greaterGroupHeal       = 3431
	massRessurection       = 3941
	partyReturn            = 3942
	restoreLife            = 4200
	resistShock            = 4201
	paagrioTack            = 4204
	paagrioRage            = 4205
	transferPain           = 4206
	manaRegeneration       = 4207
	curseGloom             = 4208
	solarFlare             = 4907
	shadowFlare            = 4909
	curseDisease           = 4911
	benediction            = 4912
	wordOfFear             = 4913
	serenadeOfEva          = 4914
	auraBolt               = 4917
	summonKaiTheCat        = 4918
	summonMerrowTheUnicorn = 4919
	summonSoulless         = 4920
	summonBindingCubic     = 4922
	summonAquaCubic        = 4923
	summonSparkCubic       = 4924
	paagrioHaste           = 4925
	soulGuard              = 4926
	chantOfRevenge         = 4927
	seedOfFire             = 4928
	seedOfWater            = 4929
	seedOfWind             = 4930
	auraSymphony           = 4931
	inferno                = 4932
	blizzard               = 4933
	demonWind              = 4934
	elementalAssault       = 5013
)

// rewardList1 returns the first of the three lists Matild and the chest each draw one item from.
func rewardList1() []int32 {
	return []int32{
		auraFlare,
		prominence,
		blazingCircle,
		surrenderToWind,
		auraBolt,
		blazingSkin,
		sleepingCloud,
		cancel,
		decay,
		seedOfFire,
		auraSymphony,
		inferno,
		elementalAssault,
		vampiricClaw,
		silence,
		curseDiscord,
		curseFear,
		summonCorruptedMan,
		transferPain,
		deathSpike,
		forget,
		anchor,
		curseGloom,
		summonReanimatedMan,
		corpseBurst,
		curseDeathLink,
		curseDisease,
		summonKaiTheCat,
		servitorPhysicalShield,
		summonStormCubic,
		summonBindingCubic,
		servitorMagicShield,
		servitorHaste,
		greaterGroupHeal,
	}
}

// rewardList2 returns the second of the three lists Matild and the chest each draw one item from.
func rewardList2() []int32 {
	return []int32{
		greaterBattleHeal,
		greaterHeal,
		mightOfHeaven,
		massRessurection,
		requiem,
		repose,
		purify,
		restoreLife,
		vitalize,
		holdUndead,
		benediction,
		guidance,
		deathWhisper,
		returnSkill,
		invigor,
		blessShield,
		wordOfFear,
		magicBarrier,
		blessedBody,
		blessedSoul,
		haste,
		hydroBlast,
		frostBolt,
		surrenderToWater,
		solarFlare,
		freezingSkin,
		manaRegeneration,
		iceDagger,
		freezingShackle,
		frostWall,
		seedOfWater,
		blizzard,
		summonMerrowTheUnicorn,
		summonLifeCubic,
	}
}

// rewardList3 returns the third of the three lists Matild and the chest each draw one item from.
func rewardList3() []int32 {
	return []int32{
		summonAquaCubic,
		resistShock,
		serenadeOfEva,
		partyReturn,
		hurricane,
		shadowFlare,
		tempest,
		seedOfWind,
		demonWind,
		summonSoulless,
		summonPhantomCubic,
		summonSparkCubic,
		stealEssence,
		sealOfWinter,
		sealOfScourge,
		paagrioGlory,
		paagrioProtection,
		paagrioWisdom,
		paagrioTack,
		sealOfMirage,
		soulGuard,
		paagrioVision,
		sealOfGloom,
		paagrioRage,
		sealOfSilence,
		sealOfSuspension,
		sealOfFlame,
		paagrioHaste,
		chantOfEvasion,
		freezingFlame,
		chantOfRage,
		chantOfFury,
		chantOfRevenge,
	}
}

// demonSetPartsRewardList returns the demon set parts reward list.
func demonSetPartsRewardList() []int32 {
	return []int32{
		demonTunicFabric,
		demonHosePattern,
		demonBootsFabric,
		demonGlovesFabric,
	}
}
