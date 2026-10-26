// Package q421 is the quest Little Wing's Big Adventure: the owner of a
// grown hatchling takes it to the four fairy trees, and the pet's attacks
// make each tree give up a leaf, to be given a stronger flute.
package q421

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q421_LittleWingsBigAdventure"

// Items.
const (
	dragonfluteOfWind     = 3500
	dragonfluteOfStar     = 3501
	dragonfluteOfTwilight = 3502
	fairyLeaf             = 4325
)

// Npcs.
const (
	cronos = 30610
	mimyu  = 30747
)

// Monsters.
const (
	fairyTreeOfWind     = 27185
	fairyTreeOfStar     = 27186
	fairyTreeOfTwilight = 27187
	fairyTreeOfAbyss    = 27188
	soulOfTreeGuardian  = 27189
)

// What the trees and the ghosts say.
const (
	sayLeafGiven     = 42111
	sayTreeAsking    = 42112
	sayTreeWind      = 42113
	sayTreeGone      = 42114
	sayTreeStar      = 42115
	sayTreeDusk      = 42116
	sayTreeAbyss     = 42117
	sayGuardianOne   = 42118
	sayGuardianTwo   = 42119
	sayGuardianThree = 42120
)

// The quest's internal condition: it is a bit set of the trees that took a
// leaf, below the values the talks with Mimyu step through.
const (
	allTrees     = 63
	acknowledged = 100
)

// The skills the trees cast.
func curseSkill() skill.Ref      { return skill.Ref{ID: 4243, Level: 1} }
func flutesCurse() skill.Ref     { return skill.Ref{ID: 4167, Level: 1} }
func dryadRootOnPets() skill.Ref { return skill.Ref{ID: 1201, Level: 33} }

const (
	castWeight   = 1000000
	attackWeight = 2000
	ghostLife    = 300000 * time.Millisecond
	ghostCount   = 20
)

// The pet of the flute gets a stronger flute by this many ids.
const strongerFluteOffset = 922

// A flute carries this enchant level at least to start the quest.
const minFluteEnchant = 55

// tree is what one fairy tree wants of the pet: the bit it sets, the attacks
// that earn a leaf, the chance a leaf is taken after them and what it says
// once its leaf is gone.
type tree struct {
	mask     int32
	attacks  int32
	chance   int
	messages [3]int32
}

func treeOf(npcID int32) (tree, bool) {
	switch npcID {
	case fairyTreeOfWind:
		return tree{4, 270, 3, [3]int32{sayTreeAsking, sayTreeGone, sayTreeWind}}, true
	case fairyTreeOfStar:
		return tree{8, 400, 2, [3]int32{sayTreeAsking, sayTreeGone, sayTreeStar}}, true
	case fairyTreeOfTwilight:
		return tree{16, 150, 2, [3]int32{sayTreeAsking, sayTreeGone, sayTreeDusk}}, true
	case fairyTreeOfAbyss:
		return tree{32, 270, 2, [3]int32{sayTreeAsking, sayTreeGone, sayTreeAbyss}}, true
	}
	return tree{}, false
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Little Wing's Big Adventure",
		QuestID: 421,
		Items:   []int32{fairyLeaf},
		Bind: script.Bindings{
			script.EventQuestStart: {cronos},
			script.EventTalked:     {cronos, mimyu},
			script.EventAttacked:   {fairyTreeOfWind, fairyTreeOfStar, fairyTreeOfTwilight, fairyTreeOfAbyss},
			script.EventCreated: {
				fairyTreeOfWind, fairyTreeOfStar, fairyTreeOfTwilight, fairyTreeOfAbyss, soulOfTreeGuardian,
			},
			script.EventMyDying: {fairyTreeOfWind, fairyTreeOfStar, fairyTreeOfTwilight, fairyTreeOfAbyss},
		},
		Hooks: script.Hooks{
			OnEvent: onAdvEvent, OnTalk: onTalk, OnAttacked: onAttacked, OnCreated: onCreated, OnMyDying: onMyDying,
		},
	}
}

