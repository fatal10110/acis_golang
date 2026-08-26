package item

import (
	"strings"
)

// Condition is one node of an item's use-condition expression tree: either
// a combinator ("and", "or", "not") wrapping child nodes, or a leaf
// predicate (e.g. one player-state check) carrying its own attributes. Kind
// is the node's element name, lowercased. Evaluating a condition against a
// creature is combat-engine behavior this package doesn't own; a Condition
// only preserves the parsed shape so that engine can consume it once built.
type Condition struct {
	Kind     string
	Attrs    map[string]string
	Children []Condition
}

// UseCondition is one <cond>-equivalent clause attached to an item
// template: the root predicate expression, and the message shown to a
// player who fails it. Message and MessageID are mutually exclusive: at
// most one is ever set, matching how the data format only ever fills in
// one. AddName reports whether the item's own name should be appended to
// the MessageID'd message.
type UseCondition struct {
	Root      Condition
	Message   string
	MessageID int32
	AddName   bool
}

// EvaluateCondition walks cond's and/or/not combinator tree, deferring to
// leaf for every non-combinator node. Shared by the player and pet
// use-condition evaluators (internal/gameserver/network/item_use_gate.go,
// internal/gameserver/petitem/conditions.go), which differ only in which
// actor a leaf checks against and which leaf kinds/attrs they support.
func EvaluateCondition(cond Condition, leaf func(Condition) bool) bool {
	switch strings.ToLower(cond.Kind) {
	case "and":
		for _, child := range cond.Children {
			if !EvaluateCondition(child, leaf) {
				return false
			}
		}
		return true
	case "or":
		for _, child := range cond.Children {
			if EvaluateCondition(child, leaf) {
				return true
			}
		}
		return false
	case "not":
		return len(cond.Children) == 1 && !EvaluateCondition(cond.Children[0], leaf)
	default:
		return leaf(cond)
	}
}
