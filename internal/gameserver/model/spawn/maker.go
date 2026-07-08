package spawn

import (
	"errors"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// Maker is one <npcmaker> group plus its resolved territory references.
type Maker struct {
	Name              string
	Territories       []*Territory
	BannedTerritories []*Territory
	AIType            string
	AIParams          map[string]string
	MaximumNPCs       int
	Event             string
	Entries           []Entry
}

// NewMaker builds a Maker from set plus already-resolved references and
// decoded child entries.
func NewMaker(set *commons.StatSet, territories []*Territory, banned []*Territory, entries []Entry, aiParams map[string]string) (*Maker, error) {
	name, err := set.GetString("name")
	if err != nil {
		return nil, err
	}
	maximum, err := set.GetInt("maximumNpcs")
	if err != nil {
		return nil, err
	}
	if len(territories) == 0 {
		return nil, errors.New("spawn: maker needs at least one territory")
	}

	return &Maker{
		Name:              name,
		Territories:       append([]*Territory(nil), territories...),
		BannedTerritories: append([]*Territory(nil), banned...),
		AIType:            set.GetStringDefault("maker", ""),
		AIParams:          copyStringMap(aiParams),
		MaximumNPCs:       maximum,
		Event:             set.GetStringDefault("event", ""),
		Entries:           append([]Entry(nil), entries...),
	}, nil
}
