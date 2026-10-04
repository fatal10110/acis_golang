package network

import (
	"math"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/cursedweapon"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/rs/zerolog"
)

// Cursed weapon presentation: a weapon dropping from a monster turns every
// sky red for cursedDropRedSky seconds and shakes the ground around it;
// one going up a stage plays its holder's cursedRankUpSocial animation.
const (
	cursedDropRedSky         = 10
	cursedDropQuakeIntensity = 14
	cursedDropQuakeDuration  = 3
	cursedDropOffset         = 70
	cursedRankUpSocial       = 17
	// formalWearID is the formal wear a holder may not put on.
	formalWearID = 6408
	// cursedTickPeriod is how often the weapons' timers are checked.
	cursedTickPeriod = time.Second
)

// toAllPlayers sends the frame build makes to every player in the world.
func (l *GameClientLink) toAllPlayers(build func() wire.Frame) {
	if l.world == nil {
		return
	}
	broadcastFrame(build, func(send func(frameReceiver)) {
		for _, p := range l.world.Players() {
			if listener, ok := p.(*livePlayer); ok {
				send(listener)
			}
		}
	})
}

// announceCursedRegion tells every player the region (x, y, z) lies in
// and itemID's name, in system message id.
func (l *GameClientLink) announceCursedRegion(id int, x, y, z int, itemID int32) {
	l.toAllPlayers(func() wire.Frame {
		return serverpackets.FrameSystemMessageParams(id, serverpackets.ZoneNameParam(x, y, z), serverpackets.ItemNameParam(itemID))
	})
}

// DropCursedWeapon implements manager.CursedWeaponDrops: killer's kill of
// the monster dropperID at (x, y, z) may drop a cursed weapon not yet out.
// One that drops lands near the monster, kept from the ground cleanup; every
// sky turns red, the ground around it shakes, and every player hears in
// which region, killer's, it dropped.
func (l *GameClientLink) DropCursedWeapon(killer *player.Character, dropperID int32, x, y, z int) {
	if l == nil || l.cursed == nil || killer == nil || l.groundItems == nil {
		return
	}
	itemID, ok := l.cursed.RollDrop(killer.Roll)
	if !ok {
		return
	}
	ground, ok := l.newCursedGroundItem(itemID)
	if !ok {
		// Nothing reached the world: the weapon goes back on the shelf.
		l.cursed.Expire(itemID)
		return
	}
	tx := x + rnd.GetRange(-cursedDropOffset, cursedDropOffset)
	ty := y + rnd.GetRange(-cursedDropOffset, cursedDropOffset)
	tz := z
	if l.geo != nil {
		at := l.geo.ValidLocation(x, y, z, tx, ty, tz)
		tx, ty, tz = at.X, at.Y, at.Z
	}
	l.groundItems.Drop(ground, task.DropOptions{X: tx, Y: ty, Z: tz, DropperID: dropperID})
	l.cursed.PlaceOnGround(itemID, ground.ObjectID(), location.Location{X: tx, Y: ty, Z: tz})
	l.toAllPlayers(func() wire.Frame { return serverpackets.FrameExRedSky(cursedDropRedSky) })
	l.toAllPlayers(func() wire.Frame {
		return serverpackets.FrameEarthquake(tx, ty, tz, cursedDropQuakeIntensity, cursedDropQuakeDuration, false)
	})
	kx, ky, kz := killer.Position()
	l.announceCursedRegion(serverpackets.SystemMessageS2WasDroppedInTheS1Region, kx, ky, kz, itemID)
}

// newCursedGroundItem creates one itemID as a ground item no cleanup
// takes.
func (l *GameClientLink) newCursedGroundItem(itemID int32) (*grounditem.Item, bool) {
	if l.itemTemplates == nil || l.ids == nil {
		return nil, false
	}
	tmpl, ok := l.itemTemplates.Get(itemID)
	if !ok {
		return nil, false
	}
	objectID, err := l.ids.NextID()
	if err != nil {
		l.log.Error().Err(err).Int32("item_id", itemID).Msg("cursed weapon: allocate object id")
		return nil, false
	}
	ground, err := grounditem.New(item.Instance{ObjectID: objectID, TemplateID: itemID, Count: 1, Location: item.LocationVoid}, tmpl)
	if err != nil {
		l.log.Error().Err(err).Int32("item_id", itemID).Msg("cursed weapon: build ground item")
		return nil, false
	}
	ground.SetDestroyProtected(true)
	return ground, true
}

