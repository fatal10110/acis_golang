package spawn

import (
	"errors"
	"fmt"

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
	idf := commons.NewFields(set, "spawn maker")
	name := idf.String("name")
	if err := idf.Err(); err != nil {
		return nil, err
	}
	f := commons.NewFields(set, fmt.Sprintf("spawn maker %q", name))
	maximum := f.Int("maximumNpcs")
	if err := f.Err(); err != nil {
		return nil, err
	}
	if len(territories) == 0 {
		return nil, errors.New("spawn: maker needs at least one territory")
	}

	return &Maker{
		Name:              name,
		Territories:       append([]*Territory(nil), territories...),
		BannedTerritories: append([]*Territory(nil), banned...),
		AIType:            f.StringDefault("maker", ""),
		AIParams:          copyStringMap(aiParams),
		MaximumNPCs:       maximum,
		Event:             f.StringDefault("event", ""),
		Entries:           append([]Entry(nil), entries...),
	}, nil
}
