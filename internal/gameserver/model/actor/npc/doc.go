// Package npc models non-player actors.
//
// # Deleting an NPC off its queue
//
// A synchronous delete (Hostile.DeleteNow, Folk.DeleteNow, and the Decay
// that a maker deleting its NPCs or a master leaving with its minions runs)
// decays the NPC on the calling goroutine: a spawner, the engine queue, a
// player's queue or another NPC's queue. The NPC has left the world when the
// call returns. The delete does not wait for a task the NPC's own queue may
// be running at that moment. Hooks raised on one NPC's queue delete other
// NPCs, so two NPC queues that each waited for the other would deadlock, and
// a task cannot tell whether the queue it would wait for is its own.
//
// An NPC's queue owns none of its state: every field sits behind its own
// lock, so a delete racing a queue task is not a data race. What keeps a
// queue task from acting for an NPC that is gone is this contract:
//
//   - The decay marks the NPC dead and decayed first, under its death lock,
//     before any hook runs or the world removal starts.
//   - Its queue is closed before the delete returns. No armed timer (a hit,
//     a movement arrival) runs after that, the script timers bound to the
//     NPC are cancelled, and later posts are refused.
//   - The one exception is a cast in flight: the delete does not stop it,
//     and its steps run to their end on the closed queue. A cast not
//     launched yet stops at its launch, its target lost to an NPC that
//     knows no one any more. A launched one lands at its hit as a dead
//     caster's skill: no damage, while its effects and the hit's notice to
//     its targets still apply. Then it finishes and raises its
//     skill-finished hooks.
//   - Every task the queue has already taken checks, before it acts, that
//     the NPC is alive, on the grid, or both: an AI tick or think, a hit, a
//     regeneration step, a region reset. One that runs after the delete
//     returns finds the NPC dead, decayed and off the grid, and does nothing.
//   - A task already past that check when the delete starts finishes, as a
//     task past its check does when a kill lands from another queue. Once
//     the NPC is off the grid nothing it emits reaches a client, since no
//     player knows it any more, and a move of it only updates its position.
//
// A new task an NPC's queue runs keeps to the same contract: it checks the
// NPC before acting for it.
package npc