func setInt(st *script.QuestState, key string, n int32) {
	st.Set(key, strconv.Itoa(int(n)))
}

// dragonfluteCount is how many flutes of every kind the player holds.
func dragonfluteCount(p *script.Player) int32 {
	return p.ItemCount(dragonfluteOfWind) + p.ItemCount(dragonfluteOfStar) + p.ItemCount(dragonfluteOfTwilight)
}

// fluteObject returns the object id of the player's first flute of itemID.
func fluteObject(p *script.Player, itemID int32) (int32, bool) {
	for _, it := range p.HeldItems() {
		if it.ItemID == itemID {
			return it.ObjectID, true
		}
	}
	return 0, false
}

// strongFlute reports whether the player holds a flute enchanted enough to
// start the quest.
func strongFlute(p *script.Player) (objectID int32, ok bool) {
	for id := int32(dragonfluteOfWind); id <= dragonfluteOfTwilight; id++ {
		if obj, held := fluteObject(p, id); held && p.ItemEnchantLevel(id) >= minFluteEnchant {
			return obj, true
		}
	}
	return 0, false
}

// summonIsTheFlutes reports whether the player's summon is the pet of the
// flute the quest registered.
func summonIsTheFlutes(p *script.Player, st *script.QuestState) bool {
	summon := p.ActiveSummon()
	return summon != nil && script.PetControlItemID(summon) == st.GetInt("summonOid")
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	// Cronos
	case strings.EqualFold(e.Name, "30610-06.htm"):
		if dragonfluteCount(p) == 1 {
			if obj, ok := strongFlute(p); ok {
				st.SetStatus(questlog.StatusStarted)
				st.SetCond(1)
				setInt(st, "iCond", 1)
				setInt(st, "summonOid", obj)
				s.PlaySound(p, script.SoundAccept)
				return "30610-05.htm"
			}
		}
	// Mimyu
	case strings.EqualFold(e.Name, "30747-02.htm"):
		if p.ActiveSummon() != nil {
			if summonIsTheFlutes(p, st) {
				htmltext = "30747-04.htm"
			} else {
				htmltext = "30747-03.htm"
			}
		}
	case strings.EqualFold(e.Name, "30747-05.htm"):
		if !summonIsTheFlutes(p, st) {
			htmltext = "30747-06.htm"
		} else {
			st.SetCond(2)
			setInt(st, "iCond", 3)
			s.PlaySound(p, script.SoundMiddle)
			s.GiveItems(p, fairyLeaf, 4)
		}
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
		switch {
		// Wrong level.
		case p.Level() < 45:
			htmltext = "30610-01.htm"
		// More than one flute, or none.
		case dragonfluteCount(p) != 1:
			htmltext = "30610-02.htm"
		default:
			if _, ok := strongFlute(p); ok {
				return "30610-04.htm"
			}
			// Invalid level.
			htmltext = "30610-03.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case cronos:
			htmltext = "30610-07.htm"
		case mimyu:
			id := st.GetInt("iCond")
			switch {
			case id == 1:
				htmltext = "30747-01.htm"
				setInt(st, "iCond", 2)
			case id == 2:
				switch {
				case p.ActiveSummon() == nil:
					htmltext = "30747-02.htm"
				case !summonIsTheFlutes(p, st):
					htmltext = "30747-03.htm"
				default:
					htmltext = "30747-04.htm"
				}
			// The explanation is done, the leaves are already given.
			case id == 3:
				htmltext = "30747-07.htm"
			// At least one tree is done, but not all of them.
			case id > 3 && id < allTrees:
				htmltext = "30747-11.htm"
			// Every tree is done, no more leaves.
			case id == allTrees:
				switch {
				case p.ActiveSummon() == nil:
					return "30747-12.htm"
				case !summonIsTheFlutes(p, st):
					return "30747-14.htm"
				}
				htmltext = "30747-13.htm"
				setInt(st, "iCond", acknowledged)
			// The fairy has spoken.
			case id == acknowledged:
				if summonIsTheFlutes(p, st) {
					return "30747-15.htm"
				}
				if dragonfluteCount(p) > 1 {
					return "30747-17.htm"
				}
				for fluteID := int32(dragonfluteOfWind); fluteID <= dragonfluteOfTwilight; fluteID++ {
					if obj, ok := fluteObject(p, fluteID); ok && obj == st.GetInt("summonOid") {
						enchant := p.ItemEnchantLevel(fluteID)
						s.TakeItems(p, fluteID, 1)
						s.GiveItemsEnchanted(p, fluteID+strongerFluteOffset, 1, enchant)
						s.PlaySound(p, script.SoundFinish)
						st.Exit(true)
						return "30747-16.htm"
					}
				}
				// The registered flute is not the one held: the fairy curses.
				htmltext = "30747-18.htm"
				e.NPC.AddCastDesire(p, flutesCurse(), castWeight)
			}
		}
	}
	return htmltext
}

