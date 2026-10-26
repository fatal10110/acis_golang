// Package q335 is the quest Song of the Hunter: the hunters' guild asks the
// player to pass two tests for hunter licenses, to hunt what the guild
// master Tor asks for, and to turn blood crystals with a dagger.
package q335

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q335_SongOfTheHunter"

// The lines of the monsters called out for a request.
const (
	say33511 = 33511
	say33512 = 33512
	say33513 = 33513
)

// NPCs.
const (
	grey     = 30744
	tor      = 30745
	cybellin = 30746
)

// Items.
const (
	adena = 57
)

// Quest items.
const (
	cybellinDagger    = 3471
	license1          = 3692
	license2          = 3693
	laurelLeafPin     = 3694
	testInstructions1 = 3695
	testInstructions2 = 3696
	cybellinRequest   = 3697
)

// Grey's requests.
const (
	basiliskScale     = 3709
	karutWeed         = 3710
	hakaHead          = 3711
	jakaHead          = 3712
	markaHead         = 3713
	alephSkin         = 3714
	indigoRunestone   = 3715
	sporeseaSeed      = 3716
	orcTotem          = 3717
	trisalimSilk      = 3718
	ambrosiusFruit    = 3719
	balefireCrystal   = 3720
	imperialArrowhead = 3721
	athuHead          = 3722
	lankaHead         = 3723
	triskaHead        = 3724
	moturaHead        = 3725
	kalathHead        = 3726
)

// Tor's requests: request items.
const (
	request1st1c  = 3727
	request1st2c  = 3728
	request1st3c  = 3729
	request1st4c  = 3730
	request1st5c  = 3731
	request1st6c  = 3732
	request1st7c  = 3733
	request1st8c  = 3734
	request1st9c  = 3735
	request1st10c = 3736
	request1st11c = 3737
	request1st12c = 3738
	request1st1b  = 3739
	request1st2b  = 3740
	request1st3b  = 3741
	request1st4b  = 3742
	request1st5b  = 3743
	request1st6b  = 3744
	request1st1a  = 3745
	request1st2a  = 3746
	request1st3a  = 3747
	request2nd1c  = 3748
	request2nd2c  = 3749
	request2nd3c  = 3750
	request2nd4c  = 3751
	request2nd5c  = 3752
	request2nd6c  = 3753
	request2nd7c  = 3754
	request2nd8c  = 3755
	request2nd9c  = 3756
	request2nd10c = 3757
	request2nd11c = 3758
	request2nd12c = 3759
	request2nd1b  = 3760
	request2nd2b  = 3761
	request2nd3b  = 3762
	request2nd4b  = 3763
	request2nd5b  = 3764
	request2nd6b  = 3765
	request2nd1a  = 3766
	request2nd2a  = 3767
	request2nd3a  = 3768
)

// Tor's requests: drop items.
const (
	charmOfKadesh         = 3769
	timakJadeNecklace     = 3770
	enchantedGolemShard   = 3771
	giantMonsterEyeMeat   = 3772
	direWyrmEgg           = 3773
	grdBasiliskTalon      = 3774
	revenantChains        = 3775
	windsusTusk           = 3776
	grandisSkull          = 3777
	taikObsidianAmulet    = 3778
	karulBugbearHead      = 3779
	tamlinIvoryCharm      = 3780
	fangOfNarak           = 3781
	enchantedGargoyleHorn = 3782
	coiledSerpentTotem    = 3783
	totemOfKadesh         = 3784
	kaikiHead             = 3785
	kronbeVenomSac        = 3786
	evaCharm              = 3787
	titanTablet           = 3788
	bookOfShunaiman       = 3789
	rottingTreeSpores     = 3790
	trisalimVenomSac      = 3791
	taikOrcTotem          = 3792
	haritBarbedNecklace   = 3793
	coinOfOldEmpire       = 3794
	skinOfFarcran         = 3795
	tempestShard          = 3796
	tsunamiShard          = 3797
	satyrMane             = 3798
	hamadryadShard        = 3799
	vanorSilenosMane      = 3800
	talkBugbearTotem      = 3801
	okunHead              = 3802
	kakranHead            = 3803
	narcissusSoulstone    = 3804
	depriveEye            = 3805
	unicornHorn           = 3806
	kerunoGoldMane        = 3807
	skullOfExecuted       = 3808
	bustOfTravis          = 3809
	swordOfCadmus         = 3810
)

// Cybellin's requests: blood crystals.
const (
	bloodCrystal01 = 3698
	bloodCrystal02 = 3699
	bloodCrystal03 = 3700
	bloodCrystal04 = 3701
	bloodCrystal05 = 3702
	bloodCrystal06 = 3703
	bloodCrystal07 = 3704
	bloodCrystal08 = 3705
	bloodCrystal09 = 3706
	bloodCrystal10 = 3707
	brokenBlood    = 3708
)

