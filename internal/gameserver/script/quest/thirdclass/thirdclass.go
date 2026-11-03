// Package thirdclass is what the third class-change sagas share: the
// trial from the class's master through six Tablets of Vision, the fights
// they open, and the Shrine of Loyalty monsters whose Marks of Halisha
// wake an Archon for the saga of whoever earns them. Each saga sets its
// own class, items, NPCs, monsters, spawn spots and lines (Saga) and
// registers in the Family the sagas and the core share.
package thirdclass

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// Items every saga uses.
const (
	iceCrystal          = 7080
	divineStoneOfWisdom = 7081
	bookOfGiants        = 6622
	adena               = 57
)

// shrineOfLoyalty are the Shrine of Loyalty monsters whose deaths give
// Marks of Halisha to the saga of the one credited.
func shrineOfLoyalty() []int32 { return []int32{21646, 21647, 21648, 21649, 21650, 21651} }

// archonsOfFourSepulchers are the Archons of Halisha in the Four
// Sepulchers; each gives the fifth Resonance Amulet.
func archonsOfFourSepulchers() []int32 {
	return []int32{18212, 18213, 18214, 18215, 18216, 18217, 18218, 18219}
}

// The skills whose animation the sagas show: the tablets' resonance and
// the class change.
const (
	resonanceSkill   = 4546
	classChangeSkill = 4339
)

// The quest variables of a saga.
const (
	varAttacks = "attacks"
	varKills   = "kills"
)

// Saga is what one saga sets: its quest, the class it leads to, and its
// items, NPCs, monsters, spawn spots and NpcStrings.
type Saga struct {
	// Name is the saga's scripts.xml name, which keys its journal rows.
	Name    string
	QuestID int32
	Title   string
	// ClassID is the third class the saga leads to; the saga is open to
	// its parent class.
	ClassID int32

	// ItemMain marks the saga; ItemOptional (0 for none) goes with the
	// Ice Crystal to the second NPC for ItemReward, which the first NPC
	// trades for the first amulet. ItemHalishaMark drops in the Shrine of
	// Loyalty.
	ItemMain, ItemOptional, ItemReward                                int32
	ItemAmulet1st, ItemAmulet2nd, ItemAmulet3rd, ItemAmulet4th        int32
	ItemHalishaMark, ItemAmulet5th, ItemAmulet6th                     int32
	NPCMain, NPC1st, NPC2nd, NPC3rd, NPC4th                           int32
	NPCTablet1st, NPCTablet2nd, NPCTablet3rd                          int32
	NPCTablet4th, NPCTablet5th, NPCTablet6th                          int32
	NPCDefender                                                       int32
	MobGuardian, MobCorrupted, MobHalisha, MobAttacker                int32
	LocCorrupted, LocAttacker, LocDefender                            script.Loc
	MsgCorruptedSpawn, MsgCorruptedDespawn, MsgCorruptedKill          int32
	MsgHalishaSpawn, MsgHalishaDespawn, MsgHalishaKill                int32
	MsgHalishaKillOther                                               int32
	MsgAttackerSpawn, MsgAttackerDespawn                              int32
	MsgAttackerAttack1, MsgAttackerAttack16                           int32
	MsgDefenderSpawn, MsgDefenderDespawnWon, MsgDefenderDespawnLost   int32
	MsgDefenderCombat, MsgDefenderCombatIdle1, MsgDefenderCombatIdle2 int32
	MsgDefenderReward                                                 int32
}

// Family is the state the core and every saga share: each saga by the
// class it is open to, each tablet's fight by the tablet, and each
// defender's attacker by the defender. The catalog builds one and hands it
// to every member.
type Family struct {
	quests script.MapCell[int32, Saga]
	busy   script.NPCMapCell
	spawns script.NPCMapCell
}

// NewFamily returns an empty family.
func NewFamily() *Family { return &Family{} }

// quest is one member: its saga and the class it is open to. The core has
// no saga and no class: every player's class differs from its -1.
type quest struct {
	f         *Family
	sg        Saga
	prevClass int32
}

// NewCore returns the core: it binds the Shrine of Loyalty monsters alone,
// and credits each death to the saga of the one it picks.
func NewCore(f *Family) script.Script {
	q := quest{f: f, prevClass: -1}
	return script.Script{
		Title:   "Third Class Quest",
		QuestID: -1,
		Bind:    script.Bindings{script.EventMyDying: shrineOfLoyalty()},
		Hooks:   q.hooks(),
	}
}

