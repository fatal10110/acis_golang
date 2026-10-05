package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
)

// clanHallManager is the clan hall manager's type.
const clanHallManager InstanceKind = "ClanHallManagerNpc"

// hallBuffCheckMs is how long, in milliseconds, a clan hall manager waits
// between two checks of its own support buff.
const hallBuffCheckMs = 300_000

// ClanHallManager reports whether f manages a clan hall: its dialog runs
// the hall's functions, and its AI buffs itself and answers the players it
// casts support magic on.
func (f *Folk) ClanHallManager() bool { return hostileKind(f.Instance) == clanHallManager }

// ResetSupportBuffCheck has a clan hall manager check its own support buff
// the next time its AI idles, as a change to its hall's support magic
// function does.
func (f *Folk) ResetSupportBuffCheck() { f.cast.buffCheckAt.Store(0) }

// hallManagerIdle is what a clan hall manager's AI does in place of
// idling: once every five minutes, it reports the check of its own support
// buff. It neither stops nor changes stance.
func (f *Folk) hallManagerIdle() {
	now := f.now().UnixMilli()
	if now-f.cast.buffCheckAt.Load() <= hallBuffCheckMs {
		return
	}
	f.cast.buffCheckAt.Store(now)
	f.emit(event.HallManagerBuffCheck{})
}

// supportCast acts on a clan hall manager's cast desire aimed at a player:
// a skill still in reuse does nothing; short of the skill's MP the manager
// only tells the player so; otherwise it acts on the desire as any NPC
// does and tells the player the magic was cast. Either answer carries the
// manager's MP.
func (f *Folk) supportCast(d *ai.Desire) {
	castAI := f.cast.castAI
	if !castAI.CanAttempt(d.FinalTarget, d.Skill) {
		return
	}
	noMana := f.MPValue() < float64(castAI.SkillMP(d.Skill))
	if !noMana {
		f.castOn(d)
	}
	f.emit(event.HallSupportCast{PlayerID: d.FinalTarget.ObjectID(), NoMana: noMana, MP: int(f.MPValue())})
}

// castsSupport reports whether f's desire d is a clan hall manager's
// support magic on a player.
func (f *Folk) castsSupport(d *ai.Desire) bool {
	return d.FinalTarget != nil && d.FinalTarget.Kind() == actor.KindPlayer && f.ClanHallManager()
}