// Monsters.
const (
	brekaOrcShaman          = 20269
	brekaOrcWarrior         = 20271
	guardianBasilisk        = 20550
	fetteredSoul            = 20552
	windsus                 = 20553
	grandis                 = 20554
	giantFungus             = 20555
	giantMonstereye         = 20556
	direWyrm                = 20557
	rottingTree             = 20558
	trisalimSpider          = 20560
	trisalimTarantula       = 20561
	sporeZombie             = 20562
	manashenGargoyle        = 20563
	enchantedStoneGolem     = 20565
	enchantedGargoyle       = 20567
	tarlkBugbearWarrior     = 20571
	letoLizardmanArcher     = 20578
	letoLizardmanSoldier    = 20579
	letoLizardmanShaman     = 20581
	letoLizardmanOverlord   = 20582
	timakOrcWarrior         = 20586
	timakOrcOverlord        = 20588
	fline                   = 20589
	liele                   = 20590
	valleyTreant            = 20591
	satyr                   = 20592
	unicorn                 = 20593
	forestRunner            = 20594
	valleyTreantElder       = 20597
	satyrElder              = 20598
	unicornElder            = 20599
	karulBugbear            = 20600
	tamlinOrc               = 20601
	tamlinOrcArcher         = 20602
	kronbeSpider            = 20603
	taikOrcArcher           = 20631
	taikOrcWarrior          = 20632
	taikOrcShaman           = 20633
	taikOrcCaptain          = 20634
	mirror                  = 20639
	haritLizardmanGrunt     = 20641
	haritLizardmanArcher    = 20642
	haritLizardmanWarrior   = 20643
	graveWanderer           = 20659
	archerOfGreed           = 20660
	hatarRatmanThief        = 20661
	hatarRatmanBoss         = 20662
	deprive                 = 20664
	farcran                 = 20667
	tairim                  = 20675
	judgeOfMarsh            = 20676
	vanorSilenosGrunt       = 20682
	vanorSilenosScout       = 20683
	vanorSilenosWarrior     = 20684
	vanorSilenosChieftain   = 20686
	brekaOverlordHaka       = 27140
	brekaOverlordJaka       = 27141
	brekaOverlordMarka      = 27142
	windsusAleph            = 27143
	tarlkRaiderAthu         = 27144
	tarlkRaiderLanka        = 27145
	tarlkRaiderTriska       = 27146
	tarlkRaiderMotura       = 27147
	tarlkRaiderKalath       = 27148
	gremlinFilcher          = 27149
	blackLegionStormtrooper = 27150
	letoShamanKetz          = 27156
	letoChiefNarak          = 27157
	timakRaiderKaikee       = 27158
	timakOverlordOkun       = 27159
	gokMagok                = 27160
	taikOverlordKakran      = 27161
	hatarChieftainKubel     = 27162
	vanorElderKerunos       = 27163
	karulChiefOrooto        = 27164
)

// greyRequirement is what the player must hold, of every one of ids, to pass
// one of the requirements of Grey's tests.
type greyRequirement struct {
	required int32
	ids      []int32
}

// greyDrop is a monster that drops item for one of Grey's tests, until the
// player holds needed of it.
type greyDrop struct{ monster, item, needed, chance int32 }

// torReward is a request of Tor: the item to collect, how many of it, and
// the adena paid.
type torReward struct{ request, item, required, reward int32 }

// torDrop is a monster that drops the item of a request; message is the
// line it says when it drops nothing.
type torDrop struct{ request, monster, chance, message int32 }

// torSpawn is a monster that, killed for a request, may call out count
// monsters of spawn; the spawn needs the item of the request to be held
// in full when reached, and not yet when not.
type torSpawn struct {
	request, monster, chance, spawn, count, message int32
	reached                                         bool
}

