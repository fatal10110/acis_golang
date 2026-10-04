package npc

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"sync/atomic"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npcinfo"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// folkInstanceKinds are the civilian service NPC types: shopkeepers,
// trainers, gatekeepers, warehouse keepers, village masters and the like.
// Two civilian types are left out and skipped at spawn: HolyThing, as the
// castle artifact belongs to the siege runtime, and ChristmasTree, whose
// click only releases the client (no selection, no talk) and which spawns
// only from event makers or as a summoned Decoration.
var folkInstanceKinds = map[InstanceKind]struct{}{
	"Adventurer":            {},
	"Auctioneer":            {},
	"CastleBlacksmith":      {},
	"CastleChamberlain":     {},
	"CastleDoorman":         {},
	"CastleGatekeeper":      {},
	"CastleMagician":        {},
	"CastleWarehouseKeeper": {},
	"ClanHallDoorman":       {},
	"ClanHallManagerNpc":    {},
	"ClassMaster":           {},
	"DawnPriest":            {},
	"DerbyTrackManagerNpc":  {},
	"Doorman":               {},
	"DungeonGatekeeper":     {},
	"DuskPriest":            {},
	"FestivalGuide":         {},
	"Fisherman":             {},
	"Folk":                  {},
	"Gatekeeper":            {},
	"ManorManagerNpc":       {},
	"MercenaryManagerNpc":   {},
	"Merchant":              {},
	"MutedFolk":             {},
	"OlympiadManagerNpc":    {},
	"SchemeBuffer":          {},
	"SiegeNpc":              {},
	"SignsPriest":           {},
	"SymbolMaker":           {},
	"Trainer":               {},
	"VillageMaster":         {},
	"VillageMasterDElf":     {},
	"VillageMasterDwarf":    {},
	"VillageMasterFighter":  {},
	"VillageMasterMystic":   {},
	"VillageMasterOrc":      {},
	"VillageMasterPriest":   {},
	"WarehouseKeeper":       {},
	"WeddingManagerNpc":     {},
	"WyvernManagerNpc":      {},
}

// FolkKind reports whether inst's instance type is a civilian service NPC
// NewFolk accepts.
func FolkKind(inst *Instance) bool {
	_, ok := folkInstanceKinds[hostileKind(inst)]
	return ok
}

// magicAttackSpeedBase is every creature's base casting speed.
const magicAttackSpeedBase = 333

// socialInterval is the least time between two talk animations of one NPC.
const socialInterval = 12 * time.Second

// Folk is a live civilian NPC. It stands where it spawned, or walks its
// route when it has one, is shown to nearby players, can be selected, and
// answers a player's interact with its chat window. It never attacks, but
// casts the skills a dialog command or script asks of it once given a cast
// runtime (SetCaster), walking toward its target first when it can move
// (EnableMovement). Any other creature may attack
// it by force: it takes damage, regenerates, and holds the buffs and
// debuffs cast on it (no other effect lands on it). An undying template
// keeps at least 1 HP; any other dies, and its corpse decays.
type Folk struct {
	world.Presence
	Instance *Instance
	// zones is the NPC's zone membership; Attach gives it the zone index.
	zones *zoneMember

	// motion is the movement of an NPC that can move; nil for one that
	// cannot. EnableMovement sets it before the NPC is published.
	motion *folkMotion

	// lastSocial is the Unix millisecond time of the last talk animation.
	lastSocial atomic.Int64

	folkCombat

	cast folkCast
}

// NewFolk builds a civilian NPC from inst. skills, when provided, resolves
// the template's passive skills into its stats. Attach gives it the runtime
// it fights with before it is published.
func NewFolk(inst *Instance, skills ...skillDefinitions) (*Folk, error) {
	if inst == nil || inst.Template == nil {
		return nil, errors.New("npc: nil folk instance")
	}
	if !FolkKind(inst) {
		return nil, fmt.Errorf("npc %d: instance type %q is not a folk type", inst.Template.ID, hostileKind(inst))
	}
	var lookup skillDefinitions
	if len(skills) > 0 {
		lookup = skills[0]
	}
	mods, err := effect.TemplatePassiveMods(lookup, inst.Template.Passives)
	if err != nil {
		return nil, fmt.Errorf("npc %d template passives: %w", inst.Template.ID, err)
	}
	f := &Folk{Instance: inst}
	f.zones = newZoneMember(f)
	f.cast.desires = ai.NewDesireQueue()
	f.initCombat(mods)
	return f, nil
}

