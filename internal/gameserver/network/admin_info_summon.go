package network

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
)

// summonInfoPage is s's summon page: its name, level and experience, its
// owner, class and current intention, its vitals and karma, and for a pet
// its inventory link, food and load.
func (l *GameClientLink) summonInfoPage(s *summon.Actor) string {
	name := "N/A"
	if s.IsNamed() {
		name = s.Name()
	}
	owner, hasOwner := s.Owner()
	ownerLink := "N/A"
	if hasOwner {
		ownerName := owner.CharacterName()
		ownerLink = ` <a action="bypass -h admin_debug ` + ownerName + `">` + ownerName + `</a>`
	}
	undead := "no"
	if s.Undead() {
		undead = "yes"
	}
	inv, food, load := "none", "N/A", "N/A"
	if s.IsPet() {
		inv = "N/A"
		if hasOwner {
			inv = ` <a action="bypass admin_summon inventory">view</a>`
		}
		food = strconv.Itoa(s.Fed()) + "/" + strconv.Itoa(l.petMaxMeal(s))
		weight := 0
		if items := s.PetInventory(); items != nil {
			weight = items.TotalWeight()
		}
		load = strconv.Itoa(weight) + "/" + strconv.Itoa(s.WeightLimit())
	}
	return fillPage(l.adminHTML("petinfo.htm"),
		"%name%", name,
		"%level%", strconv.Itoa(s.Level()),
		"%exp%", strconv.FormatInt(s.Exp(), 10),
		"%owner%", ownerLink,
		"%class%", summonClassName(s),
		"%ai%", summonIntentionName(s),
		"%hp%", strconv.Itoa(int(s.HP()))+"/"+strconv.Itoa(int(s.MaxHPValue())),
		"%mp%", strconv.Itoa(int(s.MPValue()))+"/"+strconv.Itoa(int(s.MaxMPValue())),
		"%karma%", strconv.Itoa(s.Karma()),
		"%undead%", undead,
		"%inv%", inv,
		"%food%", food,
		"%load%", load,
	)
}

// petMaxMeal is the most food pet s holds at its current level, 0 when its
// template has no growth row for that level.
func (l *GameClientLink) petMaxMeal(s *summon.Actor) int {
	if l.npcs == nil {
		return 0
	}
	tmpl, ok := l.npcs.Get(s.NPCID())
	if !ok || tmpl.Pet == nil {
		return 0
	}
	return tmpl.Pet.Levels[s.Level()].MaxMeal
}

// summonClassName names the reference class s is an instance of.
func summonClassName(s *summon.Actor) string {
	switch {
	case s.IsBabyPet():
		return "BabyPet"
	case s.IsPet():
		return "Pet"
	case s.SiegeSummon():
		return "SiegeSummon"
	default:
		return "Servitor"
	}
}

// summonIntentionName is the reference name of the intention s's AI is
// running, IDLE when it has no AI.
func summonIntentionName(s *summon.Actor) string {
	brain, ok := s.AI().(*ai.Summon)
	if !ok {
		return intentionEnumName(ai.IntentionIdle)
	}
	return intentionEnumName(brain.CurrentIntention())
}

// intentionEnumName is i's constant name in the reference IntentionType
// enum.
func intentionEnumName(i ai.Intention) string {
	return strings.ToUpper(i.String())
}
