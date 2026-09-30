package npc

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"sync/atomic"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npcinfo"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// folkInstanceKinds are the civilian service NPC types: shopkeepers,
// trainers, gatekeepers, warehouse keepers, village masters and the like.
// HolyThing, a civilian type too, is left out: the castle artifact belongs
// to the siege runtime.
var folkInstanceKinds = map[InstanceKind]struct{}{
	"Adventurer":            {},
	"Auctioneer":            {},
	"CastleBlacksmith":      {},
	"CastleChamberlain":     {},
	"CastleDoorman":         {},
	"CastleGatekeeper":      {},
	"CastleMagician":        {},
	"CastleWarehouseKeeper": {},
	"ChristmasTree":         {},
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

// Folk is a live civilian NPC. It stands where it spawned, is shown to
// nearby players, can be selected, and answers a player's interact with
// its chat window. It takes no part in combat: nothing damages it, it
// holds no effects, and it never casts, so its stats are fixed at spawn.
type Folk struct {
	world.Presence
	Instance *Instance

	maxHP          int
	pAtkSpd        int
	mAtkSpd        int
	moveMultiplier float64
	inPeace        bool

	// lastSocial is the Unix millisecond time of the last talk animation.
	lastSocial atomic.Int64
}

// NewFolk builds a civilian NPC from inst. skills, when provided, resolves
// the template's passive skills into the stats it is shown with. inPeace
// reports whether its spawn point lies in a peace zone.
func NewFolk(inst *Instance, inPeace bool, skills ...skillDefinitions) (*Folk, error) {
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
	t := inst.Template
	calc := func(s stat.Stat, base float64) float64 {
		c := effect.NewCalculator(defaultBuiltin(s))
		for _, m := range mods {
			if m.Stat == s {
				c.AddMod(m)
			}
		}
		v := c.Calc(templateStatActor{t: t}, base)
		if s.CantBeNegative() && v <= 0 {
			return 1
		}
		return v
	}
	f := &Folk{
		Instance: inst,
		maxHP:    int(calc(stat.MaxHP, t.HPMax)),
		pAtkSpd:  int(calc(stat.PowerAttackSpeed, t.AtkSpd)),
		mAtkSpd:  int(calc(stat.MagicAttackSpeed, magicAttackSpeedBase)),
		inPeace:  inPeace,
	}
	// The move speed over the base speed the stance picks, as a moving NPC
	// computes it; 0 for an NPC whose base is 0.
	base := int(t.RunSpeed)
	if inst.WalkMode {
		base = int(t.WalkSpeed)
	}
	if base != 0 {
		speed := float32(calc(stat.RunSpeed, float64(base)))
		f.moveMultiplier = float64(speed / float32(base))
	}
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

// MaxHP returns the NPC's maximum HP.
func (f *Folk) MaxHP() int { return f.maxHP }

// CurrentHP returns the NPC's HP, always full: nothing damages it.
func (f *Folk) CurrentHP() int { return f.maxHP }

// Running reports the run stance every spawned NPC takes, walk for a
// route walker.
func (f *Folk) Running() bool { return !f.Instance.WalkMode }

// Muted reports a civilian NPC a player's interact does nothing on.
func (f *Folk) Muted() bool { return hostileKind(f.Instance) == "MutedFolk" }

// TalkAnimation claims the talk animation an interact at now plays: a
// random social action id in [0, 8), at most one per socialInterval, the
// first one always. ok is false when none plays. A wedding manager greets
// with its own dialog and plays none.
func (f *Folk) TalkAnimation(now time.Time) (id int32, ok bool) {
	if f.Muted() || hostileKind(f.Instance) == "WeddingManagerNpc" {
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
	return npcinfo.Snapshot{
		ObjectID: f.ObjectID(), TemplateID: t.TemplateID,
		X: x, Y: y, Z: z, Heading: f.Heading(),
		MAtkSpd: f.mAtkSpd, PAtkSpd: f.pAtkSpd,
		RunSpd: int(t.RunSpeed), WalkSpd: int(t.WalkSpeed), MoveMultiplier: f.moveMultiplier,
		CurrentHP: f.maxHP, MaxHP: f.maxHP,
		CollisionRadius: t.CollisionRadius, CollisionHeight: t.CollisionHeight,
		RightHand: t.RightHand, LeftHand: t.LeftHand,
		Running: f.Running(), SummonAnimation: 2,
		Name: name, Title: title,
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
