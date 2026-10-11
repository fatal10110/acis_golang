// Package q350 is the quest Enhance Your Weapon: the masters hand out a
// soul crystal, and a crystal charged on the monsters that feed it moves
// up a stage, or breaks, when such a monster dies.
package q350

import (
	"slices"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q350_EnhanceYourWeapon"

// The crystals the masters hand out.
const (
	redCrystal   = 4629
	greenCrystal = 4640
	blueCrystal  = 4651
)

// The system messages of a crystal charge.
const (
	msgAbsorbSucceeded        = 974
	msgAbsorbFailed           = 975
	msgCrystalBroke           = 976
	msgAbsorbFailedResonation = 977
	msgAbsorbRefused          = 978
)

// The ways a monster's death picks the players whose crystals it tries.
const (
	absorbFullParty      = "FULL_PARTY"
	absorbPartyOneRandom = "PARTY_ONE_RANDOM"
	absorbLastHit        = "LAST_HIT"
)

const (
	// minLevel is the level the quest starts at.
	minLevel = 40
	// maxLevelGap is how many levels above the monster a player may stand
	// for its crystal to charge.
	maxLevelGap = 8
	// stageRoll is the range of the one roll a death tries every crystal
	// with.
	stageRoll = 1000
)

// New returns the quest over the soul crystals and the monsters that charge
// them.
func New(crystals *item.SoulCrystalTable) script.Script {
	q := quest{crystals: crystals}
	masters := []int32{30115, 30194, 30856}
	return script.Script{
		Title:   "Enhance Your Weapon",
		QuestID: 350,
		Bind: script.Bindings{
			script.EventQuestStart: masters,
			script.EventTalked:     masters,
			script.EventMyDying:    crystals.LevelingNPCIDs(),
		},
		UsedItems: crystals.CrystalIDs(),
		Hooks: script.Hooks{
			OnEvent:   q.onAdvEvent,
			OnTalk:    q.onTalk,
			OnItemUse: q.onItemUse,
			OnMyDying: q.onMyDying,
		},
	}
}

// quest holds what the hooks read: the immutable soul crystal table.
type quest struct{ crystals *item.SoulCrystalTable }

func (q quest) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch {
	case strings.HasSuffix(e.Name, "-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(e.Player, script.SoundAccept)
	case strings.HasSuffix(e.Name, "-09.htm"):
		s.PlaySound(e.Player, script.SoundMiddle)
		s.GiveItems(e.Player, redCrystal, 1)
	case strings.HasSuffix(e.Name, "-10.htm"):
		s.PlaySound(e.Player, script.SoundMiddle)
		s.GiveItems(e.Player, greenCrystal, 1)
	case strings.HasSuffix(e.Name, "-11.htm"):
		s.PlaySound(e.Player, script.SoundMiddle)
		s.GiveItems(e.Player, blueCrystal, 1)
	case strings.HasSuffix(e.Name, "-exit.htm"):
		st.Exit(true)
	}
	return htmltext
}

func (q quest) onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	npc := strconv.Itoa(int(e.NPC.NpcID()))
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < minLevel {
			htmltext = npc + "-lvl.htm"
		} else {
			htmltext = npc + "-01.htm"
		}
	case questlog.StatusStarted:
		for _, held := range e.Player.HeldItems() {
			if _, ok := q.crystals.Crystal(held.ItemID); ok {
				return npc + "-03.htm"
			}
		}
		htmltext = npc + "-21.htm"
	}
	return htmltext
}

// onItemUse records the player, alive, as charging the used crystal on its
// living target monster, when that monster charges crystals.
func (q quest) onItemUse(_ *script.Script, e script.ItemUse) {
	if e.Player.Dead() {
		return
	}
	monster, ok := e.Target.(*script.NPC)
	if !ok || !monster.Monster() || monster.Dead() {
		return
	}
	if _, ok := q.crystals.LevelingInfo(monster.NpcID()); !ok {
		return
	}
	monster.AddAbsorber(e.Player, e.ObjectID)
}

// onMyDying tries the crystals of the players the monster's leveling info
// names, all with one roll.
func (q quest) onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if p == nil {
		return
	}
	info, ok := q.crystals.LevelingInfo(e.NPC.NpcID())
	if !ok {
		return
	}
	chance := s.Rnd(stageRoll)
	switch info.AbsorbType {
	case absorbFullParty:
		for _, st := range s.PartyMembersState(p, e.NPC, questlog.StatusStarted) {
			q.tryToStageCrystal(s, st.Player(), e.NPC, info, chance)
		}
	case absorbPartyOneRandom:
		if st := s.RandomPartyMemberState(p, e.NPC, questlog.StatusStarted); st != nil {
			q.tryToStageCrystal(s, st.Player(), e.NPC, info, chance)
		}
	case absorbLastHit:
		if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) != nil {
			q.tryToStageCrystal(s, p, e.NPC, info, chance)
		}
	}
}

// tryToStageCrystal stages, breaks or keeps the one soul crystal p holds,
// by chance: below the stage chance it stages, below the stage and break
// chances together it breaks, otherwise the charge fails. A player with no
// crystal is told nothing; one with several is told they resonate, unless
// the monster needs the crystal skill and p did not register with it.
func (q quest) tryToStageCrystal(s *script.Script, p *script.Player, monster *script.NPC, info item.SoulCrystalLevelingInfo, chance int) {
	var crystal item.SoulCrystal
	var crystalObjectID int32
	found := false
	for _, held := range p.HeldItems() {
		data, ok := q.crystals.Crystal(held.ItemID)
		if !ok {
			continue
		}
		if found {
			if !info.SkillRequired {
				p.SystemMessage(msgAbsorbFailedResonation)
			} else if ai, ok := monster.Absorber(p); ok && ai.Registered {
				p.SystemMessage(msgAbsorbFailedResonation)
			}
			return
		}
		crystal, crystalObjectID, found = data, held.ObjectID, true
	}
	if !found {
		return
	}
	if info.SkillRequired {
		ai, ok := monster.Absorber(p)
		if !ok || !ai.Registered {
			return
		}
		if !ai.Valid(crystalObjectID) {
			p.SystemMessage(msgAbsorbRefused)
			return
		}
	}
	if !slices.Contains(info.Levels, crystal.Level) {
		p.SystemMessage(msgAbsorbRefused)
		return
	}
	if p.Level()-monster.Level() > maxLevelGap {
		p.SystemMessage(msgAbsorbRefused)
		return
	}
	switch {
	case chance < info.ChanceStage:
		exchangeCrystal(s, p, crystal, true)
	case chance < info.ChanceStage+info.ChanceBreak:
		exchangeCrystal(s, p, crystal, false)
	default:
		p.SystemMessage(msgAbsorbFailed)
	}
}

// exchangeCrystal takes p's crystal and gives the next stage, or the
// broken crystal when it broke and the crystal has one.
func exchangeCrystal(s *script.Script, p *script.Player, crystal item.SoulCrystal, stage bool) {
	s.TakeItems(p, crystal.InitialItemID, 1)
	if stage {
		p.SystemMessage(msgAbsorbSucceeded)
		s.GiveItems(p, crystal.StagedItemID, 1)
		s.PlaySound(p, script.SoundItemGet)
		return
	}
	if crystal.BrokenItemID != 0 {
		p.SystemMessage(msgCrystalBroke)
		s.GiveItems(p, crystal.BrokenItemID, 1)
	}
}
