package manager

// SetNoSpawns switches the spawn pass off (server.properties NoSpawns):
// SpawnOnStart, RespawnAll and StartSevenSigns then build the npcmakers
// without starting any. Spawns asked for afterwards still happen: a spawn
// event or maker started by a script or an admin, the Seven Signs groups at
// a period change, and every standalone spawn. Call it before SpawnOnStart.
func (n *Npcs) SetNoSpawns(on bool) {
	n.noSpawns.Store(on)
}
