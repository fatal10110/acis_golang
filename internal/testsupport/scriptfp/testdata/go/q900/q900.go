// Package q900 is a check fixture: a faithful port of the fixture class
// quest.Q900_Fixture. It is parsed, never built.
package q900

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const (
	keeper = 30_048
	mask   = 0x1F
	delay  = 3000
	chance = 0.5
	sep    = ';'
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
				reward(e.Player)
			}
			x := e.Player.Position().X()
			for _, n := range e.Player.Summons() {
				n.ID()
			}
			for _, g := range guards {
				g.ObjectID()
			}
			script.NewLocation(x, 2, 3)
			return strings.TrimSpace("30048-01.htm")
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			base.Attacked(s, e)
		},
		OnCreated: func(s *script.Script, e script.Created) {
			base.Created(s, script.Created{NPC: guards[mask]})
		},
		OnDecayed: onDecayed,
	})}
}

func reward(p *script.Player) {
	script.GiveItems(p, 57, 100)
	script.PlaySound(p, script.SoundMiddle)
}

func onDecayed(s *script.Script, e script.Decayed) {
	decay(s, e)
}

func decay(s *script.Script, e script.Decayed) {
	script.Hooks{}.Decayed(s, e)
}
