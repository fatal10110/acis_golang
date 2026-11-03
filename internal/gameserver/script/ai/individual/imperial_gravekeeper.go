package individual

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// imperialSlaveID is the private the Imperial Gravekeeper calls in fours.
const imperialSlaveID = 27180

// ImperialGravekeeper is the Imperial Gravekeeper. Its first int slot picks
// where it sends the players of its territory, its second the HP share
// (50, 30, then none) below which a hit outside a peace zone sends them
// there, its third the HP share (80, 40, 20, then none) below which a hit
// calls four Imperial Slaves. Every hit outside a peace zone makes it cast
// its skill 4080 on itself; the shares rise back with its HP. It hates a
// playable that hits it, its party or its clan by the share of its HP the
// hit took, and drops every attack desire when it strays out of its
// territory.
func ImperialGravekeeper() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{27181}, Hooks: base.With(script.Hooks{
		OnNoDesire: func(_ *script.Script, e script.NoDesire) {
			e.NPC.AddWanderDesire(5, 5)
		},
		OnCreated: func(_ *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI0, 1)
			n.SetInt(script.SlotIntAI1, 50)
			n.SetInt(script.SlotIntAI2, 80)
			n.SetInt(script.SlotWeightPoint, 10)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			hpRatio := n.HPRatio()

			if !n.InsideZone(script.ZonePeace) {
				if hpRatio*100 <= float64(n.Int(script.SlotIntAI1)) {
					if ai0 := n.Int(script.SlotIntAI0); ai0 == 1 || ai0 == 3 {
						s.TeleportPlayersInTerritory(n, script.Loc{X: 179520, Y: 6464, Z: -2706}, 200)
					} else {
						s.TeleportPlayersInTerritory(n, script.Loc{X: 171104, Y: 6496, Z: -2706}, 200)
					}
				}

				n.AddCastDesire(n, skill.Ref{ID: 4080, Level: 1}, 1000000)

				switch n.Int(script.SlotIntAI1) {
				case 50:
					n.SetInt(script.SlotIntAI1, 30)
				case 30:
					n.SetInt(script.SlotIntAI1, -1)
				}
			}

			if hpRatio*100 <= float64(n.Int(script.SlotIntAI2)) {
				switch n.Int(script.SlotIntAI2) {
				case 80:
					n.SetInt(script.SlotIntAI2, 40)
				case 40:
					n.SetInt(script.SlotIntAI2, 20)
				default:
					n.SetInt(script.SlotIntAI2, -1)
				}

				s.Spawner().CreateOnePrivate(n, imperialSlaveID, 0)
				s.Spawner().CreateOnePrivate(n, imperialSlaveID, 0)
				s.Spawner().CreateOnePrivate(n, imperialSlaveID, 0)
				s.Spawner().CreateOnePrivate(n, imperialSlaveID, 0)
			}

			if hpRatio > 0.5 {
				n.SetInt(script.SlotIntAI1, 50)
			} else if hpRatio > 0.3 {
				n.SetInt(script.SlotIntAI1, 30)
			}

			if hpRatio > 0.8 {
				n.SetInt(script.SlotIntAI2, 80)
			} else if hpRatio > 0.4 {
				n.SetInt(script.SlotIntAI2, 40)
			} else if hpRatio > 0.2 {
				n.SetInt(script.SlotIntAI2, 20)
			}

			if e.Attacker.Playable() {
				n.AddAttackDesire(e.Attacker, float64(e.Damage)/float64(n.MaxHP())/0.05*100)
			}
		},
		OnPartyAttacked: func(_ *script.Script, e script.PartyAttacked) {
			if e.Target.Playable() {
				e.Called.AddAttackDesire(e.Target, float64(e.Damage)/float64(e.Called.MaxHP())/0.05*50)
			}
		},
		OnClanAttacked: func(_ *script.Script, e script.ClanAttacked) {
			if e.Attacker.Playable() {
				e.Called.AddAttackDesire(e.Attacker, float64(e.Damage)/float64(e.Called.MaxHP())/0.05*50)
			}
		},
		OnOutOfTerritory: func(_ *script.Script, e script.OutOfTerritory) {
			e.NPC.RemoveAllAttackDesire()
		},
	})}
}
