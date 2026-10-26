// Package q227 is the quest Test of the Reformer, a second class-change test
// for clerics and Shillien oracles: Pupina sends the traveller to Sla, to the
// Ol Mahum pilgrim Katari's inspector and betrayer, to Kakan's werewolf, to
// Nyakuri's lizardman and to Ramus for the skeleton bone fragments, for the
// Mark of Reformer.
package q227

import (
	"slices"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/secondclass"
)

const questName = "Q227_TestOfTheReformer"

// Items.
const (
	bookOfReform         = 2822
	letterOfIntroduction = 2823
	slaLetter            = 2824
	greetings            = 2825
	olMahumMoney         = 2826
	katariLetter         = 2827
	nyakuriLetter        = 2828
	undeadList           = 2829
	ramusLetter          = 2830
	rippedDiary          = 2831
	hugeNail             = 2832
	letterOfBetrayer     = 2833
	boneFragment4        = 2834
	boneFragment5        = 2835
	boneFragment6        = 2836
	boneFragment7        = 2837
	boneFragment8        = 2838
	boneFragment9        = 2839
	kakanLetter          = 3037
)

// Rewards.
const markOfReformer = 2821

// NPCs.
const (
	pupina         = 30118
	sla            = 30666
	ramus          = 30667
	katari         = 30668
	kakan          = 30669
	nyakuri        = 30670
	olMahumPilgrim = 30732
)

// Monsters.
const (
	miserySkeleton   = 20022
	skeletonArcher   = 20100
	skeletonMarksman = 20102
	skeletonLord     = 20104
	silentHorror     = 20404
	namelessRevenant = 27099
	aruraune         = 27128
	olMahumInspector = 27129
	olMahumBetrayer  = 27130
	crimsonWerewolf  = 27131
	krudelLizardman  = 27132
)

// The classes the test is open to.
const (
	classCleric         = 15
	classShillienOracle = 42
)

// The NpcStrings the Aruraune and the Crimson werewolf speak.
const (
	sayAruraune  = 22719
	sayWerewolf  = 22720
	attackWeight = 2000
)

// allowedSkills are the skills that can hit the Crimson werewolf without
// making it leave.
var allowedSkills = []skill.ID{1031, 1069, 1164, 1168, 1147, 1177, 1184, 1201, 1206}

// quest holds the pilgrims, the inspector, the betrayer, the werewolf and the
// lizardman the dialogs called, each shared by every player: one at a time,
// until it decays.
type quest struct {
	pilgrimKatari, inspector, betrayer, pilgrimKakan, werewolf, pilgrimNyakuri, lizardman *script.NPCCell
}

// New returns the quest.
func New() script.Script {
	q := quest{
		pilgrimKatari: new(script.NPCCell), inspector: new(script.NPCCell), betrayer: new(script.NPCCell),
		pilgrimKakan: new(script.NPCCell), werewolf: new(script.NPCCell), pilgrimNyakuri: new(script.NPCCell),
		lizardman: new(script.NPCCell),
	}
	return script.Script{
		Title:   "Test Of The Reformer",
		QuestID: 227,
		Items: []int32{
			bookOfReform, letterOfIntroduction, slaLetter, greetings, olMahumMoney, katariLetter, nyakuriLetter,
			undeadList, ramusLetter, rippedDiary, hugeNail, letterOfBetrayer, boneFragment4, boneFragment5,
			boneFragment6, boneFragment7, boneFragment8, boneFragment9, kakanLetter,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {pupina},
			script.EventTalked:     {pupina, sla, ramus, katari, kakan, nyakuri, olMahumPilgrim},
			script.EventAttacked:   {namelessRevenant, crimsonWerewolf, olMahumPilgrim},
			script.EventDecayed:    {olMahumPilgrim, olMahumInspector, olMahumBetrayer, crimsonWerewolf, krudelLizardman},
			script.EventMyDying: {
				miserySkeleton, skeletonArcher, skeletonMarksman, skeletonLord, silentHorror, namelessRevenant,
				aruraune, olMahumInspector, olMahumBetrayer, crimsonWerewolf, krudelLizardman,
			},
		},
		Hooks: script.Hooks{
			OnEvent: q.onAdvEvent, OnTimer: q.onTimer, OnTalk: q.onTalk, OnAttacked: onAttacked,
			OnDecayed: q.onDecayed, OnMyDying: onMyDying,
		},
	}
}

