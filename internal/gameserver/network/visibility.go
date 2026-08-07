package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
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
	case *npc.Hostile:
		p.sendVisibilityFrame(serverpackets.FrameNPCInfo(npcInfoSnapshot(o)))
	case *summon.Actor:
		// Only the owner gets PetInfo; a non-owner observer would get
		// SummonInfo instead, which isn't ported yet (tracked separately —
		// see this PR's linked follow-up), so it silently sees nothing.
		if o.OwnerID() == p.ObjectID() {
			if snap, ok := petInfoSnapshot(o, p, p.npcs); ok {
				p.sendVisibilityFrame(serverpackets.FramePetInfo(snap))
			}
		}
	case groundItemObject:
		if dropped, ok := o.(interface{ DropperID() int32 }); ok {
			if dropperID := dropped.DropperID(); dropperID != 0 {
				p.sendVisibilityFrame(serverpackets.FrameDropItem(o, dropperID))
				return
			}
		}
		p.sendVisibilityFrame(serverpackets.FrameSpawnItem(o))
	case doorObject:
		p.sendVisibilityFrame(serverpackets.FrameDoorInfo(o, false))
	case staticObject:
		p.sendVisibilityFrame(serverpackets.FrameStaticObjectInfo(o))
	}
}

func (p *livePlayer) Forget(obj world.Tracked) {
	if !rendersObject(obj) {
		return
	}
	p.sendVisibilityFrame(serverpackets.FrameDeleteObject(obj.ObjectID(), false))
}

type groundItemObject interface {
	ObjectID() int32
	ItemID() int32
	Count() int
	Stackable() bool
	Position() (int, int, int)
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
	case *livePlayer, *npc.Hostile, groundItemObject, doorObject, staticObject:
		return true
	default:
		return false
	}
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
	if a.IsPet() {
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
		Karma:             owner.Karma(),
		CurFed:            curFed,
		MaxFed:            maxFed,
		CurHP:             int(a.HP()),
		MaxHP:             int(a.MaxHPValue()),
		CurMP:             int(a.MPValue()),
		MaxMP:             int(a.MaxMPValue()),
		Level:             a.Level(),
		Exp:               expForThisLevel,
		ExpForThisLevel:   expForThisLevel,
		ExpForNextLevel:   expForNextLevel,
		TotalWeight:       totalWeight,
		WeightLimit:       weightLimit,
		PAtk:              int(a.PAtk()),
		PDef:              int(a.PDef()),
		MAtk:              int(a.MAtk()),
		MDef:              int(a.MDef()),
		Accuracy:          int(a.Accuracy()),
		EvasionRate:       int(a.EvasionRate()),
		CriticalHit:       int(a.CriticalRate()),
		MoveSpeed:         int(a.MoveSpeed(tmpl.RunSpeed)),
		Mountable:         petmodel.IsMountable(a.NPCID()),
		SoulShotsPerHit:   tmpl.SSCount,
		SpiritShotsPerHit: tmpl.SPSCount,
	}, true
}

func npcInfoSnapshot(n *npc.Hostile) serverpackets.NPCInfoSnapshot {
	tmpl := n.Instance.Template
	x, y, z := n.Position()
	name, title := "", ""
	if tmpl.UsingServerSideName {
		name = tmpl.Name
	}
	if tmpl.UsingServerSideTitle {
		title = tmpl.Title
	}
	return serverpackets.NPCInfoSnapshot{
		ObjectID:        n.ObjectID(),
		TemplateID:      tmpl.TemplateID,
		Attackable:      true,
		X:               x,
		Y:               y,
		Z:               z,
		Heading:         n.Heading(),
		MAtkSpd:         int(tmpl.AtkSpd),
		PAtkSpd:         n.AttackSpeed(),
		RunSpd:          int(tmpl.RunSpeed),
		WalkSpd:         int(tmpl.WalkSpeed),
		CollisionRadius: tmpl.CollisionRadius,
		CollisionHeight: tmpl.CollisionHeight,
		RightHand:       tmpl.RightHand,
		LeftHand:        tmpl.LeftHand,
		Running:         true,
		AlikeDead:       n.AlikeDead(),
		SummonAnimation: 2,
		Name:            name,
		Title:           title,
	}
}
