package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func (p *livePlayer) Discover(obj world.Tracked) {
	switch o := obj.(type) {
	case *livePlayer:
		p.sendVisibilityFrame(serverpackets.FrameCharInfo(serverpackets.CharInfoSnapshot{
			Character: o.Character,
			Template:  o.template,
			Items:     o.inventoryItems(),
		}))
		if o.throne != nil {
			p.sendVisibilityFrame(serverpackets.FrameChairSit(o.ObjectID(), o.throne.StaticObjectID()))
		}
	case *npc.Hostile:
		p.sendVisibilityFrame(serverpackets.FrameNPCInfo(o.NPCInfoSnapshot()))
	case *npc.Decoration:
		p.sendVisibilityFrame(serverpackets.FrameNPCInfo(o.NPCInfoSnapshot()))
	case *summon.Actor:
		if o.OwnerID() == p.ObjectID() {
			o.MarkDiscoveredByOwner()
			if snap, ok := petInfoSnapshot(o, p, p.npcs); ok {
				p.sendVisibilityFrame(serverpackets.FramePetInfo(snap))
				if inv := o.PetInventory(); inv != nil {
					// PetInventoryUpdate is drained and sent on p's queue, but
					// discovery can run on another actor's: a summon-friend cast
					// teleports p from the caster's queue. Building the snapshot
					// there would let an update drained after it overtake it, so
					// the snapshot is always taken and sent on p's queue.
					seen := p.petSightings.Add(1)
					p.Queue().Post(func() { p.sendPetItemList(inv, seen) })
				}
			}
			return
		}
		if snap, ok := summonInfoSnapshot(o, p.npcs); ok {
			p.sendVisibilityFrame(serverpackets.FrameNPCInfo(snap))
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
	}
}

// sendPetItemList sends the owner's full pet inventory, draining the pending
// PetInventoryUpdate queue it supersedes. It runs on p's queue, and sends
// nothing once p has forgotten the pet (unsummoned or out of view) or seen it
// again since the Discover numbered seen.
func (p *livePlayer) sendPetItemList(inv *itemcontainer.Inventory, seen uint32) {
	sim.AssertOwner(p.Queue())
	if p.petSightings.Load() != seen {
		return
	}
	var frame wire.Frame
	err := inv.BuildAndDrainUpdates(func(items []*item.Instance) error {
		var buildErr error
		frame, buildErr = serverpackets.FramePetItemList(items, inv.Templates())
		return buildErr
	})
	if err != nil {
		p.log.Error().Err(err).Msg("build PetItemList")
		return
	}
	p.sendVisibilityFrame(frame)
}

// liveSummonOwner returns the connected player controlling a.
func liveSummonOwner(a *summon.Actor) (*livePlayer, bool) {
	owner, _ := a.Owner()
	live, ok := owner.(*livePlayer)
	return live, ok
}

// sendSummonInfosToOwner republishes the owner-only pet window. PetInfo
// wipes PartySpelled icons; re-push is deferred with #1268 — that issue's
// notifyAbnormalUpdate wiring does not cover this trigger.
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
	}
}

func (l *GameClientLink) refreshSummonAbnormalEffect(a *summon.Actor) {
	sendSummonInfosToOwner(a)
	if l.world == nil {
		return
	}
	l.world.ForEachKnown(a, func(obj world.Tracked) {
		p, ok := obj.(*livePlayer)
		if !ok || p.ObjectID() == a.OwnerID() {
			return
		}
		if snap, ok := summonInfoSnapshot(a, p.npcs); ok {
			p.sendVisibilityFrame(serverpackets.FrameNPCInfo(snap))
		}
	})
}

func (p *livePlayer) Forget(obj world.Tracked) {
	// A selection that leaves the known list is cleared before the object's
	// removal frame, with the same answer as a cancelled selection.
	if p.link != nil && p.ClearTargetIf(obj) {
		p.link.announceTargetCleared(p, obj)
	}
	if o, ok := obj.(*summon.Actor); ok {
		// A summon's removal signal to its owner is always PetDelete
		// (Summon.java's doUnsummon sends it unconditionally before
		// decayMe(), regardless of why the summon is leaving), not the
		// generic DeleteObject other Tracked kinds get. Non-owners received
		// SummonInfo, so they receive the corresponding DeleteObject below.
		if o.OwnerID() == p.ObjectID() {
			p.petSightings.Add(1)
			p.sendVisibilityFrame(serverpackets.FramePetDelete(o.SummonType(), o.ObjectID()))
			return
		}
	}
	if !rendersObject(obj) {
		return
	}
	seated := false
	if other, ok := obj.(*livePlayer); ok {
		seated = other.seated()
	}
	p.sendVisibilityFrame(serverpackets.FrameDeleteObject(obj.ObjectID(), seated))
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
	case *livePlayer, *npc.Hostile, *npc.Decoration, *summon.Actor, groundItemObject, doorObject, staticObject:
		return true
	default:
		return false
	}
}