// bloodCrystalReward is a blood crystal stage and the adena it is paid.
type bloodCrystalReward struct{ crystal, reward int32 }

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Song of the Hunter",
		QuestID: 335,
		Items: []int32{
			cybellinDagger, license1, license2, laurelLeafPin,
			testInstructions1, testInstructions2, cybellinRequest, basiliskScale,
			karutWeed, hakaHead, jakaHead, markaHead,
			alephSkin, indigoRunestone, sporeseaSeed, orcTotem,
			trisalimSilk, ambrosiusFruit, balefireCrystal, imperialArrowhead,
			athuHead, lankaHead, triskaHead, moturaHead,
			kalathHead, bloodCrystal01, bloodCrystal02, bloodCrystal03,
			bloodCrystal04, bloodCrystal05, bloodCrystal06, bloodCrystal07,
			bloodCrystal08, bloodCrystal09, bloodCrystal10, brokenBlood,
			request1st1c, request1st2c, request1st3c, request1st4c,
			request1st5c, request1st6c, request1st7c, request1st8c,
			request1st9c, request1st10c, request1st11c, request1st12c,
			request1st1b, request1st2b, request1st3b, request1st4b,
			request1st5b, request1st6b, request1st1a, request1st2a,
			request1st3a, request2nd1c, request2nd2c, request2nd3c,
			request2nd4c, request2nd5c, request2nd6c, request2nd7c,
			request2nd8c, request2nd9c, request2nd10c, request2nd11c,
			request2nd12c, request2nd1b, request2nd2b, request2nd3b,
			request2nd4b, request2nd5b, request2nd6b, request2nd1a,
			request2nd2a, request2nd3a, charmOfKadesh, timakJadeNecklace,
			enchantedGolemShard, giantMonsterEyeMeat, direWyrmEgg, grdBasiliskTalon,
			revenantChains, windsusTusk, grandisSkull, taikObsidianAmulet,
			karulBugbearHead, tamlinIvoryCharm, fangOfNarak, enchantedGargoyleHorn,
			coiledSerpentTotem, totemOfKadesh, kaikiHead, kronbeVenomSac,
			evaCharm, titanTablet, bookOfShunaiman, rottingTreeSpores,
			trisalimVenomSac, taikOrcTotem, haritBarbedNecklace, coinOfOldEmpire,
			skinOfFarcran, tempestShard, tsunamiShard, satyrMane,
			hamadryadShard, vanorSilenosMane, talkBugbearTotem, okunHead,
			kakranHead, narcissusSoulstone, depriveEye, unicornHorn,
			kerunoGoldMane, skullOfExecuted, bustOfTravis, swordOfCadmus,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {grey},
			script.EventTalked:     {grey, tor, cybellin},
			script.EventMyDying:    dyingMonsters(),
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// dyingMonsters lists every monster the quest hears the death of.
func dyingMonsters() []int32 {
	var ids []int32
	for _, d := range greyDrops1() {
		ids = append(ids, d.monster)
	}
	for _, d := range greyDrops2() {
		ids = append(ids, d.monster)
	}
	for _, d := range torDrops() {
		ids = append(ids, d.monster)
	}
	for _, sp := range torSpawns() {
		ids = append(ids, sp.monster)
	}
	return append(ids, cybellinMonsters()...)
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	// Grey
	case strings.EqualFold(e.Name, "30744-03.htm"):
		st.SetCond(1)
		st.SetStatus(questlog.StatusStarted)
		s.GiveItems(p, testInstructions1, 1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30744-09.htm"):
		// Has Tor's request, skip.
		if st.GetInt("tor") > 0 {
			htmltext = "30744-09a.htm"
		} else {
			// Start Grey's test 2.
			setInt(st, "grey2", 1)
			s.GiveItems(p, testInstructions2, 1)
			s.PlaySound(p, script.SoundMiddle)
		}
	case strings.EqualFold(e.Name, "30744-16.htm"):
		if p.ItemCount(laurelLeafPin) >= 20 {
			htmltext = "30744-17.htm"
			s.RewardItems(p, adena, 20000)
		}
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
	// Tor
	case strings.EqualFold(e.Name, "30745-02.htm"):
		// Grey's test 2 is running, can't start Tor's request.
		if st.GetInt("grey2") > 0 {
			htmltext = "30745-03.htm"
		}
	case strings.EqualFold(e.Name, "Tor_list_1"):
		// Index = 0-11 = C, 12-17 = B, 18-20 = A
		if st.GetInt("tor") == 0 {
			menu := torMenu1()
			htmltext = torOffer(s, p, st, menu, true)
		}
	case strings.EqualFold(e.Name, "Tor_list_2"):
		if st.GetInt("tor") == 0 {
			menu := torMenu2()
			htmltext = torOffer(s, p, st, menu, false)
		}
	case strings.EqualFold(e.Name, "30745-05b.htm"):
		request := st.GetInt("tor")
		if request > 0 {
			s.TakeItems(p, request, -1)
			if reward, ok := torRewardOf(request); ok {
				s.TakeItems(p, reward.item, -1)
			}
			s.TakeItems(p, laurelLeafPin, 1)
			st.Unset("tor")
			s.PlaySound(p, script.SoundGiveUp)
		}
	case script.IsDigit(e.Name) && len(e.Name) == 4:
		parsed, err := strconv.ParseInt(e.Name, 10, 32)
		request := int32(parsed)
		if _, ok := torRewardOf(request); err != nil || !ok {
			// Only the requests of the menus are accepted: any other number
			// would hand out an arbitrary item.
			return ""
		}
		setInt(st, "tor", request)
		s.GiveItems(p, request, 1)
		htmltext = "30745-" + e.Name + ".htm"
	// Cybellin
	case strings.EqualFold(e.Name, "30746-03.htm"):
		sound := false
		if !p.HasItem(cybellinRequest) {
			s.GiveItems(p, cybellinRequest, 1)
			sound = true
		}
		if !p.HasItem(cybellinDagger) {
			s.GiveItems(p, cybellinDagger, 1)
			sound = true
		}
		s.TakeItems(p, brokenBlood, -1)
		if !p.HasItem(bloodCrystal01) {
			s.GiveItems(p, bloodCrystal01, 1)
			sound = true
		}
		setInt(st, "cybellin", 1)
		if sound {
			s.PlaySound(p, script.SoundMiddle)
		}
	case strings.EqualFold(e.Name, "30746-06.htm"):
		for _, r := range bloodCrystalRewards() {
			if p.HasItem(r.crystal) {
				s.TakeItems(p, r.crystal, -1)
				s.RewardItems(p, adena, r.reward)
				break
			}
		}
	case strings.EqualFold(e.Name, "30746-10.htm"):
		st.Unset("cybellin")
		// Only the blood crystal of the first stage is destroyed, the
		// other stages are kept.
		s.TakeItems(p, bloodCrystal01, -1)
		s.TakeItems(p, cybellinRequest, -1)
		s.TakeItems(p, cybellinDagger, -1)
	}
	return htmltext
}

// torOffer returns the page of the five requests offered to p by Tor, from
// menu: the offer kept in st, or a new one of replies that all differ; first
// tells the first request list from the second.
func torOffer(s *script.Script, p *script.Player, st *script.QuestState, menu []string, first bool) string {
	var reply0, reply1, reply2, reply3, reply4 int32
	offer := st.GetInt("torOffer")
	if offer == 0 {
		// Generate new offer.
		leafPins := p.ItemCount(laurelLeafPin)
		for {
			if first {
				reply0, reply1, reply2, reply3 = firstOffer(s, leafPins)
			} else {
				reply0, reply1 = secondOffer(s, leafPins)
				reply2 = int32(s.Rnd(6))     // low C
				reply3 = int32(s.Rnd(6)) + 6 // high C
			}
			reply4 = int32(s.Rnd(12)) // C
			if reply0 != reply1 && reply0 != reply2 && reply0 != reply3 && reply0 != reply4 && reply1 != reply2 && reply1 != reply3 && reply1 != reply4 && reply2 != reply3 && reply2 != reply4 && reply3 != reply4 {
				break
			}
		}
		// Store new offer.
		offer = reply0 | reply1<<4 | reply2<<8 | reply3<<12 | reply4<<16
		setInt(st, "torOffer", offer)
	} else {
		// Parse options.
		reply0 = offer & 0x0F
		reply1 = (offer >> 4) & 0x0F
		reply2 = (offer >> 8) & 0x0F
		reply3 = (offer >> 12) & 0x0F
		reply4 = (offer >> 16) & 0x0F
	}
	page := s.HTMLText("30745-16.htm")
	page = strings.ReplaceAll(page, "%reply0%", menu[reply0])
	page = strings.ReplaceAll(page, "%reply1%", menu[reply1])
	page = strings.ReplaceAll(page, "%reply2%", menu[reply2])
	page = strings.ReplaceAll(page, "%reply3%", menu[reply3])
	return strings.ReplaceAll(page, "%reply4%", menu[reply4])
}

// rnd returns a random number from 0 to n-1.
func rnd(s *script.Script, n int) int32 { return int32(s.Rnd(n)) }

// firstOffer draws the first four replies of a new offer of the first
// request list, by the laurel leaf pins p holds.
func firstOffer(s *script.Script, leafPins int32) (reply0, reply1, reply2, reply3 int32) {
	switch {
	case leafPins == 0:
		reply0 = rnd(s, 12) // C
		reply1 = rnd(s, 12) // C
		reply2 = rnd(s, 12) // C
		reply3 = rnd(s, 12) // C
	case leafPins < 4:
		if s.Rnd(100) < 20 {
			reply0 = rnd(s, 6) + 12 // B
			reply2 = rnd(s, 6)      // low C
			reply3 = rnd(s, 6) + 6  // high C
		} else { // 5x C
			reply0 = rnd(s, 12) // C
			reply2 = rnd(s, 12) // C
			reply3 = rnd(s, 12) // C
		}
		reply1 = rnd(s, 12) // C
	default:
		if s.Rnd(100) < 20 {
			reply0 = rnd(s, 6) + 12 // B
		} else {
			reply0 = rnd(s, 12) // C
		}
		if s.Rnd(100) < 5 {
			reply1 = rnd(s, 3) + 18 // A
		} else {
			reply1 = rnd(s, 12) // C
		}
		reply2 = rnd(s, 6)     // low C
		reply3 = rnd(s, 6) + 6 // high C
	}
	return reply0, reply1, reply2, reply3
}

// secondOffer draws the first two replies of a new offer of the second
// request list, by the laurel leaf pins p holds.
func secondOffer(s *script.Script, leafPins int32) (reply0, reply1 int32) {
	switch {
	case leafPins == 0:
		reply0 = rnd(s, 12) // C
		reply1 = rnd(s, 12) // C
	case leafPins < 4:
		if s.Rnd(100) < 20 {
			reply0 = rnd(s, 6) + 12 // B
		} else {
			reply0 = rnd(s, 12) // C
		}
		reply1 = rnd(s, 12) // C
	default:
		if s.Rnd(100) < 20 {
			reply0 = rnd(s, 6) + 12 // B
		} else {
			reply0 = rnd(s, 12) // C
		}
		if s.Rnd(100) < 5 {
			reply1 = rnd(s, 3) + 18 // A
		} else {
			reply1 = rnd(s, 12) // C
		}
	}
	return reply0, reply1
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	cond := st.Cond()
	switch st.Status() {
	case questlog.StatusCreated:
		if p.Level() >= 35 {
			htmltext = "30744-02.htm"
		} else {
			htmltext = "30744-01.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case grey:
			switch cond {
			case 1:
				// Check Apprentice become Hunter license 1.
				if passedGreyRequest(p, greyTest1()) {
					htmltext = "30744-06.htm"
					st.SetCond(2)
					for _, r := range greyTest1() {
						for _, item := range r.ids {
							s.TakeItems(p, item, -1)
						}
					}
					s.TakeItems(p, testInstructions1, -1)
					s.GiveItems(p, license1, 1)
					s.PlaySound(p, script.SoundMiddle)
				} else {
					htmltext = "30744-05.htm"
				}
			case 2:
				switch {
				// Hunter license 1 below 45 lvl.
				case p.Level() < 45:
					htmltext = "30744-07.htm"
				// Hunter license 1 above 45 lvl, without started test 2.
				case st.GetInt("grey2") == 0:
					htmltext = "30744-08.htm"
				// Hunter license 1 above 45 lvl, with started test 2: check
				// Hunter license 1 become Hunter license 2.
				case passedGreyRequest(p, greyTest2()):
					htmltext = "30744-12.htm"
					st.SetCond(3)
					st.Unset("grey2")
					for _, r := range greyTest2() {
						for _, item := range r.ids {
							s.TakeItems(p, item, -1)
						}
					}
					s.TakeItems(p, testInstructions2, -1)
					s.TakeItems(p, license1, -1)
					s.GiveItems(p, license2, 1)
					s.PlaySound(p, script.SoundMiddle)
				default:
					htmltext = "30744-11.htm"
				}
			case 3:
				htmltext = "30744-14.htm"
			}
		case tor:
			request := st.GetInt("tor")
			if cond == 1 {
				// Apprentice.
				htmltext = "30745-01a.htm"
				break
			}
			// Hunter license 1 or 2.
			reward, ok := torRewardOf(request)
			switch {
			// Request doesn't exist.
			case !ok:
				if cond == 2 {
					// Hunting license 1 - multiple options.
					switch {
					case p.Level() < 45:
						htmltext = "30745-01b.htm"
					case p.HasItem(testInstructions2):
						htmltext = "30745-03.htm"
					default:
						htmltext = "30745-03a.htm"
					}
				} else {
					// Hunting license 2, just show html.
					htmltext = "30745-03b.htm"
				}
			// Request exists and reward requirements has been met.
			case p.ItemCount(reward.item) >= reward.required:
				// Show reward html depending on hunting license.
				if cond == 2 {
					htmltext = "30745-06a.htm"
				} else {
					htmltext = "30745-06b.htm"
				}
				setInt(st, "tor", 0)
				setInt(st, "torOffer", 0)
				s.TakeItems(p, request, -1)
				s.TakeItems(p, reward.item, -1)
				s.GiveItems(p, laurelLeafPin, 1)
				s.RewardItems(p, adena, reward.reward)
				s.PlaySound(p, script.SoundMiddle)
			// Request exists, but reward requirements are not met.
			default:
				htmltext = "30745-05.htm"
			}
		case cybellin:
			switch {
			// Apprentice.
			case cond == 1:
				htmltext = "30746-01.htm"
			// Hunter license 1 or 2: doesn't have Cybellin request.
			case st.GetInt("cybellin") == 0:
				htmltext = "30746-02.htm"
			// Has Blood Crystal 1.
			case p.HasItem(bloodCrystal01):
				htmltext = "30746-04.htm"
			// Has Blood Crystal 2-9.
			case p.HasAtLeastOneItem(bloodCrystal02, bloodCrystal03, bloodCrystal04, bloodCrystal05, bloodCrystal06, bloodCrystal07, bloodCrystal08, bloodCrystal09):
				htmltext = "30746-05.htm"
			// Has Blood Crystal 10.
			case p.HasItem(bloodCrystal10):
				htmltext = "30746-05a.htm"
				s.TakeItems(p, bloodCrystal10, -1)
				s.RewardItems(p, adena, bloodCrystalRewards()[9].reward)
			// Has Broken Blood.
			case p.HasItem(brokenBlood):
				htmltext = "30746-09.htm"
				s.TakeItems(p, brokenBlood, -1)
			// Has no Blood Crystal, neither Broken Blood.
			default:
				htmltext = "30746-08.htm"
			}
		}
	}
	return htmltext
}

// onMyDying drops the quest items of the killer's tests and requests, and
// calls out the monsters those ask for.
func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	npcID := e.NPC.NpcID()
	if st.Cond() == 1 {
		// Apprentice: solving Grey's request 1 and getting Hunter license 1.
		if d, ok := greyDropOf(greyDrops1(), npcID); ok {
			s.DropItems(p, d.item, 1, d.needed, d.chance)
		} else if npcID == brekaOrcWarrior && s.Rnd(100) < 20 {
			// Breka Orc Warrior summons sequentially 3 Breka Bandits.
			switch {
			case !p.HasItem(hakaHead):
				s.Spawner().AddSpawn(brekaOverlordHaka, e.NPC, true, 300000*time.Millisecond)
			case !p.HasItem(jakaHead):
				s.Spawner().AddSpawn(brekaOverlordJaka, e.NPC, true, 300000*time.Millisecond)
			case !p.HasItem(markaHead):
				s.Spawner().AddSpawn(brekaOverlordMarka, e.NPC, true, 300000*time.Millisecond)
			}
		} else if npcID == windsus && !p.HasItem(alephSkin) && s.Rnd(100) < 20 {
			// Windsus summons Windsus Aleph.
			s.Spawner().AddSpawn(windsusAleph, e.NPC, true, 300000*time.Millisecond)
		}
		return
	}
	// Hunter license 1 or 2.
	// Solving Grey's request 2 - getting Hunter license 2.
	if st.Cond() == 2 && st.GetInt("grey2") > 0 {
		if d, ok := greyDropOf(greyDrops2(), npcID); ok {
			s.DropItems(p, d.item, 1, d.needed, d.chance)
		} else if npcID == tarlkBugbearWarrior && s.Rnd(100) < 20 {
			switch {
			case !p.HasItem(athuHead):
				s.Spawner().AddSpawn(tarlkRaiderAthu, e.NPC, true, 300000*time.Millisecond)
			case !p.HasItem(lankaHead):
				s.Spawner().AddSpawn(tarlkRaiderLanka, e.NPC, true, 300000*time.Millisecond)
			case !p.HasItem(triskaHead):
				s.Spawner().AddSpawn(tarlkRaiderTriska, e.NPC, true, 300000*time.Millisecond)
			case !p.HasItem(moturaHead):
				s.Spawner().AddSpawn(tarlkRaiderMotura, e.NPC, true, 300000*time.Millisecond)
			case !p.HasItem(kalathHead):
				s.Spawner().AddSpawn(tarlkRaiderKalath, e.NPC, true, 300000*time.Millisecond)
			}
		}
	}
	// Solving Tor's request.
	if request := st.GetInt("tor"); request > 0 {
		reward, known := torRewardOf(request)
		if !known {
			return
		}
		if trd, ok := torDropOf(request, npcID); ok && !s.DropItems(p, reward.item, 1, reward.required, trd.chance) && trd.message != 0 {
			e.NPC.Say(script.NpcString(trd.message))
		}
		// Spawn another NPC when marked as reached and the collected quest
		// item reaches the required quantity, or when marked as non-reached
		// and the collected quest item is below the required quantity.
		if trs, ok := torSpawnOf(request, npcID); ok && s.Rnd(100) < int(trs.chance) && trs.reached == (p.ItemCount(reward.item) >= reward.required) {
			for range trs.count {
				spawned := s.Spawner().AddSpawn(trs.spawn, e.NPC, true, 300000*time.Millisecond)
				if spawned == nil {
					continue
				}
				spawned.AddAttackDesire(p, 200)
				if trs.message != 0 {
					spawned.Say(script.NpcString(trs.message))
				}
			}
		}
	}
	// Solving Cybellin's request: the proper monster was killed using
	// Cybellin's dagger.
	if st.GetInt("cybellin") > 0 && containsMonster(cybellinMonsters(), npcID) && p.RightHandItemID() == cybellinDagger {
		successful := s.Rnd(100) < 60
		for _, r := range bloodCrystalRewards() {
			if r.crystal == bloodCrystal10 || !p.HasItem(r.crystal) {
				continue
			}
			// Delete actual crystal.
			s.TakeItems(p, r.crystal, -1)
			if successful {
				// Success, stage up first found crystal.
				s.GiveItems(p, r.crystal+1, 1)
				// Play sound for upgrades above stage 6.
				if r.crystal >= bloodCrystal06 {
					s.PlaySound(p, script.SoundJackpot)
				}
			} else {
				// Fail, give broken crystal.
				s.GiveItems(p, brokenBlood, 1)
			}
			break
		}
	}
}

