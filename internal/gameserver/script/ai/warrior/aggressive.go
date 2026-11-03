package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// Aggressive is WarriorAggressive: a Warrior that attacks the players and
// summons it sees.
func Aggressive() script.Script {
	return script.Script{Dir: aggressiveDir, Behavior: true, NPCs: aggressiveIDs(), Hooks: AggressiveHooks()}
}

// aggressiveDir is the directory of WarriorAggressive and the children
// that keep it.
const aggressiveDir = "ai/individual/Monster/WarriorBase/Warrior/WarriorAggressive"

// AggressiveHooks are WarriorAggressive's hooks, which its children derive
// theirs from.
func AggressiveHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			if !e.Creature.Playable() {
				return
			}
			defaultnpc.TryToAttack(s, e.NPC, e.Creature)
			base.SeeCreature(s, e)
		},
	}).Named("WarriorAggressive")
}

// aggressiveIDs are WarriorAggressive's own NPCs.
func aggressiveIDs() []int32 {
	return []int32{
		20080, 27219, 27220, 27221, 27222, 27223, 27224, 27226, 27227, 27228, 27229, 27230, 27231, 27232,
		27233, 27234, 27235, 27236, 27237, 27238, 27239, 27240, 27241, 27242, 27243, 27244, 27245, 27246,
		27247, 27248, 27249, 27128, 27308, 27273, 20386, 20387, 20372, 20108, 27262, 27263, 27264, 20310,
		20287, 20350, 27131, 20460, 20394, 20135, 27279, 27282, 27302, 27303, 27304, 27305, 27306, 27307,
		20253, 20205, 20557, 20455, 20285, 20504, 20783, 20464, 20574, 27133, 27281, 27280, 27274, 27275,
		27276, 27277, 27284, 20103, 20323, 20324, 27283, 20280, 27311, 20275, 20471, 20472, 20479, 20150,
		27310, 20603, 27132, 20356, 21100, 20024, 20362, 20070, 20025, 20655, 20072, 27124, 27127, 20582,
		20579, 27265, 20049, 20921, 20155, 20156, 20364, 27309, 20636, 20637, 20638, 20371, 27312, 27254,
		27255, 27256, 21125, 20161, 20162, 27129, 20782, 20093, 20592, 20466, 20508, 20060, 20313, 27278,
		20404, 20517, 20191, 20034, 20047, 20022, 20243, 20282, 27285, 20042, 20064, 20249, 20193, 20017,
		27030, 20342, 20343, 20012, 20018, 20071, 20099, 20111, 20133, 20138, 20140, 20141, 20143, 20148,
		20159, 20166, 20167, 20169, 20170, 20173, 20174, 20254, 20295, 20302, 20305, 20314, 20329, 20340,
		20367, 20375, 20402, 20410, 20425, 20435, 20461, 20462, 20489, 20491, 20502, 20516, 20519, 20527,
		20543, 20548, 20743, 20851, 20852, 21093, 21096, 21099, 21118, 21121, 21122, 21129, 21130, 21134,
		21139, 21142, 21256, 21667, 21690, 21713, 21736, 21759, 21782, 27029, 27193, 27225, 20100, 20190,
		20164, 20542, 20608, 22018, 22057,
	}
}