// obtainCursedWeapon makes live, which was just given itemID, its holder
// when it is a cursed weapon. A player already holding one takes the new
// one in: the held weapon goes up a stage and the new one ends.
//
// A new holder gets off its mount, turns chaotic with its PK kills hidden,
// leaves its party, drops its toggles, gains the weapon's skill, wields
// the weapon and is healed in full; every player then hears in which
// region the weapon's owner appeared.
func (l *GameClientLink) obtainCursedWeapon(live *livePlayer, itemID int32) {
	if l.cursed == nil || !l.cursed.IsCursed(itemID) {
		return
	}
	inv := live.Inventory()
	if inv == nil || l.inventory == nil {
		return
	}
	res, ok := l.cursed.Activate(itemID, cursedweapon.Holder{
		ObjectID:   live.ObjectID(),
		Karma:      int32(live.Karma()),
		PKKills:    int32(live.ProgressionValues().PKKills),
		HeldItemID: live.CursedWeaponID(),
	}, live.Roll)
	if !ok {
		return
	}
	if res.Assimilated {
		if res.RankedUp {
			l.raiseCursedWeaponStage(live, res.HeldItemID, res.Stage)
		}
		inv.DestroyByTemplateID(itemID, 1)
		l.endCursedWeapon(res.End, live)
		return
	}
	if live.Mounted() {
		live.Character.Dismount()
	}
	live.SetCursedWeapon(itemID, res.Stage)
	live.SetKarma(cursedweapon.HolderKarma)
	live.SetPKKills(0)
	if l.parties != nil {
		l.applyPartyNotices(l.parties.Leave(live, party.Expelled))
	}
	live.EffectList().StopAllToggles()
	l.grantCursedSkill(live, itemID, res.Stage)
	if inst := inv.ItemByTemplateID(itemID); inst != nil && !inst.Equipped() {
		if tmpl, ok := inv.Templates().Get(itemID); ok {
			l.toggleEquipItem(live, inv, inst, tmpl, true)
		}
	}
	live.SetMaxCpHpMp()
	l.broadcastCharacterInfo(live)
	x, y, z := live.Position()
	l.announceCursedRegion(serverpackets.SystemMessageOwnerOfS2AppearedInS1Region, x, y, z, itemID)
}

// grantCursedSkill gives live itemID's skill at stage and resends its
// skill list. A stage with no skill level loaded gives nothing.
func (l *GameClientLink) grantCursedSkill(live *livePlayer, itemID, stage int32) {
	if !l.giveCursedSkill(live.Character, itemID, stage) {
		return
	}
	live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
}

// giveCursedSkill gives c itemID's skill at stage, reporting whether that
// skill level is loaded.
func (l *GameClientLink) giveCursedSkill(c *player.Character, itemID, stage int32) bool {
	skillID, ok := l.cursed.Skill(itemID)
	if !ok || l.skills == nil {
		return false
	}
	ref := modelskill.Ref{ID: modelskill.ID(skillID), Level: int(stage)}
	if !l.skills.HasDefinition(ref) {
		return false
	}
	if err := l.skills.GrantTransientSkills(c, []modelskill.Ref{ref}); err != nil {
		l.log.Error().Err(err).Int32("object_id", c.ID).Msg("cursed weapon: give skill")
	}
	return true
}

// removeCursedSkill takes a cursed weapon's skill skillID from live and
// resends its skill list.
func (l *GameClientLink) removeCursedSkill(live *livePlayer, skillID int32) {
	if l.skills == nil {
		return
	}
	l.removeLiveSkill(live, int(skillID), false)
	live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
}

// raiseCursedWeaponStage shows live's itemID going up to stage: the
// skill's next level, and the animation everyone around sees.
func (l *GameClientLink) raiseCursedWeaponStage(live *livePlayer, itemID, stage int32) {
	live.SetCursedWeapon(itemID, stage)
	l.grantCursedSkill(live, itemID, stage)
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameSocialAction(live.ObjectID(), cursedRankUpSocial)
	})
}

// feedCursedWeapon counts live's player kill on the cursed weapon it
// holds: its PK count, shown on its own status, goes up, and the weapon
// may go up a stage.
func (l *GameClientLink) feedCursedWeapon(live *livePlayer) {
	itemID := live.CursedWeaponID()
	if l.cursed == nil || itemID == 0 {
		return
	}
	ranked, stage, ok := l.cursed.Kill(itemID, live.ObjectID(), live.Roll)
	if !ok {
		return
	}
	live.SetPKKills(live.ProgressionValues().PKKills + 1)
	live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
	if ranked {
		l.raiseCursedWeaponStage(live, itemID, stage)
	}
}