// passedGreyRequest reports whether p meets at least 3 of the requirements.
func passedGreyRequest(p *script.Player, requirements []greyRequirement) bool {
	passed := 0
	for _, r := range requirements {
		pass := true
		for _, id := range r.ids {
			if p.ItemCount(id) < r.required {
				pass = false
			}
		}
		if pass {
			passed++
		}
	}
	return passed >= 3
}

func greyDropOf(drops []greyDrop, monster int32) (greyDrop, bool) {
	for _, d := range drops {
		if d.monster == monster {
			return d, true
		}
	}
	return greyDrop{}, false
}

func torRewardOf(request int32) (torReward, bool) {
	for _, r := range torRewards() {
		if r.request == request {
			return r, true
		}
	}
	return torReward{}, false
}

// torDropOf returns the first drop of request that monster makes.
func torDropOf(request, monster int32) (torDrop, bool) {
	for _, d := range torDrops() {
		if d.request == request && d.monster == monster {
			return d, true
		}
	}
	return torDrop{}, false
}

// torSpawnOf returns the first spawn of request that monster makes.
func torSpawnOf(request, monster int32) (torSpawn, bool) {
	for _, sp := range torSpawns() {
		if sp.request == request && sp.monster == monster {
			return sp, true
		}
	}
	return torSpawn{}, false
}