// keep holds the NPC a dialog just called in cell; one another player's
// dialog placed meanwhile wins, and the extra one leaves. It returns the NPC
// the cell holds.
func keep(cell *script.NPCCell, n *script.NPC) *script.NPC {
	if n != nil && !cell.SetIfEmpty(n) {
		n.DeleteMe()
	}
	return cell.Get()
}

// call places the NPC npcID at at for 300 seconds when cell holds none.
func call(s *script.Script, cell *script.NPCCell, npcID int32, at script.Loc) *script.NPC {
	if cell.Empty() {
		keep(cell, s.Spawner().AddSpawn(npcID, at, false, 300000*time.Millisecond))
	}
	return cell.Get()
}

// setOn makes attacker hate victim, when both are there.
func setOn(attacker, victim *script.NPC) {
	if attacker != nil && victim != nil {
		attacker.AddAttackDesire(victim, attackWeight)
	}
}

func (q quest) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	// Pupina.
	case strings.EqualFold(e.Name, "30118-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, bookOfReform, 1)
		if secondclass.GiveDimensionalDiamonds39(s, p) {
			htmltext = "30118-04b.htm"
		}
	case strings.EqualFold(e.Name, "30118-06.htm"):
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, bookOfReform, 1)
		s.TakeItems(p, hugeNail, 1)
		s.GiveItems(p, letterOfIntroduction, 1)
	// Sla.
	case strings.EqualFold(e.Name, "30666-04.htm"):
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, letterOfIntroduction, 1)
		s.GiveItems(p, slaLetter, 1)
	// Kakan.
	case strings.EqualFold(e.Name, "30669-03.htm"):
		if st.Cond() != 12 {
			st.SetCond(12)
			s.PlaySound(p, script.SoundMiddle)
		}
		pilgrim := call(s, q.pilgrimKakan, olMahumPilgrim, script.Loc{X: -9282, Y: -89975, Z: -2331})
		werewolf := call(s, q.werewolf, crimsonWerewolf, script.Loc{X: -9382, Y: -89852, Z: -2333})
		setOn(werewolf, pilgrim)
	// Nyakuri.
	case strings.EqualFold(e.Name, "30670-03.htm"):
		st.SetCond(15)
		s.PlaySound(p, script.SoundMiddle)
		pilgrim := call(s, q.pilgrimNyakuri, olMahumPilgrim, script.Loc{X: 125947, Y: -180049, Z: -1778})
		lizardman := call(s, q.lizardman, krudelLizardman, script.Loc{X: 126019, Y: -179983, Z: -1781})
		setOn(lizardman, pilgrim)
	}
	return htmltext
}

// onTimer makes the Crimson werewolf leave, once a hit that is not one of the
// allowed skills has told it to.
func (q quest) onTimer(_ *script.Script, e script.Timer) string {
	if strings.EqualFold(e.Name, "werewolf_despawn") {
		if w := q.werewolf.Get(); w != nil && script.Same(e.NPC, w) {
			e.NPC.Say(script.NpcString(sayWerewolf))
			e.NPC.StopAttack()
			e.NPC.DeleteMe()
		}
	}
	return ""
}

