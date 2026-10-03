package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/boat"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npcinfo"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func (p *livePlayer) Discover(obj world.Tracked) { p.sendInfoFrom(obj, false) }

// requestRecordInfo answers the client's view resync: live's UserInfo, then
// everything live knows resent as it was first seen.
func (l *GameClientLink) requestRecordInfo(live *livePlayer) {
	live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
	live.refreshInfos(l.world)
}

// refreshInfos resends p everything it knows, each object as p first saw
// it: its info packet and what it is doing. A known player in observer mode
// is left out. It runs on p's queue.
func (p *livePlayer) refreshInfos(w *world.State) {
	sim.AssertOwner(p.Queue())
	if w == nil {
		return
	}
	w.ForEachKnown(p, func(obj world.Tracked) {
		if o, ok := obj.(*livePlayer); ok && o.ObserverMode() {
			return
		}
		p.sendInfoFrom(obj, true)
	})
}

// sendInfoFrom sends p obj's info packet and, for a creature, what it is
// doing (describeState). onQueue reports that the caller runs on p's own
// queue, where an owned pet's item list and state follow its PetInfo at
// once; otherwise they are posted there.
func (p *livePlayer) sendInfoFrom(obj world.Tracked, onQueue bool) {
	switch o := obj.(type) {
	case *livePlayer:
		p.sendVisibilityFrame(serverpackets.FrameCharInfo(serverpackets.CharInfoSnapshot{
			Character: o.Character,
			Template:  o.Template(),
			Items:     o.inventoryItems(),
			Clan:      p.link.clanFields(o.Character),
			Hidden:    hiddenFrom(o, p),
		}))
		if o.throne != nil {
			p.sendVisibilityFrame(serverpackets.FrameChairSit(o.ObjectID(), o.throne.StaticObjectID()))
		}
		// Both players learn their relation to the other: p gets o's (and
		// its summon's), o gets p's (and its summon's).
		if l := p.link; l != nil {
			l.sendRelations(o, l.summonOf(o), p.Character, p.sendVisibilityFrame)
			l.sendRelations(p, l.summonOf(p), o.Character, o.SendFrame)
		}
		if title, ok := storeTitleFrame(o); ok {
			p.sendVisibilityFrame(title)
		}
		p.describeState(o, o.CastControl())
	case *npc.Hostile:
		p.sendVisibilityFrame(serverpackets.FrameNPCInfo(o.NPCInfoSnapshot()))
		p.describeState(o, o.CastControl())
	case *npc.Decoration:
		p.sendVisibilityFrame(serverpackets.FrameNPCInfo(o.NPCInfoSnapshot()))
	case *npc.Folk:
		p.sendVisibilityFrame(serverpackets.FrameNPCInfo(o.NPCInfoSnapshot()))
		p.describeState(o, o.CastControl())
	case *summon.Actor:
		if o.ShownAsOwnedBy(p.ObjectID()) {
			o.MarkDiscoveredByOwner()
			if snap, ok := petInfoSnapshot(o, p, p.npcs); ok {
				p.sendVisibilityFrame(serverpackets.FramePetInfo(snap))
				// PetInfo clears the summon's icons on the client.
				refreshSummonPartySpelled(o)
				if inv := o.PetInventory(); inv != nil {
					// PetInventoryUpdate is drained and sent on p's queue, but
					// discovery can run on another actor's: a summon-friend cast
					// teleports p from the caster's queue. Building the snapshot
					// there would let an update drained after it overtake it, so
					// the snapshot is always taken and sent on p's queue. The
					// pet's state follows its item list there.
					seen := p.petSightings.Add(1)
					sendList := func() {
						if p.sendPetItemList(inv, seen) {
							p.describeState(o, o.CastControl())
						}
					}
					if onQueue {
						sendList()
					} else {
						p.Queue().Post(sendList)
					}
					return
				}
				p.describeState(o, o.CastControl())
			}
			return
		}
		if snap, ok := summonInfoSnapshot(o, p, p.npcs, p.link.summonInCombat(o)); ok {
			p.sendVisibilityFrame(serverpackets.FrameNPCInfo(snap))
			p.describeState(o, o.CastControl())
		}
	case groundItemObject:
		if dropperID := o.DropperID(); dropperID != 0 {
			p.sendVisibilityFrame(serverpackets.FrameDropItem(o, dropperID))
			return
		}
		p.sendVisibilityFrame(serverpackets.FrameSpawnItem(o))
	case doorObject:
		p.sendVisibilityFrame(serverpackets.FrameDoorInfo(o, false))
		p.sendVisibilityFrame(serverpackets.FrameDoorStatusUpdate(o, false))
	case staticObject:
		p.sendVisibilityFrame(serverpackets.FrameStaticObjectInfo(o))
	case *boat.Boat:
		// A boat shows only the leg it sails: it never casts.
		p.sendVisibilityFrame(vehicleInfoFrame(o))
		if dest, speed, rotation, ok := o.Departure(); ok {
			p.sendVisibilityFrame(serverpackets.FrameVehicleDeparture(o.ObjectID(), speed, rotation, dest))
		}
	}
}