func containsMonster(ids []int32, id int32) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

// setInt sets the quest variable key to the number n.
func setInt(st *script.QuestState, key string, n int32) {
	st.Set(key, strconv.Itoa(int(n)))
}

// torRewards are the requests, each with the item to collect, how many of it, and the adena paid.
func torRewards() []torReward {
	return []torReward{
		{request1st1c, charmOfKadesh, 40, 2090},
		{request1st2c, timakJadeNecklace, 50, 6340},
		{request1st3c, enchantedGolemShard, 50, 9480},
		{request1st4c, giantMonsterEyeMeat, 30, 9110},
		{request1st5c, direWyrmEgg, 40, 8690},
		{request1st6c, grdBasiliskTalon, 100, 9480},
		{request1st7c, revenantChains, 50, 11280},
		{request1st8c, windsusTusk, 30, 9640},
		{request1st9c, grandisSkull, 100, 9180},
		{request1st10c, taikObsidianAmulet, 50, 5160},
		{request1st11c, karulBugbearHead, 30, 3140},
		{request1st12c, tamlinIvoryCharm, 40, 3160},
		{request1st1b, fangOfNarak, 1, 6370},
		{request1st2b, enchantedGargoyleHorn, 50, 19080},
		{request1st3b, coiledSerpentTotem, 50, 17730},
		{request1st4b, totemOfKadesh, 1, 5790},
		{request1st5b, kaikiHead, 1, 8560},
		{request1st6b, kronbeVenomSac, 30, 8320},
		{request1st1a, evaCharm, 30, 30310},
		{request1st2a, titanTablet, 1, 27540},
		{request1st3a, bookOfShunaiman, 1, 20560},
		{request2nd1c, rottingTreeSpores, 40, 6850},
		{request2nd2c, trisalimVenomSac, 40, 7250},
		{request2nd3c, taikOrcTotem, 50, 7160},
		{request2nd4c, haritBarbedNecklace, 40, 6580},
		{request2nd5c, coinOfOldEmpire, 20, 10100},
		{request2nd6c, skinOfFarcran, 30, 13000},
		{request2nd7c, tempestShard, 40, 7660},
		{request2nd8c, tsunamiShard, 40, 7660},
		{request2nd9c, satyrMane, 40, 11260},
		{request2nd10c, hamadryadShard, 40, 7660},
		{request2nd11c, vanorSilenosMane, 30, 8810},
		{request2nd12c, talkBugbearTotem, 30, 7350},
		{request2nd1b, okunHead, 1, 8760},
		{request2nd2b, kakranHead, 1, 9380},
		{request2nd3b, narcissusSoulstone, 40, 17820},
		{request2nd4b, depriveEye, 20, 17540},
		{request2nd5b, unicornHorn, 20, 14160},
		{request2nd6b, kerunoGoldMane, 1, 15960},
		{request2nd1a, skullOfExecuted, 20, 39100},
		{request2nd2a, bustOfTravis, 1, 39550},
		{request2nd3a, swordOfCadmus, 10, 41200},
	}
}