func (q quest) onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		if class := p.ClassID(); class == classCleric || class == classShillienOracle {
			if p.Level() < 39 {
				htmltext = "30118-01.htm"
			} else {
				htmltext = "30118-03.htm"
			}
		} else {
			htmltext = "30118-02.htm"
		}
	case questlog.StatusStarted:
		htmltext = q.talkStarted(s, e.NPC.NpcID(), p, st)
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// talkStarted is the page npcID gives a started test, with the step it
// takes.
func (q quest) talkStarted(s *script.Script, npcID int32, p *script.Player, st *script.QuestState) string {
	htmltext := script.NoQuestMsg()
	cond := st.Cond()
	switch npcID {
	case pupina:
		switch {
		case cond < 3:
			htmltext = "30118-04a.htm"
		case cond == 3:
			htmltext = "30118-05.htm"
		case cond > 3:
			htmltext = "30118-07.htm"
		}
	case sla:
		switch {
		case cond == 4:
			htmltext = "30666-01.htm"
		case cond > 4 && cond < 10:
			htmltext = "30666-05.htm"
		case cond == 10:
			htmltext = "30666-06.htm"
			st.SetCond(11)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, olMahumMoney, 1)
			s.GiveItems(p, greetings, 3)
		case cond > 10 && cond < 20:
			htmltext = "30666-06.htm"
		case cond == 20:
			htmltext = "30666-07.htm"
			s.TakeItems(p, katariLetter, 1)
			s.TakeItems(p, kakanLetter, 1)
			s.TakeItems(p, nyakuriLetter, 1)
			s.TakeItems(p, ramusLetter, 1)
			s.GiveItems(p, markOfReformer, 1)
			s.RewardExpAndSp(p, 164032, 17500)
			p.SocialAction(3)
			s.PlaySound(p, script.SoundFinish)
			st.Exit(false)
		}
	case katari:
		switch {
		case cond == 5 || cond == 6 || cond == 7:
			htmltext = "30668-01.htm"
			if cond == 5 {
				st.SetCond(6)
				s.PlaySound(p, script.SoundMiddle)
				s.TakeItems(p, slaLetter, 1)
			}
			pilgrim := call(s, q.pilgrimKatari, olMahumPilgrim, script.Loc{X: -4015, Y: 40141, Z: -3664})
			inspector := call(s, q.inspector, olMahumInspector, script.Loc{X: -4034, Y: 40201, Z: -3665})
			setOn(inspector, pilgrim)
		case cond == 8:
			htmltext = "30668-02.htm"
			if q.betrayer.Empty() {
				if betrayer := keep(q.betrayer, s.Spawner().AddSpawn(olMahumBetrayer,
					script.Loc{X: -4106, Y: 40174, Z: -3660}, false, 60000*time.Millisecond)); betrayer != nil {
					betrayer.SetRunning(true)
					betrayer.AddMoveToDesire(script.Loc{X: -7732, Y: 36787, Z: -3709}, 10000)
				}
			}
		case cond == 9:
			htmltext = "30668-03.htm"
			st.SetCond(10)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, letterOfBetrayer, 1)
			s.GiveItems(p, katariLetter, 1)
		case cond > 9:
			htmltext = "30668-04.htm"
		}
	case olMahumPilgrim:
		if cond == 7 {
			htmltext = "30732-01.htm"
			st.SetCond(8)
			s.PlaySound(p, script.SoundMiddle)
			s.GiveItems(p, olMahumMoney, 1)
		}
	case kakan:
		switch {
		case cond == 11 || cond == 12:
			htmltext = "30669-01.htm"
		case cond == 13:
			htmltext = "30669-04.htm"
			st.SetCond(14)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, greetings, 1)
			s.GiveItems(p, kakanLetter, 1)
		case cond > 13:
			htmltext = "30669-04.htm"
		}
	case nyakuri:
		switch {
		case cond == 14 || cond == 15:
			htmltext = "30670-01.htm"
		case cond == 16:
			htmltext = "30670-04.htm"
			st.SetCond(17)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, greetings, 1)
			s.GiveItems(p, nyakuriLetter, 1)
		case cond > 16:
			htmltext = "30670-04.htm"
		}
	case ramus:
		switch {
		case cond == 17:
			htmltext = "30667-01.htm"
			st.SetCond(18)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, greetings, 1)
			s.GiveItems(p, undeadList, 1)
		case cond == 18:
			htmltext = "30667-02.htm"
		case cond == 19:
			htmltext = "30667-03.htm"
			st.SetCond(20)
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, boneFragment4, 1)
			s.TakeItems(p, boneFragment5, 1)
			s.TakeItems(p, boneFragment6, 1)
			s.TakeItems(p, boneFragment7, 1)
			s.TakeItems(p, boneFragment8, 1)
			s.TakeItems(p, undeadList, 1)
			s.GiveItems(p, ramusLetter, 1)
		case cond > 19:
			htmltext = "30667-03.htm"
		}
	}
	return htmltext
}

