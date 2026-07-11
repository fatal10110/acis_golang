// Package attackable tracks combat threat: which creatures a hostile NPC
// currently holds a grudge against, and how strongly.
//
// Two independent tables exist because physical and magical aggression are
// scored differently. ThreatTable accumulates both raw damage and a
// separate hate weight per attacker, and drives melee auto-attack target
// selection. HateTable accumulates only a hate weight (no damage), and
// drives spell-cast target selection. An NPC that both melees and casts
// keeps one of each.
package attackable
