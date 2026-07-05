package records

import "github.com/fatal10110/acis_golang/internal/commons"

// NewbieItem is one piece of starter equipment granted to a freshly created
// character of a base profession (NewbieItem.java).
type NewbieItem struct {
	ID         int
	Count      int
	IsEquipped bool
}

// NewNewbieItem builds a NewbieItem from set. id and count are required;
// isEquipped defaults to true when absent.
func NewNewbieItem(set *commons.StatSet) (NewbieItem, error) {
	id, err := set.GetInt("id")
	if err != nil {
		return NewbieItem{}, err
	}
	count, err := set.GetInt("count")
	if err != nil {
		return NewbieItem{}, err
	}
	return NewbieItem{ID: id, Count: count, IsEquipped: set.GetBoolDefault("isEquipped", true)}, nil
}
