package network

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// adminManageAction applies one of //cancel, //heal, //kill or //res to the
// creature c for gm, each change on the queue that owns it. It reports
// false when the server cannot yet do to c what the command asks.
type adminManageAction func(l *GameClientLink, gm *livePlayer, c world.Tracked) bool

// adminCancel answers //cancel; see adminManage.
func (l *GameClientLink) adminCancel(gm *livePlayer, line string) {
	l.adminManage(gm, line, (*GameClientLink).cancelCreature)
}

// adminHeal answers //heal; see adminManage.
func (l *GameClientLink) adminHeal(gm *livePlayer, line string) {
	l.adminManage(gm, line, (*GameClientLink).healCreature)
}

// adminKill answers //kill; see adminManage.
func (l *GameClientLink) adminKill(gm *livePlayer, line string) {
	l.adminManage(gm, line, (*GameClientLink).killCreature)
}

// adminRes answers //res; see adminManage.
func (l *GameClientLink) adminRes(gm *livePlayer, line string) {
	l.adminManage(gm, line, (*GameClientLink).reviveCreature)
}

// adminManage runs act on the command's creature and, given a radius, on
// every creature within that radius of it, the GM included.
//
// The command takes "[name [radius]]": a lone all-digit argument is the
// radius, any other lone argument the name; three or more arguments are
// ignored. The creature is the online player so named, else the GM's
// selected creature, else the GM. The GM is told nothing on success.
//
// A radius that is no 32-bit number aborts the command before it acts and
// is only logged, as the client is told nothing about it either: neither
// the chat command nor the link leaves a client action pending.
func (l *GameClientLink) adminManage(gm *livePlayer, line string, act adminManageAction) {
	args := strings.FieldsFunc(line, func(r rune) bool { return r == ' ' })[1:]
	var name string
	radius := 0
	radiusText := ""
	switch len(args) {
	case 2:
		name, radiusText = args[0], args[1]
	case 1:
		if allASCIIDigits(args[0]) {
			radiusText = args[0]
		} else {
			name = args[0]
		}
	}
	if radiusText != "" {
		r, err := strconv.ParseInt(radiusText, 10, 32)
		if err != nil {
			l.log.Warn().Str("player", gm.CharacterName()).Str("command", line).Msg("admin: radius is not a number")
			return
		}
		radius = int(r)
	}

	target := adminTargetCreature(gm)
	if name != "" {
		if p, ok := l.livePlayerByName(name); ok {
			target = p
		}
	}
	targets := []world.Tracked{target}
	if radius > 0 && l.world != nil {
		l.world.ForEachKnownInRadius(target, radius, func(o world.Tracked) {
			if isCreature(o) {
				targets = append(targets, o)
			}
		})
	}
	unsupported := false
	for _, c := range targets {
		if !act(l, gm, c) {
			unsupported = true
			l.log.Warn().Str("command", line).Str("target", trackedName(c)).Msg("admin: command not supported on this creature yet")
		}
	}
	if unsupported {
		gm.SendFrame(serverpackets.FrameActionFailed())
	}
}

// allASCIIDigits reports whether s is one or more ASCII digits.
func allASCIIDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// isCreature reports whether obj is a creature: a player, NPC, summon or
// door.
//
// A boat is left out on purpose: it has no vitals and no death here, so
// //kill over a radius that holds one cannot break it (#4513).
func isCreature(obj world.Tracked) bool {
	switch obj.(type) {
	case *livePlayer, *summon.Actor, *door.Object:
		return true
	}
	return npcInstanceOf(obj) != nil
}

