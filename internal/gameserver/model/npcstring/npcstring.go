// Package npcstring ports aCis's NpcStringId.java: a numeric client string id
// to hardcoded text lookup used by broadcastNpcSay(NpcStringId).
package npcstring

// Text returns id's client-visible text and true, or "" and false if id has
// no entry (Java's NpcStringId.getMessage() has no such gap: every id a
// walkerRoutes.xml fstring can carry resolves to a static field).
func Text(id int32) (string, bool) {
	msg, ok := table[id]
	return msg, ok
}