// New returns the saga sg and registers it in f under the class it is open
// to.
func New(f *Family, sg Saga) script.Script {
	q := quest{f: f, sg: sg, prevClass: script.ClassParent(sg.ClassID)}
	f.quests.Set(q.prevClass, sg)
	return script.Script{
		Title:   sg.Title,
		QuestID: sg.QuestID,
		Items: []int32{
			sg.ItemMain, sg.ItemReward, sg.ItemAmulet1st, sg.ItemAmulet2nd, sg.ItemAmulet3rd,
			sg.ItemAmulet4th, sg.ItemHalishaMark, sg.ItemAmulet5th, sg.ItemAmulet6th,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {sg.NPCMain},
			script.EventFirstTalk:  {sg.NPCDefender},
			script.EventTalked: {
				sg.NPCMain, sg.NPC1st, sg.NPC2nd, sg.NPCTablet1st, sg.NPCTablet2nd, sg.NPCTablet3rd,
				sg.NPC3rd, sg.NPCTablet4th, sg.NPC4th, sg.NPCTablet5th, sg.NPCDefender, sg.NPCTablet6th,
			},
			script.EventAttacked: {sg.MobCorrupted, sg.MobAttacker, sg.NPCDefender},
			script.EventDecayed:  {sg.MobCorrupted, sg.MobHalisha, sg.NPCDefender, sg.MobAttacker},
			script.EventMyDying: append(
				[]int32{sg.MobGuardian, sg.MobCorrupted, sg.MobHalisha, sg.MobAttacker},
				archonsOfFourSepulchers()...),
			script.EventSeeSpell: {sg.MobCorrupted},
		},
		// A saga sets no hook of its own.
		Hooks: q.hooks().With(script.Hooks{}),
	}
}

// hooks are the hooks of the core and every saga.
func (q quest) hooks() script.Hooks {
	return script.Hooks{
		OnEvent:     q.onAdvEvent,
		OnTimer:     q.onTimer,
		OnTalk:      q.onTalk,
		OnFirstTalk: q.onFirstTalk,
		OnAttacked:  q.onAttacked,
		OnDecayed:   q.onDecayed,
		OnMyDying:   q.onMyDying,
		OnSeeSpell:  q.onSeeSpell,
	}.Named("ThirdClassQuest")
}

// cast shows the skill's animation on target, then on npc, to the players
// around target.
func cast(npc *script.NPC, target *script.Player, skillID, level int32) {
	target.BroadcastMagicSkillUse(target, target, skillID, level, 6000, 1)
	target.BroadcastMagicSkillUse(npc, npc, skillID, level, 6000, 1)
}

// finish rewards p, ends the saga and moves p to its third class: also its
// base class when p plays its base class and that is the saga's parent
// class.
func (q quest) finish(s *script.Script, st *script.QuestState, npc *script.NPC, p *script.Player) {
	s.GiveItems(p, adena, 5000000)
	s.GiveItems(p, bookOfGiants, 1)
	s.RewardExpAndSp(p, 2299404, 0)
	s.PlaySound(p, script.SoundFinish)
	st.Exit(false)

	p.SetClassID(q.sg.ClassID)
	if !p.SubClassActive() && p.BaseClass() == q.prevClass {
		p.SetBaseClass(q.sg.ClassID)
	}
	p.BroadcastUserInfo()

	cast(npc, p, classChangeSkill, 1)
}