// cancelCreature ends every effect c holds, dead or alive. A door, and a
// decoration, holds none.
func (l *GameClientLink) cancelCreature(gm *livePlayer, c world.Tracked) bool {
	switch t := c.(type) {
	case *livePlayer:
		onPlayer(gm, t, t.StopAllEffects)
	case *npc.Hostile:
		t.Queue().Post(func() { t.EffectListIfBuilt().StopAll() })
	case *npc.Folk:
		t.Queue().Post(func() { t.EffectList().StopAll() })
	case *npc.EffectPoint:
		t.Queue().Post(func() { t.EffectList().StopAll() })
	case *summon.Actor:
		t.Post(t.StopAllEffects)
	}
	return true
}

// healCreature fills a living c's HP and MP, and a player's CP, to their
// maxima.
//
// A decoration or effect point is left as it is: it refuses selection, so
// no player watches its vitals and the fill would show nowhere.
func (l *GameClientLink) healCreature(gm *livePlayer, c world.Tracked) bool {
	switch t := c.(type) {
	case *livePlayer:
		onPlayer(gm, t, t.SetMaxCpHpMp)
	case *npc.Hostile:
		t.Queue().Post(t.SetMaxHpMp)
	case *npc.Folk:
		t.Queue().Post(t.SetMaxHpMp)
	case *summon.Actor:
		t.Post(t.SetMaxHpMp)
	case *door.Object:
		if l.doors != nil {
			l.doors.RestoreDoorHP(t.DoorID())
		}
	}
	return true
}

// killCreature strips a living c other than gm of its effects, then hits it,
// gm the attacker, for its maximum HP and CP and one more: the hit takes the
// usual damage path, so invulnerability and gm's damage permission apply,
// and a death credits gm. A door outside its siege takes no damage, and
// holds no effects.
//
// A decoration or effect point cannot die yet.
func (l *GameClientLink) killCreature(gm *livePlayer, c world.Tracked) bool {
	attacker := gm.Character
	var skill modelskill.Definition
	switch t := c.(type) {
	case *livePlayer:
		if t == gm {
			return true
		}
		onPlayer(gm, t, func() {
			if t.Dead() {
				return
			}
			t.StopAllEffects()
			res := t.ResourceValues()
			t.ReduceHP(res.MaxHP+res.MaxCP+1, attacker, skill)
		})
	case *npc.Hostile:
		t.Queue().Post(func() {
			if t.Dead() {
				return
			}
			t.EffectListIfBuilt().StopAll()
			t.ReduceHP(t.MaxHPValue()+1, attacker, skill)
		})
	case *npc.Folk:
		t.Queue().Post(func() {
			if t.Dead() {
				return
			}
			t.EffectList().StopAll()
			t.ReduceHP(t.MaxHPValue()+1, attacker, skill)
		})
	case *summon.Actor:
		t.Post(func() {
			if t.Dead() {
				return
			}
			t.StopAllEffects()
			t.ReduceHP(t.MaxHPValue()+1, attacker, skill)
		})
	case *door.Object:
		if !t.Dead() {
			t.ReduceHP(float64(t.MaxHP())+1, attacker, skill)
		}
	case *npc.Decoration, *npc.EffectPoint:
		return false
	}
	return true
}

// reviveCreature stands a dead c up again: a player regains all the
// experience its death took, a summon keeps its corpse from decaying, and a
// broken door takes its template's open state back with the configured share
// of its HP. A living c is left as it is.
//
// A dead NPC cannot be revived yet. A decoration or effect point never dies.
func (l *GameClientLink) reviveCreature(gm *livePlayer, c world.Tracked) bool {
	switch t := c.(type) {
	case *livePlayer:
		onPlayer(gm, t, func() { t.ReviveRestoringExp(100) })
	case *summon.Actor:
		t.Post(func() {
			if !t.Dead() {
				return
			}
			t.CancelDecay()
			t.Revive()
		})
	case *door.Object:
		if t.Dead() && l.doors != nil {
			l.doors.ReviveDoor(t.DoorID(), l.playerConfig().RespawnRestoreHP)
		}
	case *npc.Hostile:
		return !t.Dead()
	case *npc.Folk:
		return !t.Dead()
	}
	return true
}
