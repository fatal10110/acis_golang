// Package fixture holds the script shapes the handle-operation scan is
// tested on (internal/testsupport/handleops): each constructor builds one
// script whose hooks reach kind-specific handle operations in one way.
package fixture

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// Direct calls the operation in its hook.
func Direct() script.Script {
	return script.Script{Hooks: script.Hooks{OnAttacked: func(_ *script.Script, e script.Attacked) {
		_ = e.NPC.HP()
	}}}
}

// LocalVar calls it through a local variable of function type.
func LocalVar() script.Script {
	return script.Script{Hooks: script.Hooks{OnAttacked: func(_ *script.Script, e script.Attacked) {
		f := func(n *script.NPC) { _ = n.Rooted() }
		f(e.NPC)
	}}}
}

// Param calls it through a function-typed parameter.
func Param() script.Script {
	return script.Script{Hooks: script.Hooks{OnAttacked: func(_ *script.Script, e script.Attacked) {
		apply(e.NPC, func(n *script.NPC) { _ = n.Stunned() })
	}}}
}

func apply(n *script.NPC, f func(*script.NPC)) { f(n) }

type step struct{ do func(*script.NPC, int8) }

var steps = []step{{do: func(n *script.NPC, _ int8) { _ = n.MaxHP() }}}

// Field calls it through a function-typed struct field held in a
// package-level slice.
func Field() script.Script {
	return script.Script{Hooks: script.Hooks{OnAttacked: func(_ *script.Script, e script.Attacked) {
		for _, s := range steps {
			s.do(e.NPC, 0)
		}
	}}}
}

var table = map[int]func(*script.NPC, uint8){1: func(n *script.NPC, _ uint8) { n.AbortAll() }}

// Map calls it through a package-level map element.
func Map() script.Script {
	return script.Script{Hooks: script.Hooks{OnAttacked: func(_ *script.Script, e script.Attacked) {
		table[1](e.NPC, 0)
	}}}
}

var ranged = []func(*script.NPC, int16){func(n *script.NPC, _ int16) { _ = n.Paralyzed() }}

// Range calls it through a range variable over a package-level slice.
func Range() script.Script {
	return script.Script{Hooks: script.Hooks{OnAttacked: func(_ *script.Script, e script.Attacked) {
		for _, f := range ranged {
			f(e.NPC, 0)
		}
	}}}
}

func lookup() func(*script.NPC, uint16) {
	return func(n *script.NPC, _ uint16) { _ = n.InCombat() }
}

// Returned calls it through a function a call returns.
func Returned() script.Script {
	return script.Script{Hooks: script.Hooks{OnAttacked: func(_ *script.Script, e script.Attacked) {
		lookup()(e.NPC, 0)
	}}}
}

func attacked(_ *script.Script, e script.Attacked) { _ = e.NPC.HP() }

// Invoker re-enters its attacked hook through the Hooks invoker from its
// talk hook.
func Invoker() script.Script {
	return script.Script{Hooks: script.Hooks{
		OnAttacked: attacked,
		OnTalk: func(s *script.Script, e script.Talk) string {
			s.Attacked(s, script.Attacked{NPC: e.NPC})
			return ""
		},
	}}
}

// Engine re-enters its attacked hook through the engine from its talk
// hook.
func Engine() script.Script {
	return script.Script{Hooks: script.Hooks{
		OnAttacked: attacked,
		OnTalk: func(s *script.Script, e script.Talk) string {
			s.RunAttacked(e.NPC, nil, 0)
			return ""
		},
	}}
}

// Wrapped re-enters its attacked hook through a content helper that calls
// the engine, from its timer hook.
func Wrapped() script.Script {
	return script.Script{Hooks: script.Hooks{
		OnAttacked: attacked,
		OnTimer: func(s *script.Script, e script.Timer) string {
			defaultnpc.MakeAttackEvent(s, e.NPC, nil, 0, false)
			return ""
		},
	}}
}

// Dispatch raises the party-attacked hooks of the scripts bound to another
// NPC from its talk hook: their invocations, not its own hook's.
func Dispatch() script.Script {
	return script.Script{Hooks: script.Hooks{
		OnPartyAttacked: func(_ *script.Script, e script.PartyAttacked) { _ = e.Called.HP() },
		OnTalk: func(s *script.Script, e script.Talk) string {
			s.RaisePartyAttacked(e.NPC, e.NPC, nil, 0)
			return ""
		},
	}}
}

var unset struct{ f func(*script.NPC, complex64) }

// Unmade calls through a field of a function type no script package makes
// a value of.
func Unmade() script.Script {
	return script.Script{Hooks: script.Hooks{OnAttacked: func(_ *script.Script, e script.Attacked) {
		unset.f(e.NPC, 0)
	}}}
}