func (q quest) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	p, npc := e.Player, e.NPC
	st := s.QuestState(p, s.Name)
	if st == nil {
		return ""
	}
	sg := q.sg
	switch {
	// Main quest NPC.
	case strings.EqualFold(e.Name, "0-1"):
		if p.Level() < 76 {
			htmltext = "0-02.htm"
		} else {
			htmltext = "0-05.htm"
		}
	case strings.EqualFold(e.Name, "accept"):
		htmltext = "0-03.htm"
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, sg.ItemMain, 1)
	case strings.EqualFold(e.Name, "0-2"):
		if p.Level() >= 76 {
			htmltext = "0-07.htm"
			q.finish(s, st, npc, p)
		} else {
			htmltext = "0-08.htm"
			st.SetCond(20)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, sg.ItemMain, -1)
		}
	// The first NPC trades the reward item for the first amulet.
	case strings.EqualFold(e.Name, "2-1"):
		htmltext = "2-05.htm"
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "2-2"):
		htmltext = "2-06.htm"
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, sg.ItemReward, 1)
		s.GiveItems(p, sg.ItemAmulet1st, 1)
	// The second NPC trades the Ice Crystal (and the optional item) for
	// the reward item.
	case strings.EqualFold(e.Name, "1-3"):
		htmltext = "1-05.htm"
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "1-4"):
		htmltext = "1-06.htm"
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, iceCrystal, 1)
		if sg.ItemOptional != 0 {
			s.TakeItems(p, sg.ItemOptional, 1)
		}
		s.GiveItems(p, sg.ItemReward, 1)
	// First Tablet of Vision.
	case strings.EqualFold(e.Name, "5-1"):
		htmltext = "5-02.htm"
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, sg.ItemAmulet1st, 1)
		cast(npc, p, resonanceSkill, 1)
	// Second Tablet of Vision.
	case strings.EqualFold(e.Name, "6-1"):
		htmltext = "6-03.htm"
		st.SetCond(8)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, sg.ItemAmulet2nd, 1)
		cast(npc, p, resonanceSkill, 1)
	// Third Tablet of Vision: one corrupted knight at a time, the private
	// of the tablet, bound to the player who woke it.
	case strings.EqualFold(e.Name, "7-1"):
		if corrupted := q.f.busy.Get(npc); corrupted == nil {
			corrupted = s.Spawner().CreateOnePrivateEx(npc, sg.MobCorrupted, sg.LocCorrupted, 0,
				script.Params{P1: p.ObjectID(), P2: p.ObjectID(), P3: npc.ObjectID()})
			corrupted.SetScriptValue(p.ObjectID())
			q.f.busy.Set(npc, corrupted)
			s.StartTimer("corrupted", corrupted, p, 500*time.Millisecond)
			htmltext = "7-02.htm"
		} else if corrupted.ScriptValue() == p.ObjectID() {
			htmltext = "7-03.htm"
		} else {
			htmltext = "7-04.htm"
		}
	case strings.EqualFold(e.Name, "7-2"):
		htmltext = "7-06.htm"
		st.SetCond(10)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, sg.ItemAmulet3rd, 1)
		cast(npc, p, resonanceSkill, 1)
	// The third NPC trades the Divine Stone of Wisdom for the fourth
	// amulet.
	case strings.EqualFold(e.Name, "3-5"):
		htmltext = "3-07.htm"
	case strings.EqualFold(e.Name, "3-6"):
		htmltext = "3-02.htm"
		st.SetCond(11)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "3-7"):
		htmltext = "3-03.htm"
		st.SetCond(12)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "3-8"):
		htmltext = "3-08.htm"
		st.SetCond(13)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, divineStoneOfWisdom, 1)
		s.GiveItems(p, sg.ItemAmulet4th, 1)
	// Fourth Tablet of Vision.
	case strings.EqualFold(e.Name, "8-1"):
		htmltext = "8-02.htm"
		st.SetCond(14)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, sg.ItemAmulet4th, 1)
		cast(npc, p, resonanceSkill, 1)
	// The fourth NPC sends the player after Halisha.
	case strings.EqualFold(e.Name, "11-9"):
		htmltext = "11-03.htm"
		st.SetCond(15)
		s.PlaySound(p, script.SoundMiddle)
	// Fifth Tablet of Vision.
	case strings.EqualFold(e.Name, "9-1"):
		htmltext = "9-03.htm"
		st.SetCond(17)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, sg.ItemAmulet5th, 1)
		cast(npc, p, resonanceSkill, 1)
	// Sixth Tablet of Vision: one defender and its attacker at a time,
	// bound to the player who called them.
	case strings.EqualFold(e.Name, "10-1"):
		if defender := q.f.busy.Get(npc); defender == nil {
			st.Unset(varAttacks)

			defender = s.Spawner().AddSpawn(sg.NPCDefender, sg.LocDefender, false, 60000*time.Millisecond)
			defender.SetScriptValue(p.ObjectID())

			attacker := s.Spawner().AddSpawn(sg.MobAttacker, sg.LocAttacker, false, 59000*time.Millisecond)
			attacker.SetScriptValue(p.ObjectID())
			attacker.AddAttackDesire(defender, 200)

			q.f.busy.Set(npc, defender)
			q.f.spawns.Set(defender, attacker)

			s.StartTimer("defender", defender, p, 500*time.Millisecond)
			s.StartTimer("attacker", attacker, p, 500*time.Millisecond)

			htmltext = "10-02.htm"
		} else if defender.ScriptValue() == p.ObjectID() {
			htmltext = "10-03.htm"
		} else {
			htmltext = "10-04.htm"
		}
	case strings.EqualFold(e.Name, "10-2"):
		htmltext = "10-06.htm"
		st.SetCond(19)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, sg.ItemAmulet6th, 1)
		cast(npc, p, resonanceSkill, 1)
	// The defender of the fight at the sixth tablet.
	case strings.EqualFold(e.Name, "4-1"):
		htmltext = "4-010.htm"
	case strings.EqualFold(e.Name, "4-2"):
		htmltext = "4-011.htm"
		st.Unset(varAttacks)
		st.SetCond(18)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, sg.ItemAmulet6th, 1)
	case strings.EqualFold(e.Name, "4-3"):
		htmltext = ""
		st.Unset(varAttacks)
		st.SetCond(18)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, sg.ItemAmulet6th, 1)

		npc.Say(script.NpcString(sg.MsgDefenderReward))
		s.CancelTimers(script.TimerName("defender2"), script.TimerNPC(npc))
		s.CancelTimers(script.TimerName("defender3"), script.TimerNPC(npc))
		npc.DeleteMe()
	}
	return htmltext
}

