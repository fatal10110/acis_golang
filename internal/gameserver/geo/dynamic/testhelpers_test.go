package dynamic

import "github.com/fatal10110/acis_golang/internal/commons"

type mapSet struct {
	StatSet *commons.StatSet
}

func newMapSet() *mapSet {
	return &mapSet{StatSet: commons.NewStatSet()}
}

func (m *mapSet) Set(name string, value any) {
	m.StatSet.Set(name, value)
}
