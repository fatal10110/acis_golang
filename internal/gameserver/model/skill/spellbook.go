package skill

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
)

const DivineInspirationSkillID ID = 1405

type Spellbook struct {
	SkillID ID
	ItemID  int32
}

func NewSpellbook(set *commons.StatSet) (Spellbook, error) {
	skillID, err := set.GetInt32("skillId")
	if err != nil {
		return Spellbook{}, fmt.Errorf("skill: spellbook: %w", err)
	}
	itemID, err := set.GetInt32("itemId")
	if err != nil {
		return Spellbook{}, fmt.Errorf("skill: spellbook %d: %w", skillID, err)
	}
	return Spellbook{SkillID: ID(skillID), ItemID: itemID}, nil
}

type SpellbookTable struct {
	books map[ID]int32
}

func NewSpellbookTable(books []Spellbook) (*SpellbookTable, error) {
	bookMap := make(map[ID]int32, len(books))
	for _, book := range books {
		if _, exists := bookMap[book.SkillID]; exists {
			return nil, fmt.Errorf("skill: duplicate spellbook for skill %d", book.SkillID)
		}
		bookMap[book.SkillID] = book.ItemID
	}
	return &SpellbookTable{books: bookMap}, nil
}

func (t *SpellbookTable) BookForSkill(skillID ID, level int, spellbooksRequired, divineBooksRequired bool) int32 {
	if skillID == DivineInspirationSkillID {
		if !divineBooksRequired {
			return 0
		}
		switch level {
		case 1:
			return 8618
		case 2:
			return 8619
		case 3:
			return 8620
		case 4:
			return 8621
		default:
			return 0
		}
	}
	if level != 1 || !spellbooksRequired {
		return 0
	}
	return t.books[skillID]
}

func (t *SpellbookTable) Count() int { return len(t.books) }