// onTimer says the lines of the fights' NPCs.
func (q quest) onTimer(s *script.Script, e script.Timer) string {
	sg, npc, p := q.sg, e.NPC, e.Player
	switch {
	// The corrupted knight appears.
	case strings.EqualFold(e.Name, "corrupted"):
		npc.Say(script.NpcString(sg.MsgCorruptedSpawn, p.Name()))
	// The defender appears, then cheers the player on every 10 s while
	// its attacker stands.
	case strings.EqualFold(e.Name, "defender"):
		npc.Say(script.NpcString(sg.MsgDefenderSpawn))
		s.StartTimer("defender2", npc, p, 1500*time.Millisecond)
	case strings.EqualFold(e.Name, "defender2"):
		npc.Say(script.NpcString(sg.MsgDefenderCombat, p.Name()))
		s.StartTimer("defender3", npc, p, 10000*time.Millisecond)
	case strings.EqualFold(e.Name, "defender3"):
		if s.CheckPlayerCondition(p, npc, 17) == nil {
			return ""
		}
		if q.f.spawns.Has(npc) {
			msg := sg.MsgDefenderCombatIdle2
			if s.RndBool() {
				msg = sg.MsgDefenderCombatIdle1
			}
			npc.Say(script.NpcString(msg, p.Name()))
			s.StartTimer("defender3", npc, p, 10000*time.Millisecond)
		}
	// The attacker appears.
	case strings.EqualFold(e.Name, "attacker"):
		npc.Say(script.NpcString(sg.MsgAttackerSpawn, p.Name()))
	}
	return ""
}