// onAttacked makes the werewolf, the lizardman and the pilgrims hate whoever
// hits them, and the Crimson werewolf leave when a player on the Kakan errand
// hits it with anything but an allowed skill; a Nameless revenant hit with
// the skill 1031 is marked.
func onAttacked(s *script.Script, e script.Attacked) {
	switch e.NPC.NpcID() {
	case crimsonWerewolf, krudelLizardman, olMahumPilgrim:
		e.NPC.AddAttackDesire(e.Attacker, attackWeight)
	}
	p := script.ActingPlayer(e.Attacker)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	cond := st.Cond()
	switch e.NPC.NpcID() {
	case namelessRevenant:
		if (cond == 1 || cond == 2) && e.Skill.ID == 1031 {
			e.NPC.SetScriptValue(1)
		}
	case crimsonWerewolf:
		if cond == 12 && e.NPC.IsScriptValue(0) && !slices.Contains(allowedSkills, e.Skill.ID) {
			e.NPC.SetScriptValue(1)
			s.StartTimer("werewolf_despawn", e.NPC, nil, 1000*time.Millisecond)
		}
	}
}

// onDecayed lets the dialogs call a pilgrim, an inspector, a betrayer, a
// werewolf or a lizardman again once the one that stood has left.
func (q quest) onDecayed(_ *script.Script, e script.Decayed) {
	for _, cell := range []*script.NPCCell{
		q.pilgrimKatari, q.inspector, q.betrayer, q.pilgrimKakan, q.werewolf, q.pilgrimNyakuri, q.lizardman,
	} {
		if cell.ClearIf(e.NPC) {
			return
		}
	}
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	cond := st.Cond()
	// fragment is the step a skeleton's or horror's death takes: it gives the
	// bone fragment held by no one yet, and the cond moves on once the other
	// four are held.
	fragment := func(item int32, others ...int32) {
		if cond != 18 || p.HasItem(item) {
			return
		}
		s.GiveItems(p, item, 1)
		if p.HasItems(others...) {
			st.SetCond(19)
			s.PlaySound(p, script.SoundMiddle)
		} else {
			s.PlaySound(p, script.SoundItemGet)
		}
	}
	switch e.NPC.NpcID() {
	case namelessRevenant:
		if (cond == 1 || cond == 2) && e.NPC.IsScriptValue(1) && s.DropItemsAlways(p, rippedDiary, 1, 7) {
			st.SetCond(2)
			s.TakeItems(p, rippedDiary, -1)
			s.Spawner().AddSpawn(aruraune, e.NPC, false, 300000*time.Millisecond)
		}
	case aruraune:
		if cond == 2 {
			st.SetCond(3)
			s.PlaySound(p, script.SoundMiddle)
			s.GiveItems(p, hugeNail, 1)
			e.NPC.Say(script.NpcString(sayAruraune))
		}
	case olMahumInspector:
		if cond == 6 {
			st.SetCond(7)
			s.PlaySound(p, script.SoundMiddle)
		}
	case olMahumBetrayer:
		if cond == 8 {
			st.SetCond(9)
			s.PlaySound(p, script.SoundMiddle)
			s.GiveItems(p, letterOfBetrayer, 1)
		}
	case crimsonWerewolf:
		if cond == 12 && e.NPC.IsScriptValue(0) {
			st.SetCond(13)
			s.PlaySound(p, script.SoundMiddle)
		}
	case krudelLizardman:
		if cond == 15 {
			st.SetCond(16)
			s.PlaySound(p, script.SoundMiddle)
		}
	case silentHorror:
		fragment(boneFragment4, boneFragment5, boneFragment6, boneFragment7, boneFragment8)
	case skeletonLord:
		fragment(boneFragment5, boneFragment4, boneFragment6, boneFragment7, boneFragment8)
	case skeletonMarksman:
		fragment(boneFragment6, boneFragment4, boneFragment5, boneFragment7, boneFragment8)
	case miserySkeleton:
		fragment(boneFragment7, boneFragment4, boneFragment5, boneFragment6, boneFragment8)
	case skeletonArcher:
		fragment(boneFragment8, boneFragment4, boneFragment5, boneFragment6, boneFragment7)
	}
}
