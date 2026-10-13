package script

// The npcmaker operations a behavior or a quest makes (#3748): the maker
// that placed an NPC, and the maker script events it sends.

// MakerName returns the name of the npcmaker that placed the NPC. An NPC
// no npcmaker placed (a script's spawn or a private), and a handle on
// nothing, panic.
func (n *NPC) MakerName() string {
	m := n.instance().Maker
	if m == nil {
		panic("script: an npc no npcmaker placed has no maker")
	}
	return m.Name
}

// SendMakerEvent runs, before it returns, the maker script event name with
// int1 and int2 on the npcmaker named maker (case-insensitive, the first in
// list order), and reports whether one has that name; nothing runs when
// none has. The maker's hook runs on the caller's goroutine.
func (s *Spawner) SendMakerEvent(maker, name string, int1, int2 int) bool {
	return s.population().MakerEvent(maker, name, int1, int2)
}