func onAttacked(s *script.Script, e script.Attacked) {
	p := script.ActingPlayer(e.Attacker)
	if p == nil {
		return
	}
	st := s.CheckPlayerCondition(p, e.NPC, 2)
	if st == nil {
		// The attacker is not on the quest: a hurt tree curses it.
		if e.NPC.HPRatio() < 0.67 && s.Rnd(100) < 30 {
			e.NPC.AddCastDesire(e.Attacker, curseSkill(), castWeight)
		}
		return
	}
	if script.PetControlItemID(e.Attacker) == 0 {
		// The attacker is the questing player, or a servitor: curse it.
		if s.Rnd(100) < 30 {
			e.NPC.AddCastDesire(e.Attacker, curseSkill(), castWeight)
		}
		return
	}

	// The attacker is the pet of a player on the quest.
	npcID := e.NPC.NpcID()
	td, ok := treeOf(npcID)
	if !ok {
		return
	}
	condition := st.GetInt("iCond")
	if td.mask&condition != 0 {
		// The tree gave its leaf already: it says one of its lines.
		e.NPC.Say(script.NpcString(td.messages[s.Rnd(len(td.messages))]))
		return
	}
	if script.PetControlItemID(e.Attacker) != st.GetInt("summonOid") {
		return
	}
	attack := st.GetInt("attack") + 1
	if attack > td.attacks {
		// Enough attacks: the tree may take a leaf.
		if s.Rnd(100) < td.chance && p.HasItem(fairyLeaf) {
			setInt(st, "iCond", condition|td.mask)
			setInt(st, "attack", 0)

			e.NPC.Say(script.NpcString(sayLeafGiven))
			s.TakeItems(p, fairyLeaf, 1)

			// The four leaves are used.
			if st.GetInt("iCond") == allTrees {
				st.SetCond(3)
				s.PlaySound(p, script.SoundMiddle)
			} else {
				s.PlaySound(p, script.SoundItemGet)
			}
		}
		return
	}
	setInt(st, "attack", attack)

	// The tree of the abyss roots the pet now and then.
	if npcID == fairyTreeOfAbyss && s.Rnd(100) < 2 {
		e.NPC.AddCastDesire(e.Attacker, dryadRootOnPets(), castWeight)
	}
}

func onCreated(s *script.Script, e script.Created) {
	if e.NPC.NpcID() == soulOfTreeGuardian {
		guardianLines := [3]int32{sayGuardianOne, sayGuardianTwo, sayGuardianThree}
		e.NPC.Say(script.NpcString(guardianLines[s.Rnd(len(guardianLines))]))
		return
	}
	// The trees only answer what the quest makes them do.
	e.NPC.DisableCoreAI(true)
}

// onMyDying has a dying tree send ghosts after its killer, the first of them
// cursing it.
func onMyDying(s *script.Script, e script.MyDying) {
	for i := range ghostCount {
		ghost := s.Spawner().CreateOnePrivate(e.NPC, soulOfTreeGuardian, ghostLife)
		if ghost == nil {
			continue
		}
		if i == 0 {
			ghost.AddCastDesire(e.Killer, curseSkill(), castWeight)
		}
		ghost.AddAttackDesire(e.Killer, attackWeight)
	}
}
