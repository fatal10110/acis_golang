// Package cabalbuffer is the Seven Signs town priests: a Preacher of Doom
// curses the players of the cabal losing the Seven Signs, an Orator of
// Revelations blesses those of the winning one, and both preach once a
// minute.
package cabalbuffer

import (
	"slices"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// preachTimer is the timer each priest preaches on.
const preachTimer = "5097"

// preacherOfDoomRandomChat is what a Preacher of Doom preaches.
func preacherOfDoomRandomChat() []int32 { return []int32{1000303, 1000415, 1000416, 1000417} }

// preacherOfDoomCastChat is what a Preacher of Doom says as it curses a
// player: a fighter first, a fighter again, a mystic first, a mystic again.
func preacherOfDoomCastChat() []int32 { return []int32{1000420, 1000304, 1000418, 1000419} }

// preacherOfDoomIDs are the Preachers of Doom.
func preacherOfDoomIDs() []int32 {
	return []int32{
		31093, 31172, 31174, 31176, 31178, 31180, 31182, 31184, 31186, 31188, 31190, 31192, 31194, 31196, 31198,
		31200, 31231, 31232, 31233, 31234, 31235, 31236, 31237, 31238, 31239, 31240, 31241, 31242, 31243, 31244,
		31245, 31246, 31713, 31714, 31715, 31716, 31717, 31718, 31719, 31720, 31999, 32000, 32001, 32002,
	}
}

// oratorOfRevelationsRandomChat is what an Orator of Revelations preaches.
func oratorOfRevelationsRandomChat() []int32 { return []int32{1000305, 1000421, 1000422, 1000423} }

// oratorOfRevelationsCastChat is what an Orator of Revelations says as it
// blesses a player, in the order of preacherOfDoomCastChat.
func oratorOfRevelationsCastChat() []int32 { return []int32{1000306, 1000424, 1000426, 1000425} }

// oratorOfRevelationsIDs are the Orators of Revelations.
func oratorOfRevelationsIDs() []int32 {
	return []int32{
		31094, 31173, 31175, 31177, 31179, 31181, 31183, 31185, 31187, 31189, 31191, 31193, 31195, 31197, 31199,
		31201, 31247, 31248, 31249, 31250, 31251, 31252, 31253, 31254, 31721, 31722, 31723, 31724, 31725, 31726,
		31727, 31728, 32003, 32004, 32005, 32006,
	}
}

// New returns the feature.
func New() script.Script {
	priests := append(preacherOfDoomIDs(), oratorOfRevelationsIDs()...)
	return script.Script{
		Dir: "feature",
		Bind: script.Bindings{
			script.EventCreated:     priests,
			script.EventDecayed:     priests,
			script.EventSeeCreature: priests,
		},
		Hooks: script.Hooks{OnTimer: onTimer, OnCreated: onCreated, OnDecayed: onDecayed, OnSeeCreature: onSeeCreature},
	}
}

// onTimer preaches one of the priest's lines, at random.
func onTimer(s *script.Script, e script.Timer) string {
	if strings.EqualFold(e.Name, preachTimer) {
		chat := oratorOfRevelationsRandomChat()
		if isPreacher(e.NPC) {
			chat = preacherOfDoomRandomChat()
		}
		e.NPC.Say(script.NpcString(chat[s.Rnd(len(chat))]))
	}
	var base script.Hooks
	return base.Timer(s, e)
}

// onCreated makes the priest preach every minute, the first time a minute
// after it appears.
func onCreated(s *script.Script, e script.Created) {
	s.StartTimerAtFixedRate(preachTimer, e.NPC, nil, 60000*time.Millisecond, 60000*time.Millisecond)

	var base script.Hooks
	base.Created(s, e)
}

// onDecayed stops the priest's preaching.
func onDecayed(s *script.Script, e script.Decayed) {
	s.CancelTimers(script.TimerName(preachTimer), script.TimerNPC(e.NPC))

	var base script.Hooks
	base.Decayed(s, e)
}

// onSeeCreature curses or blesses a player the priest sees. A Preacher of
// Doom curses a player of the losing cabal, an Orator of Revelations
// blesses one of the winning cabal, with the fighter or the mystic version
// of its skill: at level 1 when the player holds no effect of it, else, one
// time in twenty, at level 2. A rare line names the player. An invisible
// game master and a player of no cabal are left alone.
func onSeeCreature(s *script.Script, e script.SeeCreature) {
	var base script.Hooks
	if _, ok := e.Creature.(*script.Player); ok {
		player := script.ActingPlayer(e.Creature)

		// Don't bother about invisible GMs.
		if player.IsGM() && !player.Visible() {
			base.SeeCreature(s, e)
			return
		}

		// Don't bother about Players who didn't participated.
		playerCabal := s.PlayerCabal(player.ObjectID())
		if playerCabal == sevensigns.NoCabal {
			base.SeeCreature(s, e)
			return
		}

		i0 := s.Rnd(100)
		i1 := s.Rnd(10000)

		n := e.NPC
		if isPreacher(n) {
			if playerCabal == s.LosingCabal() {
				skillID := int32(4362)
				if !player.IsMageClass() {
					skillID = 4361
				}
				chat := preacherOfDoomCastChat()
				if !player.HasEffect(skillID) {
					if i1 < 1 {
						if !player.IsMageClass() {
							n.Say(script.NpcString(chat[0], player.Name()))
						} else {
							n.Say(script.NpcString(chat[2], player.Name()))
						}
					}
					n.AddCastDesire(player, skill.Ref{ID: skill.ID(skillID), Level: 1}, 1000000)
				} else if i0 < 5 {
					if i1 < 500 {
						if !player.IsMageClass() {
							n.Say(script.NpcString(chat[1], player.Name()))
						} else {
							n.Say(script.NpcString(chat[3], player.Name()))
						}
					}
					n.AddCastDesire(player, skill.Ref{ID: skill.ID(skillID), Level: 2}, 1000000)
				}
			}
		} else {
			if playerCabal == s.WinningCabal() {
				skillID := int32(4365)
				if !player.IsMageClass() {
					skillID = 4364
				}
				chat := oratorOfRevelationsCastChat()
				if !player.HasEffect(skillID) {
					if i1 < 1 {
						if !player.IsMageClass() {
							n.Say(script.NpcString(chat[0], player.Name()))
						} else {
							n.Say(script.NpcString(chat[2], player.Name()))
						}
					}
					n.AddCastDesire(player, skill.Ref{ID: skill.ID(skillID), Level: 1}, 1000000)
				} else if i0 < 5 {
					if i1 < 500 {
						if !player.IsMageClass() {
							n.Say(script.NpcString(chat[1], player.Name()))
						} else {
							n.Say(script.NpcString(chat[3], player.Name()))
						}
					}
					n.AddCastDesire(player, skill.Ref{ID: skill.ID(skillID), Level: 2}, 1000000)
				}
			}
		}
	}
	base.SeeCreature(s, e)
}

// isPreacher reports whether n is a Preacher of Doom; any other priest is
// an Orator of Revelations.
func isPreacher(n *script.NPC) bool {
	return slices.Contains(preacherOfDoomIDs(), n.NpcID())
}
