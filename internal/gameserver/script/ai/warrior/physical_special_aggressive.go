package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// PhysicalSpecialAggressive is WarriorPhysicalSpecialAggressive: a
// WarriorPhysicalSpecial that attacks the players and summons it sees and,
// settled in its territory and not attacking, may strike the one it most
// wants to act on with its physical special skill when it stands farther
// than 100.
func PhysicalSpecialAggressive() script.Script {
	return script.Script{Dir: physicalSpecialDir, Behavior: true, NPCs: physicalSpecialAggressiveIDs(), Hooks: PhysicalSpecialAggressiveHooks()}
}

// PhysicalSpecialAggressiveHooks are WarriorPhysicalSpecialAggressive's
// hooks, which its children derive theirs from.
func PhysicalSpecialAggressiveHooks() script.Hooks {
	base := PhysicalSpecialHooks()
	return base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if c.Playable() {
				if n.Intention() != script.IntentionAttack && n.LifeTime() > 7 && n.InMyTerritory() {
					if top := n.TopDesireTarget(); top != nil && script.Same(top, c) && n.Distance2D(c) > 100 && s.Rnd(100) < 33 {
						castPhysicalSpecial(n, c)
					}
				}
				defaultnpc.TryToAttack(s, n, c)
			}
			base.SeeCreature(s, e)
		},
	}).Named("WarriorPhysicalSpecialAggressive")
}

// physicalSpecialAggressiveIDs are WarriorPhysicalSpecialAggressive's own
// NPCs.
func physicalSpecialAggressiveIDs() []int32 {
	return []int32{
		21427, 21022, 21018, 20270, 20271, 27140, 27141, 27142, 21270, 21272, 20239, 21638, 20291, 27217,
		21238, 21242, 20824, 20818, 20645, 21403, 20927, 20650, 20924, 20626, 20625, 20157, 20090, 27190,
		20845, 21629, 21630, 20823, 21069, 21065, 21378, 21377, 21376, 21222, 21226, 21230, 21021, 21259,
		21398, 20634, 20669, 20632, 20837, 20801, 20585, 21246, 21250, 21254, 27218, 20683, 21210, 21214,
		21218, 20798, 20843, 21623, 21624, 20165, 20257, 20259, 20300, 20304, 20374, 20422, 20615, 20622,
		20832, 20857, 20930, 20933, 21014, 21015, 21120, 21123, 21150, 21151, 21158, 21234, 21296, 21326,
		21330, 21333, 21337, 21341, 21438, 21439, 21441, 21442, 21450, 21602, 21603, 21604, 21625, 21631,
		21660, 21662, 21668, 21672, 21673, 21677, 21678, 21679, 21680, 21683, 21685, 21691, 21695, 21696,
		21700, 21701, 21702, 21703, 21706, 21708, 21714, 21718, 21719, 21723, 21724, 21725, 21726, 21729,
		21731, 21737, 21741, 21742, 21746, 21747, 21748, 21749, 21752, 21754, 21760, 21764, 21765, 21769,
		21770, 21771, 21772, 21775, 21777, 21783, 21787, 21788, 21792, 21793, 21794, 21795, 27191, 27196,
		27197, 22006, 22105, 22106, 22107, 22115, 22116, 22117, 22219, 22220, 22221, 22222,
	}
}

// PhysicalSpecialAreaAggressive is WarriorPhysicalSpecialAreaAggressive: a
// WarriorPhysicalSpecialAggressive whose created and dying hooks switch
// nothing on or off: the area its AreaName AI parameter names stays as it
// is.
func PhysicalSpecialAreaAggressive() script.Script {
	base := PhysicalSpecialAggressiveHooks()
	return script.Script{Dir: physicalSpecialDir, Behavior: true, NPCs: []int32{22011, 22012, 22013, 22014, 22015, 22016}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			base.Created(s, e)
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			base.MyDying(s, e)
		},
	})}
}

// PhysicalSpecialGatheringAggressive is
// WarriorPhysicalSpecialGatheringAggressive: a
// WarriorPhysicalSpecialAggressive that, every 5 s from its creation, sends
// the NPCs within 500 of it script event 10019 carrying its object id.
func PhysicalSpecialGatheringAggressive() script.Script {
	base := PhysicalSpecialAggressiveHooks()
	return script.Script{Dir: physicalSpecialDir, Behavior: true, NPCs: []int32{21676, 21699, 21722, 21745, 21768, 21791}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			s.StartTimerAtFixedRate("3100", e.NPC, nil, 5000*time.Millisecond, 5000*time.Millisecond)
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "3100") {
				s.BroadcastScriptEvent(e.NPC, 10019, e.NPC.ObjectID(), 500)
			}
			return base.Timer(s, e)
		},
	})}
}