// torDrops are the monsters that drop the item of a request, at a chance out of a million.
func torDrops() []torDrop {
	return []torDrop{
		{request1st1c, letoLizardmanArcher, 900000, 0},
		{request1st1c, letoLizardmanSoldier, 930000, 0},
		{request1st2c, timakOrcOverlord, 1000000, 0},
		{request1st2c, timakOrcWarrior, 950000, 0},
		{request1st3c, enchantedStoneGolem, 1000000, 0},
		{request1st4c, giantMonstereye, 600000, 0},
		{request1st5c, direWyrm, 900000, 0},
		{request1st6c, guardianBasilisk, 1600000, 0},
		{request1st7c, fetteredSoul, 1000000, 0},
		{request1st8c, windsus, 630000, 0},
		{request1st9c, grandis, 2000000, 0},
		{request1st10c, taikOrcArcher, 1000000, 0},
		{request1st10c, taikOrcWarrior, 930000, 0},
		{request1st11c, karulBugbear, 600000, 0},
		{request1st12c, tamlinOrc, 720000, 0},
		{request1st12c, tamlinOrcArcher, 900000, 0},
		{request1st1b, letoChiefNarak, 1000000, 0},
		{request1st2b, enchantedGargoyle, 600000, 0},
		{request1st3b, brekaOrcShaman, 930000, 0},
		{request1st3b, brekaOrcWarrior, 1000000, 0},
		{request1st4b, letoShamanKetz, 1000000, 0},
		{request1st5b, timakRaiderKaikee, 1000000, 0},
		{request1st6b, kronbeSpider, 600000, 0},
		{request1st1a, sporeZombie, 600000, 0},
		{request1st2a, gokMagok, 1000000, 0},
		{request1st3a, karulChiefOrooto, 1000000, 0},
		{request2nd1c, rottingTree, 770000, 0},
		{request2nd2c, trisalimSpider, 760000, 0},
		{request2nd2c, trisalimTarantula, 850000, 0},
		{request2nd3c, taikOrcCaptain, 990000, 0},
		{request2nd3c, taikOrcShaman, 630000, 0},
		{request2nd4c, haritLizardmanArcher, 980000, 0},
		{request2nd4c, haritLizardmanGrunt, 980000, 0},
		{request2nd4c, haritLizardmanWarrior, 1000000, 0},
		{request2nd5c, hatarRatmanBoss, 620000, 0},
		{request2nd5c, hatarRatmanThief, 600000, 0},
		{request2nd5c, gremlinFilcher, 3000000, say33513},
		{request2nd6c, farcran, 1000000, 0},
		{request2nd7c, fline, 590000, 0},
		{request2nd7c, gremlinFilcher, 5000000, say33513},
		{request2nd8c, liele, 610000, 0},
		{request2nd8c, gremlinFilcher, 5000000, say33513},
		{request2nd9c, satyr, 900000, 0},
		{request2nd9c, satyrElder, 1000000, 0},
		{request2nd10c, forestRunner, 740000, 0},
		{request2nd10c, gremlinFilcher, 5000000, say33513},
		{request2nd11c, vanorSilenosGrunt, 800000, 0},
		{request2nd11c, vanorSilenosScout, 950000, 0},
		{request2nd11c, vanorSilenosWarrior, 1000000, 0},
		{request2nd12c, tarlkBugbearWarrior, 730000, 0},
		{request2nd1b, timakOverlordOkun, 1000000, 0},
		{request2nd2b, taikOverlordKakran, 1000000, 0},
		{request2nd3b, mirror, 960000, 0},
		{request2nd3b, gremlinFilcher, 5000000, say33513},
		{request2nd4b, deprive, 870000, 0},
		{request2nd5b, unicorn, 780000, 0},
		{request2nd5b, unicornElder, 960000, 0},
		{request2nd6b, vanorElderKerunos, 1000000, 0},
		{request2nd1a, graveWanderer, 830000, 0},
		{request2nd2a, hatarChieftainKubel, 1000000, 0},
		{request2nd3a, judgeOfMarsh, 740000, 0},
	}
}

