package item

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
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

// NewUseCondition builds a UseCondition from root, the already-parsed
// predicate tree, and set, the folded attributes of the clause's own
// element ("msg", "msgId", "addName", all optional).
func NewUseCondition(root Condition, set *commons.StatSet) (UseCondition, error) {
	uc := UseCondition{Root: root}

	switch {
	case set.Has("msg"):
		msg, err := set.GetString("msg")
		if err != nil {
			return UseCondition{}, fmt.Errorf("item: use condition: %w", err)
		}
		uc.Message = msg
	case set.Has("msgId"):
		id, err := set.GetInt32("msgId")
		if err != nil {
			return UseCondition{}, fmt.Errorf("item: use condition: %w", err)
		}
		uc.MessageID = id
		if set.Has("addName") && id > 0 {
			uc.AddName = true
		}
	}

	return uc, nil
}