// ObjectID returns this NPC's world object id.
func (f *Folk) ObjectID() int32 { return f.Instance.ObjectID }

// Kind reports KindNPC.
func (f *Folk) Kind() actor.Kind { return actor.KindNPC }

// NpcID returns the template id the NPC's data pages are named after.
func (f *Folk) NpcID() int { return f.Instance.Template.ID }

// Level returns the template level.
func (f *Folk) Level() int { return f.Instance.Template.Level }

// CollisionRadius returns the template body radius.
func (f *Folk) CollisionRadius() float64 { return f.Instance.Template.CollisionRadius }

// CollisionHeight returns the template body height.
func (f *Folk) CollisionHeight() float64 { return f.Instance.Template.CollisionHeight }

// weddingManager is the wedding manager's type.
const weddingManager InstanceKind = "WeddingManagerNpc"

// Muted reports a civilian NPC a player's interact does nothing on.
func (f *Folk) Muted() bool { return hostileKind(f.Instance) == "MutedFolk" }

// TalkAnimation claims the talk animation an interact at now plays: a
// random social action id in [0, 8), at most one per socialInterval, the
// first one always. ok is false when none plays. A wedding manager greets
// with its own dialog and plays none.
func (f *Folk) TalkAnimation(now time.Time) (id int32, ok bool) {
	if f.Muted() || hostileKind(f.Instance) == weddingManager {
		return 0, false
	}
	ms := now.UnixMilli()
	for {
		last := f.lastSocial.Load()
		if last != 0 && ms-last <= socialInterval.Milliseconds() {
			return 0, false
		}
		if f.lastSocial.CompareAndSwap(last, ms) {
			return int32(rand.IntN(8)), true
		}
	}
}

// NPCInfoSnapshot captures this NPC's client-visible state.
func (f *Folk) NPCInfoSnapshot() npcinfo.Snapshot {
	t := f.Instance.Template
	x, y, z := f.Position()
	name, title := "", ""
	if t.UsingServerSideName {
		name = t.Name
	}
	if t.UsingServerSideTitle {
		title = t.Title
	}
	pAtkSpd := f.AttackSpeed()
	return npcinfo.Snapshot{
		ObjectID: f.ObjectID(), TemplateID: t.TemplateID,
		X: x, Y: y, Z: z, Heading: f.Heading(),
		MAtkSpd: f.MagicAttackSpeed(), PAtkSpd: pAtkSpd,
		RunSpd: int(t.RunSpeed), WalkSpd: int(t.WalkSpeed),
		MoveMultiplier: float64(f.MovementSpeedMultiplier()), AtkSpdMultiplier: npcinfo.AttackSpeedMultiplier(pAtkSpd, t.AtkSpd),
		MoveType:  f.zones.moveType(),
		CurrentHP: f.CurrentHP(), MaxHP: f.MaxHP(),
		CollisionRadius: t.CollisionRadius, CollisionHeight: t.CollisionHeight,
		RightHand: t.RightHand, LeftHand: t.LeftHand,
		Running: f.Running(), InCombat: f.InCombat(), AlikeDead: f.AlikeDead(), SummonAnimation: 2,
		AbnormalEffect: f.AbnormalEffect(), Name: name, Title: title,
	}
}

// Owner reports no owner: an NPC is not a summon.
func (f *Folk) Owner() (attackable.Combatant, bool) { return nil, false }

// templateStatActor adapts a template's attributes to the stat funcs.
type templateStatActor struct{ t *Template }

var _ stat.Actor = templateStatActor{}

func (a templateStatActor) STR() int { return a.t.STR }
func (a templateStatActor) CON() int { return a.t.CON }
func (a templateStatActor) DEX() int { return a.t.DEX }
func (a templateStatActor) INT() int { return a.t.INT }
func (a templateStatActor) WIT() int { return a.t.WIT }
func (a templateStatActor) MEN() int { return a.t.MEN }

func (a templateStatActor) Level() int {
	if a.t.Level <= 0 {
		return 1
	}
	return a.t.Level
}

func (a templateStatActor) LevelMod() float64 { return (89 + float64(a.Level())) / 100 }

func (a templateStatActor) IsSummon() bool { return false }