// discoveredCreature is a creature a player can come to know while it walks.
type discoveredCreature interface {
	ObjectID() int32
	Position() (x, y, z int)
	MovingTo() (location.Location, bool)
}

// castInFlight is the cast controller surface describeState reads.
type castInFlight interface {
	InFlight() (modelskill.Definition, actorcast.Target, bool)
}

// Every production cast controller is an *actorcast.Controller; pin that it
// keeps the surface describeState asserts at run time.
var _ castInFlight = (*actorcast.Controller)(nil)

// describeState shows p, right after c's info packet, what c is doing: the
// leg it walks, from where it stands, or else the cast it has in flight,
// with the skill's own hit time and reuse delay. cast is c's cast
// controller, nil when it has none. A creature doing neither shows nothing
// more.
func (p *livePlayer) describeState(c discoveredCreature, cast any) {
	x, y, z := c.Position()
	at := location.Location{X: x, Y: y, Z: z}
	if dest, ok := c.MovingTo(); ok {
		p.sendVisibilityFrame(serverpackets.FrameMoveToLocation(c.ObjectID(), dest, at))
		return
	}
	ctl, ok := cast.(castInFlight)
	if !ok {
		return
	}
	def, target, ok := ctl.InFlight()
	if !ok || target == nil {
		return
	}
	p.sendVisibilityFrame(serverpackets.FrameMagicSkillUse(
		serverpackets.SkillCastObject{ObjectID: c.ObjectID(), Location: at},
		skillCastObject(target),
		int32(def.ID), int32(def.Level), def.HitTime, def.ReuseDelay, false,
	))
}

// sendPetItemList sends the owner's full pet inventory, draining the pending
// PetInventoryUpdate queue it supersedes. It runs on p's queue, and sends
// nothing once p has forgotten the pet (unsummoned or out of view) or seen it
// again since the Discover numbered seen; it reports whether p still sees the
// pet as that Discover did.
func (p *livePlayer) sendPetItemList(inv *itemcontainer.Inventory, seen uint32) bool {
	sim.AssertOwner(p.Queue())
	if p.petSightings.Load() != seen {
		return false
	}
	var frame wire.Frame
	err := inv.BuildAndDrainUpdates(func(items []*item.Instance) error {
		var buildErr error
		frame, buildErr = serverpackets.FramePetItemList(items, inv.Templates())
		return buildErr
	})
	if err != nil {
		p.log.Error().Err(err).Msg("build PetItemList")
		return true
	}
	p.sendVisibilityFrame(frame)
	return true
}

// liveSummonOwner returns the connected player controlling a.
func liveSummonOwner(a *summon.Actor) (*livePlayer, bool) {
	owner, _ := a.Owner()
	live, ok := owner.(*livePlayer)
	return live, ok
}

// sendSummonInfosToOwner republishes the owner-only pet window, then the
// summon's effect icons, which PetInfo clears on the client.
func sendSummonInfosToOwner(a *summon.Actor) {
	if a == nil {
		return
	}
	owner, ok := liveSummonOwner(a)
	if !ok {
		return
	}
	if snap, ok := petInfoSnapshot(a, owner, owner.npcs); ok {
		owner.sendVisibilityFrame(serverpackets.FramePetInfo(snap))
		refreshSummonPartySpelled(a)
	}
}

func (l *GameClientLink) refreshSummonAbnormalEffect(a *summon.Actor) {
	sendSummonInfosToOwner(a)
	if l.world == nil {
		return
	}
	l.world.ForEachKnown(a, func(obj world.Tracked) {
		p, ok := obj.(*livePlayer)
		if !ok || a.ShownAsOwnedBy(p.ObjectID()) {
			return
		}
		if snap, ok := summonInfoSnapshot(a, p, p.npcs, l.summonInCombat(a)); ok {
			p.sendVisibilityFrame(serverpackets.FrameNPCInfo(snap))
		}
	})
}