func (q quest) onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	p, sg := e.Player, q.sg
	st := s.QuestState(p, s.Name)
	if st == nil {
		return htmltext
	}
	npcID := e.NPC.NpcID()
	switch st.Status() {
	case questlog.StatusCreated:
		if npcID == sg.NPCMain {
			if p.ClassID() == q.prevClass {
				htmltext = "0-01.htm"
			} else {
				htmltext = "0-02.htm"
			}
		}
	case questlog.StatusStarted:
		// A player who no longer plays the saga's parent class sees no
		// quest.
		if p.ClassID() != q.prevClass {
			return htmltext
		}
		switch st.Cond() {
		case 1:
			if npcID == sg.NPCMain {
				htmltext = "0-04.htm"
			} else if npcID == sg.NPC1st {
				htmltext = "2-01.htm"
			}
		case 2:
			if npcID == sg.NPC1st {
				htmltext = "2-02.htm"
			} else if npcID == sg.NPC2nd {
				htmltext = "1-01.htm"
			}
		case 3:
			if npcID == sg.NPC1st {
				htmltext = "2-02.htm"
			} else if npcID == sg.NPC2nd {
				if p.HasItem(iceCrystal) && (sg.ItemOptional == 0 || p.HasItem(sg.ItemOptional)) {
					htmltext = "1-03.htm"
				} else {
					htmltext = "1-02.htm"
				}
			}
		case 4:
			if npcID == sg.NPC2nd {
				htmltext = "1-04.htm"
			} else if npcID == sg.NPC1st {
				htmltext = "2-03.htm"
			}
		case 5:
			if npcID == sg.NPC1st {
				htmltext = "2-04.htm"
			} else if npcID == sg.NPCTablet1st {
				htmltext = "5-01.htm"
			}
		case 6:
			if npcID == sg.NPCTablet1st {
				htmltext = "5-03.htm"
			} else if npcID == sg.NPCTablet2nd {
				htmltext = "6-01.htm"
			}
		case 7:
			if npcID == sg.NPCTablet2nd {
				htmltext = "6-02.htm"
			}
		case 8:
			if npcID == sg.NPCTablet2nd {
				htmltext = "6-04.htm"
			} else if npcID == sg.NPCTablet3rd {
				htmltext = "7-01.htm"
			}
		case 9:
			if npcID == sg.NPCTablet3rd {
				htmltext = "7-05.htm"
			}
		case 10:
			if npcID == sg.NPCTablet3rd {
				htmltext = "7-07.htm"
			} else if npcID == sg.NPC3rd {
				htmltext = "3-01.htm"
			}
		case 11, 12:
			if npcID == sg.NPC3rd {
				if p.HasItem(divineStoneOfWisdom) {
					htmltext = "3-05.htm"
				} else {
					htmltext = "3-04.htm"
				}
			}
		case 13:
			if npcID == sg.NPC3rd {
				htmltext = "3-06.htm"
			} else if npcID == sg.NPCTablet4th {
				htmltext = "8-01.htm"
			}
		case 14:
			if npcID == sg.NPCTablet4th {
				htmltext = "8-03.htm"
			} else if npcID == sg.NPC4th {
				htmltext = "11-01.htm"
			}
		case 15:
			if npcID == sg.NPC4th {
				htmltext = "11-02.htm"
			} else if npcID == sg.NPCTablet5th {
				htmltext = "9-01.htm"
			}
		case 16:
			if npcID == sg.NPCTablet5th {
				htmltext = "9-02.htm"
			}
		case 17:
			if npcID == sg.NPCTablet5th {
				htmltext = "9-04.htm"
			} else if npcID == sg.NPCTablet6th {
				htmltext = "10-01.htm"
			}
		case 18:
			if npcID == sg.NPCTablet6th {
				htmltext = "10-05.htm"
			}
		case 19:
			if npcID == sg.NPCTablet6th {
				htmltext = "10-07.htm"
			} else if npcID == sg.NPCMain {
				htmltext = "0-06.htm"
			}
		case 20:
			if npcID == sg.NPCMain {
				// The saga is done; the class changes at level 76.
				if p.Level() >= 76 {
					htmltext = "0-09.htm"
					q.finish(s, st, e.NPC, p)
				} else {
					htmltext = "0-010.htm"
				}
			}
		}
	case questlog.StatusCompleted:
		if npcID == sg.NPCMain {
			htmltext = script.AlreadyCompletedMsg()
		}
	}
	return htmltext
}

// onFirstTalk answers the defender's first dialog from the fight's state:
// whether its attacker stands, who called it, and whether the player
// landed the sixteen hits.
func (q quest) onFirstTalk(s *script.Script, e script.FirstTalk) string {
	htmltext := script.NoQuestMsg()
	npc, p := e.NPC, e.Player
	st := s.CheckPlayerState(p, npc, questlog.StatusStarted)
	if st == nil {
		return htmltext
	}
	switch st.Cond() {
	case 17:
		switch {
		case q.f.spawns.Has(npc):
			// The attacker stands: the player's own fight, or another's.
			if npc.ScriptValue() == p.ObjectID() {
				if st.GetInt(varAttacks) < 16 {
					htmltext = "4-01.htm"
				} else {
					htmltext = "4-04.htm"
				}
			} else if st.GetInt(varAttacks) < 16 {
				htmltext = "4-02.htm"
			} else {
				htmltext = "4-05.htm"
			}
		case st.GetInt(varAttacks) < 16:
			htmltext = "4-03.htm"
		case npc.ScriptValue() == p.ObjectID():
			htmltext = "4-06.htm"
		default:
			htmltext = "4-07.htm"
		}
	case 18:
		if q.f.spawns.Has(npc) {
			htmltext = "4-08.htm"
		} else {
			htmltext = "4-09.htm"
		}
	}
	return htmltext
}

