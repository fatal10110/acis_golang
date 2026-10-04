package npc

// DerbyTrackManager reports whether f is a monster race track manager,
// which sells race tickets and shows the race to the players who know it.
func (f *Folk) DerbyTrackManager() bool {
	return hostileKind(f.Instance) == "DerbyTrackManagerNpc"
}
