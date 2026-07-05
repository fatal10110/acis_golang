// Package skillnode contains skill tree node datatypes.
package skillnode

import "github.com/fatal10110/acis_golang/internal/commons"

// GeneralSkillNode is one skill/level combination a player template can
// learn (GeneralSkillNode.java). The Java class extends SkillNode, which
// extends IntIntHolder; neither is ported yet, so their fields are
// flattened here until another unit needs those types on their own
// (SkillNode.java, IntIntHolder.java).
type GeneralSkillNode struct {
	// ID is the skill id and Value the skill level ("lvl" in the data),
	// following IntIntHolder's id/value naming.
	ID    int
	Value int
	// MinLvl is the character level required to learn this node.
	MinLvl int
	// Cost is the SP cost. Divine Inspiration deliberately uses -1 (0 would
	// read as an autoGet skill and be freely given).
	Cost int
}

// NewGeneralSkillNode builds a GeneralSkillNode from set; id, lvl, minLvl
// and cost are all required.
func NewGeneralSkillNode(set *commons.StatSet) (GeneralSkillNode, error) {
	id, err := set.GetInt("id")
	if err != nil {
		return GeneralSkillNode{}, err
	}
	value, err := set.GetInt("lvl")
	if err != nil {
		return GeneralSkillNode{}, err
	}
	minLvl, err := set.GetInt("minLvl")
	if err != nil {
		return GeneralSkillNode{}, err
	}
	cost, err := set.GetInt("cost")
	if err != nil {
		return GeneralSkillNode{}, err
	}
	return GeneralSkillNode{ID: id, Value: value, MinLvl: minLvl, Cost: cost}, nil
}

// CorrectedCost returns 0 or the initial cost if superior to 0. Only used
// to display the correct value to the client; regular uses read Cost.
func (n GeneralSkillNode) CorrectedCost() int {
	return max(0, n.Cost)
}