// loseCursedWeapon settles the cursed weapon live held when it was
// killed: the weapon ends, or it drops next to live, kept from the ground
// cleanup, and live gets its karma and PK kills back and loses the
// weapon's skill; every player then hears in which region it dropped.
func (l *GameClientLink) loseCursedWeapon(live *livePlayer) {
	itemID := live.CursedWeaponID()
	if l.cursed == nil || itemID == 0 {
		return
	}
	death, ok := l.cursed.HolderDied(itemID, live.Roll)
	if !ok {
		return
	}
	if death.Disappeared {
		l.endCursedWeapon(death.End, live)
		return
	}
	if live.attack != nil {
		live.attack.Stop()
	}
	if inv := live.Inventory(); inv != nil && l.inventory != nil && l.groundItems != nil {
		if inst := inv.ItemByTemplateID(itemID); inst != nil {
			if inst.Equipped() {
				if changed := inv.UnequipItem(inst); len(changed) > 0 {
					l.applyEquipItemStats(live, inv, invops.Result{EquipmentChanged: true, Changed: changed})
				}
			}
			if ground, ok := l.dropHeldItem(live, inv, inst, true); ok {
				gx, gy, gz := ground.Position()
				l.cursed.PlaceOnGround(itemID, ground.ObjectID(), location.Location{X: gx, Y: gy, Z: gz})
			}
		}
	}
	live.SetKarma(int(death.Karma))
	live.SetPKKills(int(death.PKKills))
	live.SetCursedWeapon(0, 0)
	if skillID, ok := l.cursed.Skill(itemID); ok {
		l.removeCursedSkill(live, skillID)
	}
	x, y, z := live.Position()
	l.announceCursedRegion(serverpackets.SystemMessageS2WasDroppedInTheS1Region, x, y, z, itemID)
}

// endCursedWeapon applies a weapon's end, then tells every player it
// disappeared. A holder in the world gets its karma and PK kills back,
// loses the weapon's skill and the weapon itself, and its status is
// refreshed: on the calling queue when it is current, its own otherwise.
// A holder out of the world has that done to its stored state. A weapon on
// the ground leaves the world.
func (l *GameClientLink) endCursedWeapon(end cursedweapon.EndOfLife, current *livePlayer) {
	l.endCursedWeaponThen(end, current, nil)
}

// endCursedWeaponThen is endCursedWeapon running then, when not nil, once
// every player was told the weapon disappeared: on the queue that told
// them.
func (l *GameClientLink) endCursedWeaponThen(end cursedweapon.EndOfLife, current *livePlayer, then func()) {
	announce := func() {
		l.toAllPlayers(func() wire.Frame {
			return serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageS1HasDisappeared, end.ItemID)
		})
		if then != nil {
			then()
		}
	}
	switch {
	case end.Held:
		live, online := l.livePlayerByID(end.HolderID)
		if !online {
			l.cursed.ReleaseHolder(end)
			break
		}
		release := func() {
			l.releaseCursedHolder(live, end)
			announce()
		}
		if live == current || !postLive(live, release) {
			release()
		}
		return
	case end.GroundObjectID != 0:
		l.removeCursedGroundItem(end.GroundObjectID)
	}
	announce()
}

// releaseCursedHolder takes the ended weapon end from live, on live's
// queue: its karma and PK kills come back, the weapon's skill goes, the
// weapon comes off and is destroyed, and its status is refreshed around
// it. The karma and PK kills are stored at once, and the weapon's stored
// row is deleted as well: a release that runs after live's logout already
// released its inventory's persistence and queued its last item save, so
// the destroy above reaches no row, and the delete, queued on the same lane
// after that save, is what takes the weapon out of the stored items.
func (l *GameClientLink) releaseCursedHolder(live *livePlayer, end cursedweapon.EndOfLife) {
	if live.attack != nil {
		live.attack.Stop()
	}
	live.SetKarma(int(end.Karma))
	live.SetPKKills(int(end.PKKills))
	live.SetCursedWeapon(0, 0)
	l.removeCursedSkill(live, end.SkillID)
	if inv := live.Inventory(); inv != nil {
		if inst := inv.ItemByTemplateID(end.ItemID); inst != nil {
			if tmpl, ok := inv.Templates().Get(end.ItemID); ok && inst.Equipped() && l.inventory != nil {
				l.toggleEquipItem(live, inv, inst, tmpl, true)
			}
			inv.DestroyByTemplateID(end.ItemID, 1)
		}
	}
	l.broadcastCharacterInfo(live)
	l.cursed.ReleaseHolder(end)
}