// onAttacked sends away a corrupted knight struck by anyone but the player
// who woke it in the saga's class, counts the player's hits on its own
// attacker, and makes the defender fight back at a non-player attacker.
func (q quest) onAttacked(s *script.Script, e script.Attacked) {
	p := script.ActingPlayer(e.Attacker)
	if p == nil {
		return
	}
	npc, sg := e.NPC, q.sg
	switch npcID := npc.NpcID(); {
	case npcID == sg.MobCorrupted:
		if p.ObjectID() != npc.ScriptValue() || p.ClassID() != q.prevClass {
			// Gone at once, with no despawn line.
			npc.SetScriptValue(0)
			npc.DeleteMe()
		}
	case npcID == sg.MobAttacker:
		if p.ObjectID() != npc.ScriptValue() || p.ClassID() != q.prevClass {
			return
		}
		st := s.CheckPlayerCondition(p, npc, 17)
		if st == nil {
			return
		}
		attacks := st.GetInt(varAttacks) + 1
		st.Set(varAttacks, strconv.Itoa(int(attacks)))
		switch attacks {
		case 1:
			npc.Say(script.NpcString(sg.MsgAttackerAttack1, p.Name()))
		case 16:
			npc.Say(script.NpcString(sg.MsgAttackerAttack16, p.Name()))
			// Gone, with no despawn line.
			npc.SetScriptValue(0)
			npc.DeleteMe()
		}
	case npcID == sg.NPCDefender && !e.Attacker.Playable():
		npc.AddAttackDesire(e.Attacker, 2000)
	}
}

// onDecayed frees the tablets and defenders of the fights' NPCs as they
// leave. An NPC is dead by the time it decays, so only the defender says
// its line: whether the player it fought for won.
func (q quest) onDecayed(s *script.Script, e script.Decayed) {
	npc, sg := e.NPC, q.sg
	switch npcID := npc.NpcID(); npcID {
	case sg.MobCorrupted:
		q.f.busy.DeleteValue(npc)
		if !npc.Dead() && npc.ScriptValue() > 0 {
			npc.Say(script.NpcString(sg.MsgCorruptedDespawn))
		}
	case sg.MobHalisha:
		if !npc.Dead() {
			npc.Say(script.NpcString(sg.MsgHalishaDespawn))
		}
	case sg.MobAttacker:
		q.f.spawns.DeleteValue(npc)
		if !npc.Dead() && npc.ScriptValue() > 0 {
			npc.Say(script.NpcString(sg.MsgAttackerDespawn))
		}
	case sg.NPCDefender:
		q.f.busy.DeleteValue(npc)
		q.f.spawns.Delete(npc)

		p := s.WorldPlayer(npc.ScriptValue())
		if p == nil {
			npc.Say(script.NpcString(sg.MsgDefenderDespawnLost))
			return
		}
		if st := s.CheckPlayerCondition(p, npc, 17); st != nil {
			if st.GetInt(varAttacks) > 15 {
				npc.Say(script.NpcString(sg.MsgDefenderDespawnWon))
			} else {
				npc.Say(script.NpcString(sg.MsgDefenderDespawnLost))
			}
		}
	}
}

