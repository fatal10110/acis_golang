package door

import (
	"errors"
	"fmt"
	"math"
	"sync/atomic"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// ErrEmptyFootprint reports a door whose triangulated footprint sampled to
// no geodata cells. The door loader logs and skips such a door rather than
// treating it as fatal.
var ErrEmptyFootprint = errors.New("door: empty geo shape")

// Kind classifies a door template as a regular door or a wall.
type Kind uint8

const (
	KindDoor Kind = iota
	KindWall
)

// ParseKind maps a door's XML type spelling onto its Kind.
func ParseKind(s string) (Kind, error) {
	for k, name := range kindStrings {
		if name == s {
			return Kind(k), nil
		}
	}
	return 0, fmt.Errorf("door: unknown type %q", s)
}

var kindStrings = [...]string{"DOOR", "WALL"}

// String returns k's canonical XML spelling.
func (k Kind) String() string {
	if int(k) < len(kindStrings) {
		return kindStrings[k]
	}
	return fmt.Sprintf("Kind(%d)", uint8(k))
}

// OpenKind classifies what opens a door.
type OpenKind uint8

const (
	OpenClick OpenKind = iota
	OpenTime
	OpenSkill
	OpenNPC
)

// ParseOpenKind maps a door's XML openType spelling onto its OpenKind.
func ParseOpenKind(s string) (OpenKind, error) {
	for o, name := range openKindStrings {
		if name == s {
			return OpenKind(o), nil
		}
	}
	return 0, fmt.Errorf("door: unknown openType %q", s)
}

var openKindStrings = [...]string{"CLICK", "TIME", "SKILL", "NPC"}

// String returns o's canonical XML spelling.
func (o OpenKind) String() string {
	if int(o) < len(openKindStrings) {
		return openKindStrings[o]
	}
	return fmt.Sprintf("OpenKind(%d)", uint8(o))
}

// Template is one static door entry from doors.xml.
type Template struct {
	ID    int
	Name  string
	Kind  Kind
	Level int

	Position    location.Location
	Coordinates []location.Point

	HP, PDef, MDef, Height int

	CastleID, ClanHallID, TriggeredID int
	Opened                            bool
	OpenKind                          OpenKind
	OpenTime, RandomTime, CloseTime   int
}

// CollisionRadius is the body radius every door has, whatever its footprint:
// what a range check to a door target, such as a cast's approach to it, adds
// for the door.
const CollisionRadius = 16

// GeoShape is the geodata footprint calculated for a door.
type GeoShape interface {
	GeoX() int
	GeoY() int
	GeoZ() int
	Height() int
	GeoData() [][]block.NSWE
}

// Object is one live door spawned into the world.
//
// Position and visibility are guarded by the embedded world.Presence.
type Object struct {
	world.Presence

	objectID int32
	Template *Template

	geoX, geoY, geoZ int
	height           int
	geoData          [][]block.NSWE

	opened atomic.Bool

	// hp holds the current HP as math.Float64bits; it and dead change only
	// through the state owner, under the lock it applies door changes with.
	hp     atomic.Uint64
	dead   atomic.Bool
	siege  atomic.Pointer[siegeRef]
	strife atomic.Pointer[strifeRef]
	// hpUpgrade multiplies the template HP into the maximum HP: 1 until
	// the castle owning the door upgrades it. It changes only through the
	// state owner, like hp.
	hpUpgrade atomic.Int32

	sink  event.Sink
	owner StateOwner
	sight Sight
}

// Siege is the siege state of the residence a door belongs to. A door with
// no siege can be neither attacked nor damaged, whoever hits it.
type Siege interface {
	// AllowsDoorAttack reports whether attacker, a playable, may attack the
	// door: during a castle siege in progress only the attacker side may,
	// and a wall only through a siege summon other than the anti-infantry
	// Swoop Cannon; during a siegable clan hall's siege only the attacker
	// side may, once the siege lets its doors be attacked.
	AllowsDoorAttack(attacker skilltarget.Actor, wall bool) bool
	// AllowsDoorDamage reports whether a hit lowers the door's HP: a
	// castle's siege in progress lets every attacker but the Swoop Cannon
	// through, and a siegable clan hall's active siege zone any attacker.
	AllowsDoorDamage(attacker attackable.Combatant) bool
	// ShowsDoorHP reports whether the door's status updates show its HP:
	// only while its castle's siege is in progress.
	ShowsDoorHP() bool
	// DoorBroken tells the residence one of its doors broke down: during
	// a castle siege in progress the defenders hear that a wall was
	// damaged or a gate broken down.
	DoorBroken(wall bool)
}

type siegeRef struct{ siege Siege }

// Strife is the Seven Signs state a door's defences read: the owner of each
// seal, in Avarice, Gnosis, Strife order.
type Strife interface {
	SealOwners() [3]sevensigns.Cabal
}

type strifeRef struct{ strife Strife }

// HPRegen is the HP a damaged door regains on each regeneration tick: the
// creature base HP regeneration, which no door overrides.
const HPRegen = 1.5

// Sight is the geodata line-of-sight query a door answers skill sight with:
// mutual sight between two points, each raised by the eye height its
// collision height gives, with ignore's own geodata edits left out of the
// query when ignore is non-nil.
type Sight interface {
	CanSeeActorIgnoring(ox, oy, oz int, oCollisionHeight float64, tx, ty, tz int, tCollisionHeight float64, ignore GeoShape) bool
}

// StateOwner applies a door's open/close change together with everything
// that follows it: the geodata blocker, the status broadcast, a linked
// door, and the auto open/close timer. It applies HP loss the same way,
// with the regeneration schedule and, once HP runs out, the door's death.
type StateOwner interface {
	SetDoorOpen(id int, open bool) bool
	ReduceDoorHP(id int, amount float64)
	TakeDoorHP(id int, amount float64)
}

// NewObject creates a live door object from a static template and geodata shape.
func NewObject(objectID int32, tmpl *Template, shape GeoShape) (*Object, error) {
	if tmpl == nil {
		return nil, fmt.Errorf("door: nil template")
	}
	if shape == nil {
		return nil, fmt.Errorf("door %d: nil geo shape", tmpl.ID)
	}
	data := shape.GeoData()
	if len(data) == 0 || len(data[0]) == 0 {
		return nil, fmt.Errorf("door %d: %w", tmpl.ID, ErrEmptyFootprint)
	}

	o := &Object{
		objectID: objectID,
		Template: tmpl,
		geoX:     shape.GeoX(),
		geoY:     shape.GeoY(),
		geoZ:     shape.GeoZ(),
		height:   shape.Height(),
		geoData:  cloneGeoData(data),
	}
	o.opened.Store(tmpl.Opened)
	o.hpUpgrade.Store(1)
	o.StoreHP(float64(tmpl.HP))
	return o, nil
}

// ObjectID returns the world object id assigned to this door.
func (o *Object) ObjectID() int32 { return o.objectID }

// Dead reports whether the door has been broken down.
func (o *Object) Dead() bool { return o.dead.Load() }

// SetDead updates only the door's dead flag and reports whether it changed.
// It is the state owner's primitive, like SetOpened.
func (o *Object) SetDead(dead bool) bool {
	return o.dead.CompareAndSwap(!dead, dead)
}

// Unlockable reports whether a skill can open this door.
func (o *Object) Unlockable() bool { return o.Template.OpenKind == OpenSkill }

// AttackableBy reports whether attacker may attack the door: a standing
// door is attacked only by a playable its residence's siege lets through.
func (o *Object) AttackableBy(attacker skilltarget.Actor) bool {
	if o.Dead() || attacker == nil || attacker.ObjectID() == o.objectID || !attacker.Kind().Playable() {
		return false
	}
	ref := o.siege.Load()
	return ref != nil && ref.siege.AllowsDoorAttack(attacker, o.Wall())
}

// Wall reports whether the door is a castle wall rather than a gate.
func (o *Object) Wall() bool { return o.Template.Kind == KindWall }

// ShowsHP reports whether the door's status updates show its HP: only
// while its residence's siege lets them.
func (o *Object) ShowsHP() bool {
	ref := o.siege.Load()
	return ref != nil && ref.siege.ShowsDoorHP()
}

// TellBroken tells the door's residence it broke down. The state owner
// calls it once the door's death has been applied and broadcast.
func (o *Object) TellBroken() {
	if ref := o.siege.Load(); ref != nil {
		ref.siege.DoorBroken(o.Wall())
	}
}

// AttackableWithoutForceBy matches the door attackability rule.
func (o *Object) AttackableWithoutForceBy(caster skilltarget.Actor) bool {
	return o.AttackableBy(caster)
}

// Kind reports KindDoor.
func (o *Object) Kind() actor.Kind { return actor.KindDoor }

// DoorID returns the static door id from doors.xml.
func (o *Object) DoorID() int { return o.Template.ID }

// Level returns the door's template level.
func (o *Object) Level() int { return o.Template.Level }

// CharacterName returns the door's template name, the name messages show.
func (o *Object) CharacterName() string { return o.Template.Name }

// SetStrife installs the Seven Signs state the door's defences read.
func (o *Object) SetStrife(s Strife) {
	if s == nil {
		o.strife.Store(nil)
		return
	}
	o.strife.Store(&strifeRef{strife: s})
}

// PhysicalDefence returns the door's P.Def: the template value under the
// Seal of Strife, truncated.
func (o *Object) PhysicalDefence() int { return o.underStrife(o.Template.PDef) }

// MagicDefence returns the door's M.Def: the template value under the Seal
// of Strife, truncated.
func (o *Object) MagicDefence() int { return o.underStrife(o.Template.MDef) }

// underStrife scales a template defence by the Seal of Strife: raised by a
// fifth while Dawn owns the seal, cut to 30% while Dusk does.
func (o *Object) underStrife(base int) int {
	factor := 1.0
	if ref := o.strife.Load(); ref != nil {
		switch ref.strife.SealOwners()[sevensigns.Strife-sevensigns.Avarice] {
		case sevensigns.Dawn:
			factor = 1.2
		case sevensigns.Dusk:
			factor = 0.3
		}
	}
	return int(float64(base) * factor)
}

// MaxHP returns the door's maximum HP: its template HP times its HP
// upgrade ratio.
func (o *Object) MaxHP() int { return o.Template.HP * o.HPUpgradeRatio() }

// HPUpgradeRatio returns the ratio the door's castle upgraded its maximum
// HP by, 1 for none.
func (o *Object) HPUpgradeRatio() int { return int(o.hpUpgrade.Load()) }

// StoreHPUpgradeRatio updates only the door's HP upgrade ratio. It is the
// state owner's primitive, like StoreHP.
func (o *Object) StoreHPUpgradeRatio(ratio int) { o.hpUpgrade.Store(int32(ratio)) }

// CurrentHP returns the door's current HP.
func (o *Object) CurrentHP() float64 { return math.Float64frombits(o.hp.Load()) }

// StoreHP updates only the door's current HP. It is the state owner's
// primitive: the owner clamps the value and applies what follows it.
func (o *Object) StoreHP(hp float64) { o.hp.Store(math.Float64bits(hp)) }

// HP returns the door's current HP truncated to the whole points status
// packets carry.
func (o *Object) HP() int { return int(o.CurrentHP()) }

// HPRatio returns current HP over maximum HP.
func (o *Object) HPRatio() float64 { return o.CurrentHP() / float64(o.MaxHP()) }

// Damage returns the visual damage stage sent in door status packets: 0 at
// full HP up to 6 once the door is broken, one stage per sixth of HP lost.
func (o *Object) Damage() int {
	ratio := o.HPRatio()
	if math.IsNaN(ratio) {
		// 0/0: the stage a zero-HP template reports.
		return 6
	}
	return max(0, min(6, 6-int(math.Ceil(ratio*6))))
}

// SetSiege installs the siege of the residence this door belongs to, the
// only state that lets a hit lower the door's HP.
func (o *Object) SetSiege(s Siege) {
	if s == nil {
		o.siege.Store(nil)
		return
	}
	o.siege.Store(&siegeRef{siege: s})
}

// ReduceHP applies a hit's damage to the door: an auto-attack's, with no
// skill, or a damage skill's. Every hit routed onto a door ends here. Only a
// hit its residence's siege allows lowers HP, and an attacker whose access
// level deals no damage lowers nothing; the state owner applies the loss,
// its status broadcast, the regeneration schedule and, once HP runs out,
// the door's death.
func (o *Object) ReduceHP(amount float64, attacker attackable.Combatant, _ modelskill.Definition) {
	ref := o.siege.Load()
	if ref == nil || !ref.siege.AllowsDoorDamage(attacker) {
		return
	}
	if attacker != nil && !attacker.CanGiveDamage() {
		return
	}
	if o.owner != nil {
		o.owner.ReduceDoorHP(o.DoorID(), amount)
	}
}

// TakeHP takes amount straight off the door's HP, as a REAL_DAMAGE skill
// does: no siege permission and no attacker check apply. A loss leaving
// zero HP or less breaks the door; any other sets its HP. The state owner
// applies the change and what follows it.
func (o *Object) TakeHP(amount float64) {
	if o.owner != nil {
		o.owner.TakeDoorHP(o.DoorID(), amount)
	}
}

// Opened reports whether this door is currently open.
func (o *Object) Opened() bool {
	return o.opened.Load()
}

// SetOpened updates only the door's open flag and reports whether it
// changed. It is the state owner's primitive; everything else opens a door
// with Open, which also applies the change's world side effects.
func (o *Object) SetOpened(open bool) bool {
	return o.opened.CompareAndSwap(!open, open)
}

// Open opens this door through its state owner and reports whether it was
// closed. A door with no owner stays as it is.
func (o *Object) Open() bool {
	if o.owner == nil {
		return false
	}
	return o.owner.SetDoorOpen(o.DoorID(), true)
}

// Attach installs sink as the receiver of this door's events. Call it once,
// before the door is spawned.
func (o *Object) Attach(sink event.Sink) { o.sink = sink }

// SetOwner installs the owner that applies this door's state changes. Call
// it once, before the door is spawned.
func (o *Object) SetOwner(owner StateOwner) { o.owner = owner }

// SetSight installs the geodata query CanSeeTarget runs. Call it once,
// before the door is spawned.
func (o *Object) SetSight(sight Sight) { o.sight = sight }

// BroadcastStatus reports this door's current open/close state, HP and
// damage stage to observers.
func (o *Object) BroadcastStatus() {
	if o.sink != nil {
		o.sink.Emit(event.StatusChanged{})
	}
}

// BroadcastRevive reports this door standing again to observers.
func (o *Object) BroadcastRevive() {
	if o.sink != nil {
		o.sink.Emit(event.Revived{})
	}
}

// GeoX returns the door footprint's starting geodata X coordinate.
func (o *Object) GeoX() int { return o.geoX }

// GeoY returns the door footprint's starting geodata Y coordinate.
func (o *Object) GeoY() int { return o.geoY }

// GeoZ returns the door footprint's baseline geodata Z coordinate.
func (o *Object) GeoZ() int { return o.geoZ }

// Height returns the vertical span the door blocks.
func (o *Object) Height() int { return o.height }

// GeoData returns the door footprint's NSWE edits.
func (o *Object) GeoData() [][]block.NSWE { return cloneGeoData(o.geoData) }

func cloneGeoData(data [][]block.NSWE) [][]block.NSWE {
	out := make([][]block.NSWE, len(data))
	for i := range data {
		out[i] = append([]block.NSWE(nil), data[i]...)
	}
	return out
}

// Table stores door templates keyed by door id.
type Table struct {
	doors map[int]*Template
	order []*Template
}

// NewTable builds a door table and rejects duplicate ids.
func NewTable(templates []*Template) (*Table, error) {
	t := &Table{
		doors: make(map[int]*Template, len(templates)),
		order: make([]*Template, 0, len(templates)),
	}
	for _, tmpl := range templates {
		if tmpl == nil {
			return nil, fmt.Errorf("door: nil template")
		}
		if _, exists := t.doors[tmpl.ID]; exists {
			return nil, fmt.Errorf("door: duplicate template id %d", tmpl.ID)
		}
		t.doors[tmpl.ID] = tmpl
		t.order = append(t.order, tmpl)
	}
	return t, nil
}

// Len returns the number of loaded door templates.
func (t *Table) Len() int {
	if t == nil {
		return 0
	}
	return len(t.order)
}

// Get returns the template for id.
func (t *Table) Get(id int) (*Template, bool) {
	if t == nil {
		return nil, false
	}
	tmpl, ok := t.doors[id]
	return tmpl, ok
}

// All returns the loaded templates in file order.
func (t *Table) All() []*Template {
	if t == nil {
		return nil
	}
	return append([]*Template(nil), t.order...)
}