// removeCursedGroundItem takes the ground item objectID out of the world,
// unless a pickup already holds it.
func (l *GameClientLink) removeCursedGroundItem(objectID int32) {
	if l.world == nil {
		return
	}
	obj, ok := l.world.Object(objectID)
	if !ok {
		return
	}
	ground, ok := obj.(*grounditem.Item)
	if !ok || !ground.Claim() {
		return
	}
	if l.groundItems != nil {
		l.groundItems.Remove(ground)
	}
	l.world.Despawn(ground)
}

// restoreCursedWeapon gives c, selected to enter the world, back the
// cursed weapon it holds and the weapon's skill. Nothing is sent: the
// login burst carries both.
func (l *GameClientLink) restoreCursedWeapon(c *player.Character) {
	if l.cursed == nil {
		return
	}
	itemID, stage, ok := l.cursed.Held(c.ID)
	if !ok {
		return
	}
	c.SetCursedWeapon(itemID, stage)
	l.giveCursedSkill(c, itemID, stage)
}

// enterWorldCursedWeapon announces a holder entering the world: every
// player hears in which region the owner of its weapon logged in, and the
// holder how long the weapon has left.
func (l *GameClientLink) enterWorldCursedWeapon(live *livePlayer) {
	itemID := live.CursedWeaponID()
	if l.cursed == nil || itemID == 0 {
		return
	}
	x, y, z := live.Position()
	l.announceCursedRegion(serverpackets.SystemMessageS2OwnerLoggedIntoS1Region, x, y, z, itemID)
	live.SendFrame(cursedTimeLeftFrame(itemID, l.cursed.TimeLeft(itemID)))
}

// cursedTimeLeftFrame tells how long itemID has left: in hours, rounded,
// past an hour; in whole minutes otherwise.
func cursedTimeLeftFrame(itemID int32, left time.Duration) wire.Frame {
	minutes := int32(left.Milliseconds() / 60000)
	if minutes > 60 {
		hours := int32(math.Floor(float64(float32(minutes)/60) + 0.5))
		return serverpackets.FrameSystemMessageItemNameNumber(serverpackets.SystemMessageS2HoursOfUsageTimeLeftForS1, itemID, hours)
	}
	return serverpackets.FrameSystemMessageItemNameNumber(serverpackets.SystemMessageS2MinutesOfUsageTimeLeftForS1, itemID, minutes)
}

// sendCursedWeaponLocations answers RequestCursedWeaponLocation with every
// cursed weapon out in the world: where its holder stands, or where it
// lies. A weapon whose holder is out of the world and that never lay on
// the ground is left out, and with none to list nothing is sent: the
// client's map request leaves no action pending.
func (l *GameClientLink) sendCursedWeaponLocations(live *livePlayer) {
	if l.cursed == nil {
		return
	}
	var entries []serverpackets.CursedWeaponLocation
	for _, st := range l.cursed.Active() {
		entry := serverpackets.CursedWeaponLocation{ItemID: st.ItemID, Active: st.Activated}
		holder, online := l.livePlayerByID(st.HolderID)
		switch {
		case st.Activated && online:
			x, y, z := holder.Position()
			entry.Location = location.Location{X: x, Y: y, Z: z}
		case st.HasGroundAt:
			entry.Location = st.GroundAt
		default:
			continue
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return
	}
	live.SendFrame(serverpackets.FrameExCursedWeaponLocation(entries))
}

// TickCursedWeapons runs the cursed weapons' timers due by now: weapons
// whose time ran out end, and a holder in the world is told every hour
// how long its weapon has left.
func (l *GameClientLink) TickCursedWeapons(now time.Time) {
	if l == nil || l.cursed == nil {
		return
	}
	ends, reminders := l.cursed.Tick(now)
	for _, r := range reminders {
		live, ok := l.livePlayerByID(r.HolderID)
		if !ok {
			continue
		}
		frame := cursedTimeLeftFrame(r.ItemID, r.Left)
		if !postLive(live, func() { live.SendFrame(frame) }) {
			frame.Release()
		}
	}
	for _, end := range ends {
		l.endCursedWeapon(end, nil)
	}
}

// StartCursedWeapons starts the cursed weapons' timer ticks.
func (l *GameClientLink) StartCursedWeapons(log zerolog.Logger) *scheduler.Ticker {
	return scheduler.Start(cursedTickPeriod, func() { l.TickCursedWeapons(time.Now()) }, log)
}

// isCursedWeapon reports whether itemID is a configured cursed weapon.
func (l *GameClientLink) isCursedWeapon(itemID int32) bool {
	if l.cursedWeapons == nil {
		return false
	}
	_, ok := l.cursedWeapons.Weapon(itemID)
	return ok
}
