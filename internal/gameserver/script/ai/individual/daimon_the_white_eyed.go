package individual

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// daimonID is Daimon the White-Eyed.
const daimonID = 31705

// daimonDebuffChat and daimonBuffChat are the lines Daimon says as it
// curses or blesses a player.
func daimonDebuffChat() []int32 { return []int32{1000458, 1000459, 1000460} }

func daimonBuffChat() []int32 { return []int32{1000461, 1000462, 1000463} }

// DaimonTheWhiteEyed is the plain script of Daimon the White-Eyed, which
// walks the argos_daemon_roaming route from the moment it is created. A
// player who talks to it is cursed or blessed, at even odds, with the
// fighter or the mystic skill of the player's class type, and sees no
// page. Its first int slot keeps it to one line per 30 s.
func DaimonTheWhiteEyed() script.Script {
	return script.Script{
		Dir: dir,
		Bind: script.Bindings{
			script.EventCreated:   {daimonID},
			script.EventFirstTalk: {daimonID},
		},
		Hooks: script.Hooks{
			OnTimer: func(s *script.Script, e script.Timer) string {
				if strings.EqualFold(e.Name, "6543") {
					e.NPC.SetInt(script.SlotIntAI0, 0)
				}
				var base script.Hooks
				return base.Timer(s, e)
			},
			OnCreated: func(_ *script.Script, e script.Created) {
				e.NPC.AddMoveRouteDesire("argos_daemon_roaming", 2000)

				e.NPC.SetInt(script.SlotIntAI0, 0)
			},
			OnFirstTalk: func(s *script.Script, e script.FirstTalk) string {
				n, p := e.NPC, e.Player
				fighter := script.ClassTypeOf(p.ClassID()) == player.ClassTypeFighter
				if s.RndBool() {
					if n.Int(script.SlotIntAI0) == 0 {
						chat := daimonDebuffChat()
						n.Say(script.NpcString(chat[s.Rnd(len(chat))]))
						n.SetInt(script.SlotIntAI0, 1)

						s.StartTimer("6543", n, nil, 30000*time.Millisecond)
					}

					if fighter {
						n.AddCastDesire(p, skill.Ref{ID: 1206, Level: 19}, 1000000)
					} else {
						n.AddCastDesire(p, skill.Ref{ID: 1083, Level: 17}, 1000000)
					}
				} else {
					if n.Int(script.SlotIntAI0) == 0 {
						chat := daimonBuffChat()
						n.Say(script.NpcString(chat[s.Rnd(len(chat))]))
						n.SetInt(script.SlotIntAI0, 1)

						s.StartTimer("6543", n, nil, 30000*time.Millisecond)
					}

					if fighter {
						n.AddCastDesire(p, skill.Ref{ID: 1086, Level: 2}, 1000000)
					} else {
						n.AddCastDesire(p, skill.Ref{ID: 1059, Level: 3}, 1000000)
					}
				}
				return ""
			},
		},
	}
}
