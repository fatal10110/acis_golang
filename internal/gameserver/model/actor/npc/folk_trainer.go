package npc

import (
	"slices"
	"strconv"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// ThirdClassRequiredPage is the page a trainer opens instead of its enchant
// skill list for a talker short of a third profession.
const ThirdClassRequiredPage = "<html><body> You must have 3rd class change quest completed.</body></html>"

// CanTeach reports whether this NPC trains profession classID: its
// template's teach list names the profession, or, for a third-tier
// profession, the second-tier one it upgrades from.
func (f *Folk) CanTeach(classID int) bool {
	teach := f.Instance.Template.TeachTo
	if teach == nil {
		return false
	}
	if level, ok := player.ClassLevel(classID); ok && level == 3 {
		classID, _ = player.ClassParent(classID)
	}
	return slices.Contains(teach, classID)
}

// NoSkillsPage is the page this NPC opens instead of a skill list for a
// talker whose profession it does not train: trainer/<npcId>-noskills.htm
// as is, or a "My html is missing" notice naming it.
func (f *Folk) NoSkillsPage(pages Pages) string {
	path := "data/html/trainer/" + strconv.Itoa(f.NpcID()) + "-noskills.htm"
	if page, ok := pages.Get(path); ok {
		return page
	}
	return "<html><body>My html is missing:<br>" + path + "</body></html>"
}
