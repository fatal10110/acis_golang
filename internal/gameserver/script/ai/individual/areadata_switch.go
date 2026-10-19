package individual

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// AreadataSwitch is Triol's Revelation: it speaks its line as it spawns.
// The area it names (its AreaName AI parameter) is switched by nothing.
func AreadataSwitch() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{
		Dir: dir, Behavior: true,
		NPCs: []int32{32058, 32059, 32060, 32061, 32062, 32063, 32064, 32065, 32066, 32067, 32068},
		Hooks: base.With(script.Hooks{
			OnCreated: func(s *script.Script, e script.Created) {
				e.NPC.Say(script.NpcString(1010472))
				base.Created(s, e)
			},
			OnMyDying: func(s *script.Script, e script.MyDying) {
				base.MyDying(s, e)
			},
		}),
	}
}
