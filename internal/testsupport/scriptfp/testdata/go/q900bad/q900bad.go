// Package q900bad is a check fixture: a port of quest.Q900_Fixture with
// one difference of every kind Check reports. It is parsed, never built.
package q900bad

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

const (
	keeper = 30048
	mask   = 31
	delay  = 3_000
	chance = 0.5
)

var guards map[int32]*script.NPC

func New() script.Script {
	base := script.Hooks{}
	return script.Script{Name: "Fixture \"quoted\"\n", QuestID: 900, Hooks: base.With(script.Hooks{
		OnEvent: func(s *script.Script, e script.Event) string {
			return base.Event(s, e)
		},
		OnTalk: func(s *script.Script, e script.Talk) string {
			if e.NPC.ID() == keeper {
				script.GiveItems(e.Player, 58, 100)
				script.TakeItems(e.Player, 58, 100)
			}
			x := e.Player.Position().X()
			for _, n := range e.Player.Summons() {
				n.ID()
			}
			for _, g := range guards {
				g.ObjectID()
			}
			script.NewLocation(x, 2, 3)
			return "30048-01.htm" + "extra"
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			base.Attacked(s, script.Attacked{NPC: e.NPC})
		},
		OnCreated: func(s *script.Script, e script.Created) {
			base.Created(s, script.Created{NPC: guards[mask]})
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			s.SeeCreature(e)
		},
	})}
}