func summonInfoSnapshot(a *summon.Actor, npcs *npc.Table) (serverpackets.NPCInfoSnapshot, bool) {
	if npcs == nil {
		return serverpackets.NPCInfoSnapshot{}, false
	}
	tmpl, ok := npcs.Get(a.NPCID())
	if !ok {
		return serverpackets.NPCInfoSnapshot{}, false
	}
	x, y, z := a.Position()
	title, pvpFlag, karma := "", 0, 0
	if owner, ok := liveSummonOwner(a); ok {
		title = owner.Name
		pvpFlag = int(owner.PvPFlagState())
		karma = owner.Karma()
	}
	return serverpackets.NPCInfoSnapshot{
		ObjectID: a.ObjectID(), TemplateID: tmpl.TemplateID,
		X: x, Y: y, Z: z, Heading: a.Heading(),
		MAtkSpd: int(a.MAtkSpd()), PAtkSpd: int(a.PAtkSpd(tmpl.AtkSpd)),
		RunSpd: int(tmpl.RunSpeed), WalkSpd: int(tmpl.WalkSpeed),
		CollisionRadius: a.CollisionRadius(), CollisionHeight: tmpl.CollisionHeight,
		Running: true, AlikeDead: a.AlikeDead(),
		RightHand: tmpl.RightHand, LeftHand: tmpl.LeftHand,
		Name: a.Name(), Title: title, Summon: true, PvpFlag: pvpFlag, Karma: karma,
		AbnormalEffect: a.AbnormalEffect(),
	}, true
}

// petInfoSnapshot resolves a's owner-visible PetInfo fields, given owner
// (a's confirmed owner) and npcs to look up a's template. It returns
// (zero, false) if the template is missing, matching Java's silent
// no-op for an unresolvable summon.
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
	// Pet.getSoulShotsPerHit/getSpiritShotsPerHit (Pet.java:396-405) use the
	// per-level pet-data row, not the npc template's base Summon.java
	// (506-514) value that servitors use — those two can differ (e.g. Wolf
	// 12077: template ssCount=2, level-row ssCount=1).
	ssCount, spsCount := tmpl.SSCount, tmpl.SPSCount
	if a.IsPet() {
		ssCount, spsCount = a.SSCount(), a.SPSCount()
		curFed, maxFed = a.Fed(), 0
		if tmpl.Pet != nil {
			if row, ok := tmpl.Pet.Levels[a.Level()]; ok {
				maxFed = row.MaxMeal
			}
			if row, ok := tmpl.Pet.Levels[a.Level()]; ok {
				expForThisLevel = row.MaxExp
			}
			if row, ok := tmpl.Pet.Levels[a.Level()+1]; ok {
				expForNextLevel = row.MaxExp
			}
		}
		if inv := a.PetInventory(); inv != nil {
			totalWeight = inv.TotalWeight()
			weightLimit = inv.WeightLimit
		}
	} else {
		lifetime := a.Lifetime()
		curFed, maxFed = lifetime.TimeRemaining, lifetime.TotalLifeTime
	}

	return serverpackets.PetInfoSnapshot{
		SummonType:        a.SummonType(),
		ObjectID:          a.ObjectID(),
		TemplateID:        a.NPCID(),
		X:                 x,
		Y:                 y,
		Z:                 z,
		Heading:           a.Heading(),
		MAtkSpd:           int(a.MAtkSpd()),
		PAtkSpd:           int(a.PAtkSpd(tmpl.AtkSpd)),
		RunSpd:            int(tmpl.RunSpeed),
		WalkSpd:           int(tmpl.WalkSpeed),
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
		AbnormalEffect:    a.AbnormalEffect(),
		Mountable:         petmodel.IsMountable(a.NPCID()),
		SoulShotsPerHit:   ssCount,
		SpiritShotsPerHit: spsCount,
	}, true
}
