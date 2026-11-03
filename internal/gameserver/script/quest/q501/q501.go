// Package q501 is the quest Proof Of Clan Alliance (Sir Kristof Rodemai): the
// leader of a level 3 clan sends the clan's members to sacrifice themselves
// at the Statue of Offering, to fetch three herbs and to open the Boxes of
// Athrea for the Blood of Eva. Every progress check follows the leader's quest
// state, and a member's kill counts only while the member stands near the
// monster.
package q501

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q501_ProofOfClanAlliance"

// Items.
const (
	adena            = 57
	potionOfRecovery = 3889
)

// Quest items.
const (
	herbOfHarit        = 3832
	herbOfVanor        = 3833
	herbOfOelMahum     = 3834
	bloodOfEva         = 3835
	symbolOfLoyalty    = 3837
	voucherOfFaith     = 3873
	antidoteRecipeList = 3872
)

// The reward.
const allianceManifesto = 3874

// NPCs.
const (
	sirKristofRodemai = 30756
	statueOfOffering  = 30757
	athrea            = 30758
	kalis             = 30759
)

// Monsters.
const (
	vanorSilenosShaman   = 20685
	haritLizardmanShaman = 20644
	oelMahumWitchDoctor  = 20576
)

// The chat line of a special box that was opened.
const sayBox = 50110

// The poison of Kalis and the one of the Statue of Offering.
const (
	skillPoison       = 4082
	skillStatuePoison = 4083
)

// boxesOfAthrea are the monsters that stand in the boxes' places.
func boxesOfAthrea() []int32 { return []int32{27173, 27174, 27175, 27176, 27177} }

// spawnList is where the boxes of Athrea are spawned.
func spawnList() []script.Loc {
	return []script.Loc{
		{X: 102273, Y: 103433, Z: -3512},
		{X: 102190, Y: 103379, Z: -3524},
		{X: 102107, Y: 103325, Z: -3533},
		{X: 102024, Y: 103271, Z: -3500},
		{X: 102327, Y: 103350, Z: -3511},
		{X: 102244, Y: 103296, Z: -3518},
		{X: 102161, Y: 103242, Z: -3529},
		{X: 102078, Y: 103188, Z: -3500},
		{X: 102381, Y: 103267, Z: -3538},
		{X: 102298, Y: 103213, Z: -3532},
		{X: 102215, Y: 103159, Z: -3520},
		{X: 102132, Y: 103105, Z: -3513},
		{X: 102435, Y: 103184, Z: -3515},
		{X: 102352, Y: 103130, Z: -3522},
		{X: 102269, Y: 103076, Z: -3533},
		{X: 102186, Y: 103022, Z: -3541},
	}
}

// quest holds the boxes of Athrea that stand in the world.
type quest struct {
	boxes *script.ListCell[*script.NPC]
}

// New returns the quest.
func New() script.Script {
	q := quest{boxes: new(script.ListCell[*script.NPC])}
	return script.Script{
		Title:   "Proof Of Clan Alliance",
		QuestID: 501,
		Items: []int32{
			symbolOfLoyalty, herbOfHarit, herbOfVanor, herbOfOelMahum, bloodOfEva, voucherOfFaith, antidoteRecipeList,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {sirKristofRodemai, kalis, statueOfOffering, athrea},
			script.EventTalked:     {sirKristofRodemai, kalis, statueOfOffering, athrea},
			script.EventDecayed:    boxesOfAthrea(),
			script.EventMyDying: {
				vanorSilenosShaman, haritLizardmanShaman, oelMahumWitchDoctor,
				27173, 27174, 27175, 27176, 27177,
			},
		},
		Hooks: script.Hooks{
			OnEvent: q.onAdvEvent, OnTimer: onTimer, OnTalk: onTalk, OnDecayed: q.onDecayed, OnMyDying: q.onMyDying,
		},
	}
}

