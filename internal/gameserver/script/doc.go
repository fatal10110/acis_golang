// Package script runs scripted content: quests, NPC behaviors, feature NPCs,
// teleporters and scheduled tasks.
//
// A script is a Script value built by a Go constructor. scripts.xml decides
// which scripts load, in what order and on what schedule; literal catalogs
// map each listed path to its constructor. Build registers the listed
// scripts once, at boot, into an immutable Registry that the engine reads
// lock-free from any goroutine. Hooks run synchronously where the engine
// raises them, and a panicking hook is recovered per invocation.
package script