func (p *livePlayer) Forget(obj world.Tracked) {
	if o, ok := obj.(*summon.Actor); ok {
		if o.ShownAsOwnedBy(p.ObjectID()) {
			p.petSightings.Add(1)
		}
	}
	p.forgetTarget(obj)
	if !rendersObject(obj) {
		return
	}
	seated := false
	if other, ok := obj.(*livePlayer); ok {
		seated = other.seated()
	}
	p.sendVisibilityFrame(serverpackets.FrameDeleteObject(obj.ObjectID(), seated))
}

// forgetTarget clears a selection that left the known list, with the same
// answer as a cancelled selection. It runs before the object's DeleteObject;
// an unsummoned owner's selection clears after its explicit PetDelete.
func (p *livePlayer) forgetTarget(obj world.Tracked) {
	if p.link != nil && p.ClearTargetIf(obj) {
		p.link.announceTargetCleared(p, obj)
	}
}

type groundItemObject interface {
	ObjectID() int32
	ItemID() int32
	Count() int
	Stackable() bool
	Position() (int, int, int)
	// DropperID is the object that dropped the item, or 0.
	DropperID() int32
}

type doorObject interface {
	ObjectID() int32
	DoorID() int
	Opened() bool
	MaxHP() int
	HP() int
	Damage() int
}

type staticObject interface {
	ObjectID() int32
	StaticObjectID() int
}

func rendersObject(obj world.Tracked) bool {
	switch obj.(type) {
	case *livePlayer, *npc.Hostile, *npc.Decoration, *npc.Folk, *summon.Actor, groundItemObject, doorObject, staticObject, *boat.Boat:
		return true
	default:
		return false
	}
}

// summonInCombat reports whether a is drawn in its combat pose: while its
// owner holds an attack stance, or, for a summon revived after its owner left
// the world, while it holds the stance of its own that
// startSummonAttackStance keeps for it.
func (l *GameClientLink) summonInCombat(a *summon.Actor) bool {
	if !a.OwnerLeft() {
		return a.InCombat()
	}
	return l != nil && l.attackStance != nil && l.attackStance.InAttackStance(a)
}

// summonInfoSnapshot resolves the NpcInfo fields viewer sees for a summon it
// does not own. Attackable is per viewer: whether viewer may attack a without
// forcing, which follows a's owner's karma and PvP flag. A nil viewer sees it
// as not attackable. inCombat is summonInCombat's answer for a. The summon
// of an invisible player is not shown: it reports false.
func summonInfoSnapshot(a *summon.Actor, viewer *livePlayer, npcs *npc.Table, inCombat bool) (serverpackets.NPCInfoSnapshot, bool) {
	if npcs == nil {
		return serverpackets.NPCInfoSnapshot{}, false
	}
	tmpl, ok := npcs.Get(a.NPCID())
	if !ok {
		return serverpackets.NPCInfoSnapshot{}, false
	}
	x, y, z := a.Position()
	title, pvpFlag, karma, team := "", 0, 0, 0
	if owner, ok := liveSummonOwner(a); ok {
		// No one but its owner is shown an invisible player's summon.
		if owner.Invisible() && (viewer == nil || viewer.ObjectID() != owner.ObjectID()) {
			return serverpackets.NPCInfoSnapshot{}, false
		}
		title = owner.Name
		pvpFlag = int(owner.PvPFlagState())
		karma = owner.Karma()
		team = owner.DuelTeam()
	}
	pAtkSpd := int(a.PAtkSpd(tmpl.AtkSpd))
	return serverpackets.NPCInfoSnapshot{
		ObjectID: a.ObjectID(), TemplateID: tmpl.TemplateID,
		X: x, Y: y, Z: z, Heading: a.Heading(),
		MAtkSpd: int(a.MAtkSpd()), PAtkSpd: pAtkSpd,
		RunSpd: int(tmpl.RunSpeed), WalkSpd: int(tmpl.WalkSpeed),
		MoveMultiplier:   float64(a.MovementSpeedMultiplier(tmpl.RunSpeed)),
		AtkSpdMultiplier: npcinfo.AttackSpeedMultiplier(pAtkSpd, tmpl.AtkSpd),
		CollisionRadius:  a.CollisionRadius(), CollisionHeight: tmpl.CollisionHeight,
		Running: true, InCombat: inCombat, AlikeDead: a.AlikeDead(),
		RightHand: tmpl.RightHand, LeftHand: tmpl.LeftHand,
		Name: a.Name(), Title: title, Summon: true, PvpFlag: pvpFlag, Karma: karma,
		AbnormalEffect: a.AbnormalEffect(), Team: team,
		Attackable: viewer != nil && a.AttackableWithoutForceBy(viewer.Character),
	}, true
}

