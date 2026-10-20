package npc

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// The self-casts an NPC shows as it charges a soulshot and a spiritshot.
const (
	shotRechargeSoulSkill   int32 = 2154
	shotRechargeSpiritSkill int32 = 2061
)

// folkShots is a civilian NPC's shot charges: the soulshots and spiritshots
// its spawn has left, from its template's SoulShot and SpiritShot AI
// parameters, and the shots charged now. mu guards them: an attacker's
// formulas read the charges from the attacker's queue.
type folkShots struct {
	mu     sync.Mutex
	soul   int
	spirit int
	mask   int32
}

// SoulshotCharged reports whether a soulshot is charged.
func (f *Folk) SoulshotCharged() bool { return f.shotCharged(item.ShotSoul) }

// SpiritshotCharged reports whether a spiritshot is charged.
func (f *Folk) SpiritshotCharged() bool { return f.shotCharged(item.ShotSpirit) }

func (f *Folk) shotCharged(kind item.ShotKind) bool {
	f.shots.mu.Lock()
	defer f.shots.mu.Unlock()
	return f.shots.mask&kind.Mask() != 0
}

// SetChargedShot charges or spends kind.
func (f *Folk) SetChargedShot(kind item.ShotKind, charged bool) {
	f.shots.mu.Lock()
	defer f.shots.mu.Unlock()
	if charged {
		f.shots.mask |= kind.Mask()
	} else {
		f.shots.mask &^= kind.Mask()
	}
}

// RechargeShots charges a soulshot when physical is set and a spiritshot
// when magic is set, each only while the spawn has one left and none of
// that kind is charged, showing the players around the NPC each charge.
func (f *Folk) RechargeShots(physical, magic bool) {
	var skills []int32
	f.shots.mu.Lock()
	if physical && f.shots.soul > 0 && f.shots.mask&item.ShotSoul.Mask() == 0 {
		f.shots.soul--
		f.shots.mask |= item.ShotSoul.Mask()
		skills = append(skills, shotRechargeSoulSkill)
	}
	if magic && f.shots.spirit > 0 && f.shots.mask&item.ShotSpirit.Mask() == 0 {
		f.shots.spirit--
		f.shots.mask |= item.ShotSpirit.Mask()
		skills = append(skills, shotRechargeSpiritSkill)
	}
	f.shots.mu.Unlock()
	for _, id := range skills {
		f.emit(event.ShotRecharged{SkillID: id, At: f.location()})
	}
}
