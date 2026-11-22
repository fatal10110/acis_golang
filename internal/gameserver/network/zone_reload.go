package network

import (
	"context"
	"errors"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
)

// BossZoneStore keeps who may enter the boss zones while their zones are
// read anew.
type BossZoneStore interface {
	// SaveZones replaces the stored permissions with those of zones.
	SaveZones(ctx context.Context, zones []*zone.Boss) error
}

// reloadZonesTimeout bounds the boss zone save of //reload zone.
const reloadZonesTimeout = 30 * time.Second

// wireZones gives the zones of ix the effects the link carries out for
// them. It runs on a zone set before any creature can stand in it.
func (l *GameClientLink) wireZones(ix *zone.Index) {
	if ix == nil {
		return
	}
	l.wireScriptZones(ix)
	ix.Wire(l.newZoneEffects(ix))
}

// reloadZones reads the zones anew and puts every creature in the new ones:
// each boss zone hands its entry permissions over to its new self, and they
// are saved; the new zones are wired and the sieges moved onto their new
// battlefields; then every holder of the zone index sees the new zones at
// once, and each creature leaves the zones it stood in and enters those
// around it.
func (l *GameClientLink) reloadZones() error {
	if l.reloads.Zones == nil || l.zones == nil {
		return errNoReloadHook
	}
	l.zoneReloadMu.Lock()
	defer l.zoneReloadMu.Unlock()
	next, err := l.reloads.Zones()
	if err != nil {
		return err
	}
	if next == nil {
		return errors.New("zone reload read no zones")
	}
	handOverBossZones(l.zones, next)
	l.saveBossPermissions()
	l.wireZones(next)
	l.sieges.Rebind(zone.OfKind[*zone.Siege](next))
	l.halls.RebindBattlefields(zone.OfKind[*zone.Siege](next))
	old := l.zones.Swap(next)
	l.rebaseCreatures(old)
	return nil
}

// saveBossPermissions stores who may enter the current boss zones. A
// failure is logged and the reload goes on.
func (l *GameClientLink) saveBossPermissions() {
	if l.bossZones == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), reloadZonesTimeout)
	defer cancel()
	if err := l.bossZones.SaveZones(ctx, zone.OfKind[*zone.Boss](l.zones)); err != nil {
		l.log.Error().Err(err).Msg("admin: //reload zone could not save the boss zones")
	}
}

// handOverBossZones hands each boss zone of cur over to the boss zone of
// next with its id (Boss.HandOver): next gets the permissions it holds as
// stored ones, and every one granted or revoked through it from then on, so
// a grant made while the reload saves and swaps, through a zone resolved
// before the swap, reaches the new zone with its entry window.
func handOverBossZones(cur, next *zone.Index) {
	for _, old := range zone.OfKind[*zone.Boss](cur) {
		if k, ok := next.ByID(old.ID()); ok {
			if b, ok := k.(*zone.Boss); ok {
				old.HandOver(b)
			}
		}
	}
}

// zoneReloader is a creature whose zone membership a reload moves.
type zoneReloader interface {
	ReloadZones()
}

// rebaseCreatures moves every creature in the world off the zones of old,
// the swapped-out index, and into the zones now around it. Each creature is
// moved under its own zone lock, so a revalidation that read old before
// the swap has finished by then, and one that reads the new zones moves it
// first; old is cleared last of whatever such a revalidation entered
// meanwhile.
func (l *GameClientLink) rebaseCreatures(old *zone.Index) {
	if l.world != nil {
		for _, obj := range l.world.Objects() {
			if r, ok := obj.(zoneReloader); ok {
				r.ReloadZones()
			}
		}
	}
	old.Clear()
}

// ReloadZones moves the player off the zones a reload swapped out and into
// the reloaded zones of the region holding it, sending its compass zone
// when it changed.
func (p *livePlayer) ReloadZones() {
	if p.zoneActor == nil || p.link == nil {
		return
	}
	p.zoneActor.reload(p.link.zones)
}