func (q quest) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	p := e.Player
	st := s.QuestState(p, questName)
	if st == nil {
		return htmltext
	}
	switch {
	// Sir Kristof Rodemai
	case strings.EqualFold(e.Name, "30756-07.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	// Kalis
	case strings.EqualFold(e.Name, "30759-03.htm"):
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30759-07.htm"):
		// Flag the herbs and the Blood of Eva to be fetched.
		st.Set("3832", "true")
		st.Set("3833", "true")
		st.Set("3834", "true")
		st.Set("3835", "true")

		st.SetCond(3)
		st.Unset("sacrificed")
		s.TakeItems(p, symbolOfLoyalty, -1)
		s.GiveItems(p, antidoteRecipeList, 1)
		s.PlaySound(p, script.SoundMiddle)

		// The poison.
		e.NPC.AddCastDesire(p, skill.Ref{ID: skillPoison, Level: 1}, 1000000)
	// Statue of Offering
	case strings.EqualFold(e.Name, "30757-03.htm"):
		lst := s.CheckClanLeaderCondition(p, nil, 2)
		if lst == nil {
			return ""
		}

		if s.Rnd(100) < 40 {
			// The player is sacrificed and survives.
			htmltext = "30757-04.htm"
			s.GiveItems(p, symbolOfLoyalty, 1)
			s.PlaySound(p, script.SoundItemGet)
			sacrificed(lst, p)
		} else {
			// The player is sacrificed and dies.
			e.NPC.AddCastDesire(p, skill.Ref{ID: skillStatuePoison, Level: 1}, 100000)
			s.StartTimer("die", nil, p, 4000*time.Millisecond)
		}
	// Athrea
	case strings.EqualFold(e.Name, "30758-03.htm"):
		if q.boxes.Len() == 0 {
			lst := s.CheckClanLeaderVariable(p, nil, "3835", "true")
			if lst == nil {
				return ""
			}

			lst.Set("spawn", "true")
			lst.Set("boxes", strconv.Itoa(0))
			q.spawnBoxes(s, lst.Player().ClanID())
		} else {
			htmltext = "30758-03a.htm"
		}
	case strings.EqualFold(e.Name, "30758-07.htm"):
		if q.boxes.Len() == 0 {
			if p.Adena() < 10000 {
				htmltext = "30758-06.htm"
			} else {
				s.TakeItems(p, adena, 10000)
			}
		} else {
			htmltext = "30758-03a.htm"
		}
	}
	return htmltext
}

// sacrificed marks p as sacrificed in the leader's state lst.
func sacrificed(lst *script.QuestState, p *script.Player) {
	lst.Set("sacrificed", strconv.Itoa(int(lst.GetInt("sacrificed")+1)))
	lst.Set(strconv.Itoa(int(p.ObjectID())), "true")
}

// spawnBoxes fills every place of the spawn list with a box of Athrea, and
// marks four of them, picked apart, as the clan's special boxes.
func (q quest) spawnBoxes(s *script.Script, clanID int32) {
	var special [4]int
	places := spawnList()
	boxes := boxesOfAthrea()
	for {
		for i := range special {
			special[i] = s.Rnd(len(places))
		}
		if special[0] != special[1] && special[0] != special[2] && special[0] != special[3] &&
			special[1] != special[2] && special[1] != special[3] && special[2] != special[3] {
			break
		}
	}

	for i, at := range places {
		box := s.Spawner().AddSpawn(boxes[s.Rnd(len(boxes))], at, false, 300000*time.Millisecond)
		if box == nil {
			continue
		}
		q.boxes.Add(box)

		if i == special[0] || i == special[1] || i == special[2] || i == special[3] {
			box.SetScriptValue(clanID)
		}
	}
}

// onTimer pays the sacrifice of a player who dies of the Statue's poison.
func onTimer(s *script.Script, e script.Timer) string {
	p := e.Player
	if e.Name == "die" && p.Dead() {
		lst := s.CheckClanLeaderCondition(p, nil, 2)
		if lst == nil {
			return ""
		}

		s.GiveItems(p, symbolOfLoyalty, 1)
		s.PlaySound(p, script.SoundItemGet)
		sacrificed(lst, p)
	}
	return ""
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	p := e.Player
	lst := s.ClanLeaderQuestState(p, nil)
	if lst == nil {
		return htmltext
	}

	switch lst.Status() {
	case questlog.StatusCreated:
		if e.NPC.NpcID() == sirKristofRodemai {
			if p.IsClanLeader() {
				level := p.ClanLevel()
				switch {
				case level < 3:
					htmltext = "30756-01.htm"
				case level == 3:
					if p.HasItem(allianceManifesto) {
						htmltext = "30756-03.htm"
					} else {
						htmltext = "30756-04.htm"
					}
				default:
					htmltext = "30756-02.htm"
				}
			} else {
				htmltext = "30756-05.htm"
			}
		}
	case questlog.StatusStarted:
		cond := lst.Cond()
		leader := p.IsClanLeader()
		switch e.NPC.NpcID() {
		case sirKristofRodemai:
			if leader {
				if cond == 4 {
					s.RewardExpAndSp(p, 0, 120000)
					s.TakeItems(p, voucherOfFaith, -1)
					s.GiveItems(p, allianceManifesto, 1)
					s.PlaySound(p, script.SoundFinish)
					lst.Exit(true)
					htmltext = "30756-09.htm"
				} else {
					htmltext = "30756-10.htm"
				}
			}
		case kalis:
			if leader {
				switch cond {
				case 1:
					htmltext = "30759-01.htm"
				case 2:
					if p.ItemCount(symbolOfLoyalty) < 3 {
						htmltext = "30759-05.htm"
					} else if lst.GetInt("sacrificed") >= 3 {
						htmltext = "30759-06.htm"
					} else {
						resetKalis(s, lst)
						htmltext = "30759-09.htm"
					}
				case 3:
					switch {
					case !p.HasEffect(skillPoison):
						resetKalis(s, lst)
						htmltext = "30759-09.htm"
					case p.HasItems(herbOfHarit, herbOfVanor, herbOfOelMahum, bloodOfEva):
						if !lst.GetBool("3832") && !lst.GetBool("3833") && !lst.GetBool("3834") && !lst.GetBool("3835") {
							lst.SetCond(4)
							s.TakeItems(p, antidoteRecipeList, -1)
							s.TakeItems(p, herbOfHarit, -1)
							s.TakeItems(p, herbOfVanor, -1)
							s.TakeItems(p, herbOfOelMahum, -1)
							s.TakeItems(p, bloodOfEva, -1)
							s.GiveItems(p, voucherOfFaith, 1)
							s.GiveItems(p, potionOfRecovery, 1)
							s.PlaySound(p, script.SoundMiddle)
							htmltext = "30759-08.htm"
						} else {
							resetKalis(s, lst)
							htmltext = "30759-09.htm"
						}
					default:
						htmltext = "30759-10.htm"
					}
				case 4:
					htmltext = "30759-11.htm"
				}
			} else if cond >= 1 && cond <= 3 {
				htmltext = "30759-12.htm"
			}
		case statueOfOffering:
			if leader {
				if cond == 2 {
					htmltext = "30757-01a.htm"
				}
			} else {
				switch {
				case cond != 2:
					htmltext = "30757-06.htm"
				case p.Level() < 40:
					htmltext = "30757-02.htm"
				case lst.GetBool(strconv.Itoa(int(p.ObjectID()))):
					htmltext = "30757-01b.htm"
				default:
					htmltext = "30757-01.htm"
				}
			}
		case athrea:
			if !leader && cond == 3 {
				switch {
				case !lst.GetBool("3835"):
					htmltext = "30758-09.htm"
				case !lst.GetBool("spawn"):
					htmltext = "30758-01.htm"
				case lst.GetInt("boxes") < 4:
					htmltext = "30758-05.htm"
				default:
					lst.Unset("3835")
					lst.Unset("boxes")
					lst.Unset("spawn")

					s.GiveItems(p, bloodOfEva, 1)
					s.PlaySound(p, script.SoundItemGet)
					htmltext = "30758-08.htm"
				}
			}
		}
	}
	return htmltext
}