func (q quest) onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if p == nil {
		return
	}
	npc, sg := e.NPC, q.sg
	npcID := npc.NpcID()

	// The Guardians of Forbidden Knowledge at the second tablet: ten
	// kills give the second amulet.
	if npcID == sg.MobGuardian {
		if p.ClassID() != q.prevClass {
			return
		}
		st := s.CheckPlayerCondition(p, npc, 6)
		if st == nil {
			return
		}
		if kills := st.GetInt(varKills) + 1; kills < 10 {
			st.Set(varKills, strconv.Itoa(int(kills)))
		} else {
			st.Unset(varKills)
			st.SetCond(7)
			s.PlaySound(p, script.SoundMiddle)
			s.GiveItems(p, sg.ItemAmulet2nd, 1)
		}
		return
	}

	// The corrupted knight at the third tablet, killed by the player who
	// woke it.
	if npcID == sg.MobCorrupted {
		if p.ObjectID() != npc.ScriptValue() || p.ClassID() != q.prevClass {
			return
		}
		st := s.CheckPlayerCondition(p, npc, 8)
		if st == nil {
			return
		}
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, sg.ItemAmulet3rd, 1)
		npc.Say(script.NpcString(sg.MsgCorruptedKill))
		return
	}

	// The Shrine of Loyalty: a mark for the saga of the killer, or of one
	// party member picked among those at cond 15; 700 marks wake that
	// saga's Archon of Halisha instead, for 20 of them.
	if slices.Contains(shrineOfLoyalty(), npcID) {
		var cr credit
		if p.InParty() {
			var valid []credit
			for _, pm := range p.PartyMembers() {
				if c, ok := q.creditOf(s, pm, npc); ok {
					valid = append(valid, c)
				}
			}
			if len(valid) == 0 {
				return
			}
			cr = valid[s.Rnd(len(valid))]
		} else {
			var ok bool
			if cr, ok = q.creditOf(s, p, npc); !ok {
				return
			}
		}

		pl, tcq := cr.st.Player(), cr.saga
		if pl.ItemCount(tcq.ItemHalishaMark) < 700 {
			s.DropItemsAlways(pl, tcq.ItemHalishaMark, 1, 700)
		} else {
			s.TakeItems(pl, tcq.ItemHalishaMark, 20)

			archon := s.Spawner().AddSpawn(tcq.MobHalisha, npc, false, 600000*time.Millisecond)
			archon.SetScriptValue(pl.ObjectID())
			archon.AddAttackDesire(pl, 200)
			archon.Say(script.NpcString(tcq.MsgHalishaSpawn, pl.Name()))
		}
		return
	}

	// The Archon of Halisha the shrine woke, killed by the player it woke
	// for.
	if npcID == sg.MobHalisha {
		if p.ObjectID() != npc.ScriptValue() {
			npc.Say(script.NpcString(sg.MsgHalishaKillOther))
			return
		}
		if p.ClassID() != q.prevClass {
			return
		}
		st := s.CheckPlayerCondition(p, npc, 15)
		if st == nil {
			return
		}
		st.SetCond(16)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, sg.ItemHalishaMark, -1)
		s.GiveItems(p, sg.ItemAmulet5th, 1)
		npc.Say(script.NpcString(sg.MsgHalishaKill))
		return
	}

	// An Archon of Halisha in the Four Sepulchers: the fifth amulet for
	// the killer, or for every party member of the saga's class at cond
	// 15.
	if slices.Contains(archonsOfFourSepulchers(), npcID) {
		members := []*script.Player{p}
		if p.InParty() {
			members = p.PartyMembers()
		}
		for _, pm := range members {
			if pm.ClassID() != q.prevClass {
				continue
			}
			st := s.CheckPlayerCondition(pm, npc, 15)
			if st == nil {
				continue
			}
			st.SetCond(16)
			s.PlaySound(pm, script.SoundMiddle)
			s.TakeItems(pm, sg.ItemHalishaMark, -1)
			s.GiveItems(pm, sg.ItemAmulet5th, 1)
		}
		return
	}

	// The attacker at the sixth tablet: no despawn line once dead.
	if npcID == sg.MobAttacker {
		npc.SetScriptValue(0)
	}
}

// credit is a Shrine of Loyalty death's candidate: a player's state in the
// saga of its class, at cond 15.
type credit struct {
	st   *script.QuestState
	saga Saga
}

// creditOf returns pm's credit for the death of npc: its state in the saga
// registered for its class, when that state is at cond 15 and pm stands in
// range, as that saga checks it.
func (q quest) creditOf(s *script.Script, pm *script.Player, npc *script.NPC) (credit, bool) {
	tcq, ok := q.f.quests.Get(pm.ClassID())
	if !ok {
		return credit{}, false
	}
	st := s.QuestState(pm, tcq.Name)
	if st == nil {
		return credit{}, false
	}
	if st = st.Quest().CheckPlayerCondition(pm, npc, 15); st == nil {
		return credit{}, false
	}
	return credit{st: st, saga: tcq}, true
}

// onSeeSpell sends away a corrupted knight a spell targets, unless the
// caster is the player who woke it, in the saga's class.
func (q quest) onSeeSpell(_ *script.Script, e script.SeeSpell) {
	npc := e.NPC
	if e.Caster == nil || !slices.ContainsFunc(e.Targets, func(c script.Creature) bool { return script.Same(c, npc) }) {
		return
	}
	if q.sg.MobCorrupted == npc.NpcID() && (e.Caster.ObjectID() != npc.ScriptValue() || e.Caster.ClassID() != q.prevClass) {
		npc.SetScriptValue(0)
		npc.DeleteMe()
	}
}
