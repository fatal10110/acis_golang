package network

import (
	"strconv"
	"strings"

	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// adminStriderNPCID is the strider //ride mounts.
const adminStriderNPCID int32 = 12526

// adminSummonUsage is what //summon answers a command it cannot read.
const adminSummonUsage = "Usage: //summon food|inventory|level>"

// adminRide answers //ride <1|strider|2|wyvern>: gm leaves its mount, or
// else sends its summon away, and mounts a strider or a wyvern no collar
// called. A cursed weapon holder is refused before anything else, then a
// missing or unknown mount is named.
func (l *GameClientLink) adminRide(gm *livePlayer, line string) {
	if gm.Character.CursedWeaponEquipped() {
		sendText(gm, "You can't use //ride owning a Cursed Weapon.")
		return
	}
	// The mount is read from words split at spaces alone.
	args := strings.FieldsFunc(line, func(r rune) bool { return r == ' ' })[1:]
	if len(args) == 0 {
		sendText(gm, "You must enter a parameter for that command.")
		return
	}
	var npcID int32
	switch args[0] {
	case "wyvern", "2":
		npcID = player.WyvernNPCID
	case "strider", "1":
		npcID = adminStriderNPCID
	default:
		sendText(gm, "Parameter '"+args[0]+"' isn't recognized for that command.")
		return
	}
	if gm.Mounted() {
		gm.Character.Dismount()
	} else if s := l.liveSummonOf(gm); s != nil {
		// gm's summon runs on gm's queue, the one this command runs on.
		s.Unsummon()
	}
	l.mountWyvern(gm, npcID, 0)
}

// adminUnride answers //unride: gm leaves its mount. Off a mount the
// dismount is shown all the same. A //unride followed by anything is no
// //unride and does nothing: a typed command leaves no client action
// pending.
func (l *GameClientLink) adminUnride(gm *livePlayer, line string) {
	if line != "admin_unride" {
		return
	}
	if !gm.Character.Dismount() {
		l.broadcastDismount(gm)
	}
}

// adminUnsummon answers //unsummon: the summon of the selected player, or
// of the selected summon's owner, else gm's own, is sent away. Without one
// the target is invalid.
func (l *GameClientLink) adminUnsummon(gm *livePlayer, _ string) {
	owner, s := l.adminSummonTarget(gm)
	if s == nil {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
		return
	}
	onPlayer(gm, owner, s.Unsummon)
}

// adminSummon answers //summon on the pet //unsummon would send away:
//
//	//summon food           its meal filled to its level's most
//	//summon inventory      its inventory shown to gm
//	//summon level <n>      its experience set to where level n starts
//
// A summon that is no pet makes the target invalid; a level its growth
// table has no row for is refused; anything else unreadable gets the usage.
func (l *GameClientLink) adminSummon(gm *livePlayer, line string) {
	owner, s := l.adminSummonTarget(gm)
	if s == nil || !s.IsPet() {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
		return
	}
	args := handleradmin.Args(line)
	if len(args) == 0 {
		sendText(gm, adminSummonUsage)
		return
	}
	switch args[0] {
	case "food":
		onPlayer(gm, owner, func() { s.AddFed(l.petMaxMeal(s)) })
	case "inventory":
		l.sendPetInventoryView(gm, s)
	case "level":
		if len(args) < 2 {
			sendText(gm, adminSummonUsage)
			return
		}
		level, err := strconv.ParseInt(args[1], 10, 32)
		if err != nil {
			sendText(gm, adminSummonUsage)
			return
		}
		exp, ok := l.petLevelExp(s, int(level))
		if !ok {
			sendText(gm, "Invalid level for //summon level.")
			return
		}
		onPlayer(gm, owner, func() { s.SetExp(exp) })
	default:
		sendText(gm, adminSummonUsage)
	}
}

// adminSummonTarget resolves the player whose summon //summon and
// //unsummon act on, and that summon, nil without one: the selected
// player, the selected summon's owner, else gm.
func (l *GameClientLink) adminSummonTarget(gm *livePlayer) (*livePlayer, *summon.Actor) {
	owner := gm
	switch t := gm.Target().(type) {
	case *livePlayer:
		owner = t
	case *summon.Actor:
		live, ok := l.livePlayerByID(t.OwnerID())
		if !ok {
			return nil, nil
		}
		owner = live
	}
	return owner, l.liveSummonOf(owner)
}

// liveSummonOf returns live's summon, nil without one.
func (l *GameClientLink) liveSummonOf(live *livePlayer) *summon.Actor {
	if l.world == nil {
		return nil
	}
	obj, ok := l.world.Summon(live.ObjectID())
	if !ok {
		return nil
	}
	s, _ := obj.(*summon.Actor)
	return s
}

// petLevelExp is the experience level starts at in pet s's growth table,
// false for a level the table has no row for.
func (l *GameClientLink) petLevelExp(s *summon.Actor, level int) (int64, bool) {
	if l.npcs == nil {
		return 0, false
	}
	tmpl, ok := l.npcs.Get(s.NPCID())
	if !ok || tmpl.Pet == nil {
		return 0, false
	}
	row, ok := tmpl.Pet.Levels[level]
	return row.MaxExp, ok
}

// sendPetInventoryView shows gm everything pet s carries. An unnamed pet
// has no name to show.
func (l *GameClientLink) sendPetInventoryView(gm *livePlayer, s *summon.Actor) {
	inv := s.PetInventory()
	if inv == nil {
		return
	}
	name := ""
	if s.IsNamed() {
		name = s.Name()
	}
	frame, err := serverpackets.FrameGMViewItemList(name, s.InventoryLimit(), inv.Items(), inv.Templates())
	if err != nil {
		l.log.Error().Err(err).Msg("build GMViewItemList")
		gm.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	gm.SendFrame(frame)
}