// torSpawns are the monsters that call out another monster while a request runs.
func torSpawns() []torSpawn {
	return []torSpawn{
		{request1st1b, letoLizardmanOverlord, 20, letoChiefNarak, 1, 0, false},
		{request1st4b, letoLizardmanShaman, 20, letoShamanKetz, 1, 0, false},
		{request1st5b, timakOrcWarrior, 20, timakRaiderKaikee, 1, 0, false},
		{request1st2a, grandis, 20, gokMagok, 1, 0, false},
		{request1st2a, gokMagok, 50, blackLegionStormtrooper, 2, say33511, true},
		{request1st3a, karulBugbear, 20, karulChiefOrooto, 1, 0, false},
		{request1st3a, karulChiefOrooto, 50, blackLegionStormtrooper, 2, say33511, true},
		{request2nd5c, hatarRatmanBoss, 10, gremlinFilcher, 1, say33512, false},
		{request2nd5c, hatarRatmanThief, 10, gremlinFilcher, 1, say33512, false},
		{request2nd7c, fline, 10, gremlinFilcher, 1, say33512, false},
		{request2nd8c, liele, 10, gremlinFilcher, 1, say33512, false},
		{request2nd10c, forestRunner, 10, gremlinFilcher, 1, say33512, false},
		{request2nd1b, timakOrcOverlord, 10, timakOverlordOkun, 1, 0, false},
		{request2nd2b, taikOrcCaptain, 20, taikOverlordKakran, 1, 0, false},
		{request2nd3b, mirror, 10, gremlinFilcher, 1, say33512, false},
		{request2nd6b, vanorSilenosChieftain, 20, vanorElderKerunos, 1, 0, false},
		{request2nd2a, hatarRatmanBoss, 20, hatarChieftainKubel, 1, 0, false},
		{request2nd2a, hatarChieftainKubel, 50, blackLegionStormtrooper, 2, say33511, true},
	}
}

