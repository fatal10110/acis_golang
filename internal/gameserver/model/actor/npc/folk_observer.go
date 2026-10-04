package npc

// SetObserverGroups makes the NPC a broadcasting tower offering the
// viewpoint groups groups, in order: talking to it lists them instead of
// opening its chat window.
func (f *Folk) SetObserverGroups(groups []int) {
	g := append([]int(nil), groups...)
	f.observerGroups.Store(&g)
}

// ObserverGroups are the viewpoint groups the NPC offers, nil for an NPC
// that is no broadcasting tower.
func (f *Folk) ObserverGroups() []int {
	if g := f.observerGroups.Load(); g != nil {
		return append([]int(nil), (*g)...)
	}
	return nil
}