// petInfoSnapshot resolves a's owner-visible PetInfo fields, given owner
// (a's confirmed owner) and npcs to look up a's template. It returns
// (zero, false) if the template is missing: an unresolvable summon is a
// silent no-op.
func petInfoSnapshot(a *summon.Actor, owner *livePlayer, npcs *npc.Table) (serverpackets.PetInfoSnapshot, bool) {
	if npcs == nil {
		return serverpackets.PetInfoSnapshot{}, false
	}
	tmpl, ok := npcs.Get(a.NPCID())
	if !ok {
		return serverpackets.PetInfoSnapshot{}, false
	}
	x, y, z := a.Position()

	curFed, maxFed := 0, 0
	var expForThisLevel, expForNextLevel int64
	totalWeight, weightLimit := 0, 0
	// A pet's soulshots/spiritshots per hit come from the per-level
	// pet-data row, not the npc template's base value that servitors use —
	// those two can differ (e.g. Wolf
	// 12077: template ssCount=2, level-row ssCount=1).
	ssCount, spsCount := tmpl.SSCount, tmpl.SPSCount
	if a.IsPet() {
		ssCount, spsCount = a.SSCount(), a.SPSCount()
		curFed, maxFed = a.Fed(), 0
		if tmpl.Pet != nil {
			if row, ok := tmpl.Pet.Levels[a.Level()]; ok {
				maxFed = row.MaxMeal
				expForThisLevel = row.MaxExp
			}
			if row, ok := tmpl.Pet.Levels[a.Level()+1]; ok {
				expForNextLevel = row.MaxExp
			}
		}
		if inv := a.PetInventory(); inv != nil {
			totalWeight = inv.TotalWeight()
		}
		weightLimit = a.WeightLimit()
	} else {
		lifetime := a.Lifetime()
		curFed, maxFed = lifetime.TimeRemaining, lifetime.TotalLifeTime
	}

	pAtkSpd := int(a.PAtkSpd(tmpl.AtkSpd))
	return serverpackets.PetInfoSnapshot{
		SummonType:        a.SummonType(),
		ObjectID:          a.ObjectID(),
		TemplateID:        a.NPCID(),
		X:                 x,
		Y:                 y,
		Z:                 z,
		Heading:           a.Heading(),
		MAtkSpd:           int(a.MAtkSpd()),
		PAtkSpd:           pAtkSpd,
		RunSpd:            int(tmpl.RunSpeed),
		WalkSpd:           int(tmpl.WalkSpeed),
		MoveMultiplier:    float64(a.MovementSpeedMultiplier(tmpl.RunSpeed)),
		AtkSpdMultiplier:  npcinfo.AttackSpeedMultiplier(pAtkSpd, tmpl.AtkSpd),
		CollisionRadius:   tmpl.CollisionRadius,
		CollisionHeight:   tmpl.CollisionHeight,
		InCombat:          owner.InCombat(),
		AlikeDead:         a.AlikeDead(),
		Name:              a.Name(),
		Title:             tmpl.Title,
		PvpFlag:           int(owner.PvPFlagState()),
		Karma:             owner.Karma(),
		CurFed:            curFed,
		MaxFed:            maxFed,
		CurHP:             int(a.HP()),
		MaxHP:             int(a.MaxHPValue()),
		CurMP:             int(a.MPValue()),
		MaxMP:             int(a.MaxMPValue()),
		Level:             a.Level(),
		Exp:               a.Exp(),
		ExpForThisLevel:   expForThisLevel,
		ExpForNextLevel:   expForNextLevel,
		SP:                a.SP(),
		TotalWeight:       totalWeight,
		WeightLimit:       weightLimit,
		PAtk:              int(a.PAtk()),
		PDef:              int(a.PDef()),
		MAtk:              int(a.MAtk()),
		MDef:              int(a.MDef()),
		Accuracy:          int(a.Accuracy()),
		EvasionRate:       int(a.EvasionRate()),
		CriticalHit:       int(a.CriticalRate(tmpl.CritRate)),
		MoveSpeed:         int(a.MoveSpeed(tmpl.RunSpeed)),
		AbnormalEffect:    petAbnormalEffect(a, owner),
		Mountable:         petmodel.IsMountable(a.NPCID()),
		Team:              owner.DuelTeam(),
		SoulShotsPerHit:   ssCount,
		SpiritShotsPerHit: spsCount,
	}, true
}