// onDecayed forgets a box that left the world.
func (q quest) onDecayed(_ *script.Script, e script.Decayed) {
	q.boxes.Remove(e.NPC)
}

// onMyDying pays the herbs to a clan whose leader asks for them, and counts
// the special boxes of the clan that opened them.
func (q quest) onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if p == nil {
		return
	}

	npcID := e.NPC.NpcID()
	switch npcID {
	case vanorSilenosShaman, haritLizardmanShaman, oelMahumWitchDoctor:
		var herbID int32
		switch npcID {
		case vanorSilenosShaman:
			herbID = herbOfVanor
		case haritLizardmanShaman:
			herbID = herbOfHarit
		case oelMahumWitchDoctor:
			herbID = herbOfOelMahum
		}
		herbFlag := strconv.Itoa(int(herbID))

		who, lst := randomPartyMemberClanLeaderState(s, p, e.NPC, herbFlag, "true")
		if who == nil {
			return
		}

		if s.DropItems(who, herbID, 1, 1, 100000) {
			lst.Unset(herbFlag)
		}
	default:
		who, lst := randomPartyMemberClanLeaderState(s, p, e.NPC, "3835", "true")
		if who == nil {
			return
		}

		// Only the clan that summoned the boxes gains progress from the special ones.
		if e.NPC.ScriptValue() == lst.Player().ClanID() {
			lst.Set("boxes", strconv.Itoa(int(lst.GetInt("boxes")+1)))

			e.NPC.Say(script.NpcString(sayBox))
		}
	}
}

// resetKalis ends the leader's quest state and starts it over at the
// sacrifice request of Kalis (cond 1).
func resetKalis(s *script.Script, lst *script.QuestState) {
	leader := lst.Player()
	lst.Exit(true)

	st := s.NewQuestState(leader)
	st.SetStatus(questlog.StatusStarted)
	st.SetCond(1)
	s.PlaySound(leader, script.SoundMiddle)
}

// randomPartyMemberClanLeaderState returns a random member of p's party, p
// itself without a party, who stands near npc and whose clan leader's state
// holds the variable key at value, with that state; nil when there is none.
// Only the member's distance counts, the leader may stand anywhere.
func randomPartyMemberClanLeaderState(s *script.Script, p *script.Player, npc *script.NPC, key, value string) (*script.Player, *script.QuestState) {
	members := p.PartyMembers()
	if len(members) == 0 {
		if !s.InPartyRange(p, npc) {
			return nil, nil
		}

		lst := s.CheckClanLeaderVariable(p, nil, key, value)
		if lst == nil {
			return nil, nil
		}
		return p, lst
	}

	var who []*script.Player
	var states []*script.QuestState
	for _, member := range members {
		if !s.InPartyRange(member, npc) {
			continue
		}

		lst := s.CheckClanLeaderVariable(member, nil, key, value)
		if lst == nil {
			continue
		}

		who = append(who, member)
		states = append(states, lst)
	}
	if len(who) == 0 {
		return nil, nil
	}

	i := s.Rnd(len(who))
	return who[i], states[i]
}
