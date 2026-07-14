package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func (p *livePlayer) Discover(obj world.Tracked) {
	switch o := obj.(type) {
	case *livePlayer:
		p.SendFrame(serverpackets.FrameCharInfo(serverpackets.CharInfoSnapshot{
			Character: o.Character,
			Template:  o.template,
			Items:     o.inventoryItems(),
		}))
	case *npc.Hostile:
		p.SendFrame(serverpackets.FrameNPCInfo(npcInfoSnapshot(o)))
	case groundItemObject:
		if dropped, ok := o.(interface{ DropperID() int32 }); ok {
			if dropperID := dropped.DropperID(); dropperID != 0 {
				p.SendFrame(serverpackets.FrameDropItem(o, dropperID))
				return
			}
		}
		p.SendFrame(serverpackets.FrameSpawnItem(o))
	case doorObject:
		p.SendFrame(serverpackets.FrameDoorInfo(o, false))
	case staticObject:
		p.SendFrame(serverpackets.FrameStaticObjectInfo(o))
	}
}

func (p *livePlayer) Forget(obj world.Tracked) {
	if !rendersObject(obj) {
		return
	}
	p.SendFrame(serverpackets.FrameDeleteObject(obj.ObjectID(), false))
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
