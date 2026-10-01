// Package npcstring is the numeric client NpcString id to hardcoded text
// lookup that NPC say broadcasts use.
package npcstring

// Text returns id's client-visible text and true, or "" and false if id has
// no entry (the complete id set has no such gap: every id a walkerRoutes.xml
// fstring can carry has text).
func Text(id int32) (string, bool) {
	msg, ok := table[id]
	return msg, ok
}