// torMenu1 are the links of the first request offers.
func torMenu1() []string {
	return []string{
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3727">C: 40 Totems of Kadesh</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3728">C: 50 Jade Necklaces of Timak</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3729">C: 50 Enchanted Golem Shards</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3730">C: 30 Pieces Monster Eye Meat</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3731">C: 40 Eggs of Dire Wyrm</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3732">C: 100 Claws of Guardian Basilisk</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3733">C: 50 Revenant Chains</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3734">C: 30 Windsus Tusks</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3735">C: 100 Skulls of Grandis</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3736">C: 50 Taik Obsidian Amulets</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3737">C: 30 Heads of Karul Bugbear</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3738">C: 40 Ivory Charms of Tamlin</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3739">B: Situation Preparation - Leto Chief</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3740">B: 50 Enchanted Gargoyle Horns</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3741">B: 50 Coiled Serpent Totems</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3742">B: Situation Preparation - Sorcerer Catch of Leto</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3743">B: Situation Preparation - Timak Raider Kaikee</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3744">B: 30 Kronbe Venom Sacs</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3745">A: 30 Charms of Eva</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3746">A: Titan's Tablet</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3747">A: Book of Shunaiman</a><br>`,
	}
}

// torMenu2 are the links of the second request offers.
func torMenu2() []string {
	return []string{
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3748">C: 40 Rotted Tree Spores</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3749">C: 40 Trisalim Venom Sacs</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3750">C: 50 Totems of Taik Orc</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3751">C: 40 Harit Barbed Necklaces</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3752">C: 20 Coins of Ancient Empire</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3753">C: 30 Skins of Farkran</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3754">C: 40 Tempest Shards</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3755">C: 40 Tsunami Shards</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3756">C: 40 Satyr Manes</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3757">C: 40 Hamadryad Shards</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3758">C: 30 Manes of Vanor Silenos</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3759">C: 30 Totems of Tarlk Bugbears</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3760">B: Situation Preparation - Overlord Okun of Timak</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3761">B: Situation Preparation - Overlord Kakran of Taik</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3762">B: 40 Narcissus Soulstones</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3763">B: 20 Eyes of Deprived</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3764">B: 20 Unicorn Horns</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3765">B: Kerunos's Gold Mane</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3766">A: 20 Skull of Executed</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3767">A: Bust of Travis</a><br>`,
		`<a action="bypass -h Quest Q335_SongOfTheHunter 3768">A: 10 Swords of Cadmus</a><br>`,
	}
}

// greyTest1 are the requirements of the first test of Grey.
func greyTest1() []greyRequirement {
	return []greyRequirement{
		{40, []int32{basiliskScale}},
		{20, []int32{karutWeed}},
		{1, []int32{hakaHead, jakaHead, markaHead}},
		{1, []int32{alephSkin}},
		{20, []int32{indigoRunestone}},
		{30, []int32{sporeseaSeed}},
	}
}

// greyTest2 are the requirements of the second test of Grey.
func greyTest2() []greyRequirement {
	return []greyRequirement{
		{20, []int32{orcTotem}},
		{20, []int32{trisalimSilk}},
		{30, []int32{ambrosiusFruit}},
		{20, []int32{balefireCrystal}},
		{20, []int32{imperialArrowhead}},
		{1, []int32{athuHead, lankaHead, triskaHead, moturaHead, kalathHead}},
	}
}

// greyDrops1 are the monsters that drop for the first test: monster, item, how many of it, chance out of a million.
func greyDrops1() []greyDrop {
	return []greyDrop{
		{guardianBasilisk, basiliskScale, 40, 900000},
		{letoLizardmanShaman, karutWeed, 20, 600000},
		{brekaOverlordHaka, hakaHead, 1, 1000000},
		{brekaOverlordJaka, jakaHead, 1, 1000000},
		{brekaOverlordMarka, markaHead, 1, 1000000},
		{windsusAleph, alephSkin, 1, 1000000},
		{manashenGargoyle, indigoRunestone, 20, 600000},
		{enchantedStoneGolem, indigoRunestone, 20, 620000},
		{giantFungus, sporeseaSeed, 30, 840000},
	}
}

// greyDrops2 are the monsters that drop for the second test.
func greyDrops2() []greyDrop {
	return []greyDrop{
		{timakOrcWarrior, orcTotem, 20, 600000},
		{trisalimSpider, trisalimSilk, 20, 600000},
		{trisalimTarantula, trisalimSilk, 20, 600000},
		{valleyTreant, ambrosiusFruit, 30, 850000},
		{valleyTreantElder, ambrosiusFruit, 30, 850000},
		{tairim, balefireCrystal, 20, 600000},
		{archerOfGreed, imperialArrowhead, 20, 600000},
		{tarlkRaiderAthu, athuHead, 1, 1000000},
		{tarlkRaiderLanka, lankaHead, 1, 1000000},
		{tarlkRaiderTriska, triskaHead, 1, 1000000},
		{tarlkRaiderMotura, moturaHead, 1, 1000000},
		{tarlkRaiderKalath, kalathHead, 1, 1000000},
	}
}

// cybellinMonsters are the monsters that Cybellin's dagger turns blood crystals with.
func cybellinMonsters() []int32 {
	return []int32{
		letoLizardmanArcher,
		letoLizardmanSoldier,
		letoLizardmanShaman,
		letoLizardmanOverlord,
		haritLizardmanGrunt,
		haritLizardmanArcher,
		haritLizardmanWarrior,
	}
}

// bloodCrystalRewards are the blood crystals, by stage, with the adena each one is paid.
func bloodCrystalRewards() []bloodCrystalReward {
	return []bloodCrystalReward{
		{bloodCrystal01, 0},
		{bloodCrystal02, 3400},
		{bloodCrystal03, 6800},
		{bloodCrystal04, 13600},
		{bloodCrystal05, 37200},
		{bloodCrystal06, 54400},
		{bloodCrystal07, 108800},
		{bloodCrystal08, 217600},
		{bloodCrystal09, 435200},
		{bloodCrystal10, 870400},
	}
}
