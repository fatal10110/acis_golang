package network

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	skillhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// adminEffectPageLimit is how many effects a page of //effect lists.
const adminEffectPageLimit = 14

// adminEffect answers //effect on the selected creature, the game master
// without one:
//
//   - //effect [page] opens the creature's effect list on that page;
//   - //effect set <id> <level> [page] lands that skill's effects on it,
//     the game master their effector, as a landed cast would;
//   - //effect remove <all|id> [page] ends all its effects, or those of one
//     skill, and tells the game master when the creature is another;
//   - //effect visual <id> [page] shows the creature casting that skill, at
//     level 1, on the game master.
//
// Each then opens the list, on the page that follows when one does. A
// malformed set, remove or visual answers its usage instead of the change.
// A well-formed set naming no skill answers the usage and opens nothing, a
// remove of an id below 1 does nothing at all, and a page number past the
// int range ends the command where it is read, after any change. The
// change and the list run on the creature's own queue.
func (l *GameClientLink) adminEffect(gm *livePlayer, line string) {
	target := adminTargetCreature(gm)
	args := handleradmin.Args(line)
	next := func() (string, bool) {
		if len(args) == 0 {
			return "", false
		}
		tok := args[0]
		args = args[1:]
		return tok, true
	}
	page := 1
	var change func()
	run := func(showPage bool) {
		onCreature(gm, target, func() {
			if change != nil {
				change()
			}
			if showPage {
				l.showAdminEffects(gm, target, page)
			}
		})
	}

	param, ok := next()
	if !ok {
		run(true)
		return
	}
	if isDigits(param) {
		n, ok := parseInt32(param)
		if !ok {
			return
		}
		page = int(n)
		run(true)
		return
	}
	switch param {
	case "set":
		def, usage, show := l.adminEffectSkill(next)
		if !show {
			sendText(gm, usage)
			return
		}
		change = func() {
			if def == nil {
				sendText(gm, usage)
				return
			}
			landAdminEffects(gm, target, *def)
		}
	case "remove":
		var show bool
		if change, show = adminEffectRemoval(gm, target, next); !show {
			// An id below 1 ends the command with nothing to tell and no
			// client action pending: a chat line or bypass awaits no reply.
			return
		}
	case "visual":
		tok, _ := next()
		id, ok := parseInt32(tok)
		change = func() {
			if !ok {
				sendText(gm, "Usage: //effect visual id")
				return
			}
			l.showAdminVisual(gm, target, id)
		}
	}
	if tok, ok := next(); ok && isDigits(tok) {
		n, ok := parseInt32(tok)
		if !ok {
			run(false)
			return
		}
		page = int(n)
	}
	run(true)
}

// adminEffectSkill reads the id and level of //effect set and returns the
// skill they name, nil with its usage when they are missing or malformed.
// show is false, with the usage, when they are well formed but name no
// skill: the command then ends without its list.
func (l *GameClientLink) adminEffectSkill(next func() (string, bool)) (def *modelskill.Definition, usage string, show bool) {
	usage = "Usage: //effect set id level [page]"
	idTok, ok := next()
	if !ok {
		return nil, usage, true
	}
	id, ok := parseInt32(idTok)
	if !ok {
		return nil, usage, true
	}
	levelTok, ok := next()
	if !ok {
		return nil, usage, true
	}
	level, ok := parseInt32(levelTok)
	if !ok {
		return nil, usage, true
	}
	def, ok = l.skills.KeyedDefinition(id, level)
	if !ok {
		return nil, usage, false
	}
	return def, usage, true
}

// adminEffectRemoval reads what //effect remove ends and returns the change
// that ends it on target, run on target's queue. show is false for an id
// below 1, which changes nothing and opens no list.
func adminEffectRemoval(gm *livePlayer, target world.Tracked, next func() (string, bool)) (change func(), show bool) {
	usage := func() { sendText(gm, "Usage: //effect remove id [page]") }
	tok, ok := next()
	if !ok {
		return usage, true
	}
	if tok == "all" {
		return func() {
			stopAdminEffects(target)
			if target != world.Tracked(gm) {
				sendText(gm, "You removed all effects from "+creatureName(target)+".")
			}
		}, true
	}
	id, ok := parseInt32(tok)
	if !ok {
		return usage, true
	}
	if id < 1 {
		return nil, false
	}
	return func() {
		adminEffectList(target).StopBySkillID(modelskill.ID(id))
		if target != world.Tracked(gm) {
			sendText(gm, fmt.Sprintf("You removed %d skillId effect from %s.", id, creatureName(target)))
		}
	}, true
}

// adminEffectList is the effect list of a creature //effect acts on, nil
// for one holding none.
func adminEffectList(c world.Tracked) *effect.List {
	switch t := c.(type) {
	case *livePlayer:
		return t.EffectList()
	case *npc.Hostile:
		return t.EffectListIfBuilt()
	case *npc.Folk:
		return t.EffectList()
	case *summon.Actor:
		return t.EffectList()
	}
	return nil
}

// stopAdminEffects ends every effect c holds; a player or summon then shows
// its owner the result once.
func stopAdminEffects(c world.Tracked) {
	switch t := c.(type) {
	case *livePlayer:
		t.StopAllEffects()
	case *summon.Actor:
		t.StopAllEffects()
	default:
		adminEffectList(c).StopAll()
	}
}

// landAdminEffects lands def's effects on target from gm, outside any
// cast, and tells gm of each effect with an icon target resisted. A door
// takes none.
func landAdminEffects(gm *livePlayer, target world.Tracked, def modelskill.Definition) {
	to := effectTaker(target)
	if to == nil {
		return
	}
	resisted := skillhandler.LandEffects(gm.Character, to, def)
	name := creatureName(target)
	for range resisted {
		gm.SendFrame(serverpackets.FrameSystemMessageStringSkillName(serverpackets.SystemMessageS1ResistedYourS2, name, int32(def.ID), int32(def.Level)))
	}
}

// effectTaker is c as //effect set lands effects on it, nil for a creature
// that takes none (a door).
func effectTaker(c world.Tracked) skillhandler.Actor {
	switch t := c.(type) {
	case *livePlayer:
		return t.Character
	case *npc.Hostile:
		return t
	case *npc.Folk:
		return t
	case *summon.Actor:
		return t
	}
	return nil
}

// showAdminVisual shows target casting skill id, level 1, on gm, to every
// player that knows target and to target itself when it is a player.
func (l *GameClientLink) showAdminVisual(gm *livePlayer, target world.Tracked, id int32) {
	caster, ok := target.(actorcast.Target)
	if !ok {
		return
	}
	from, to := skillCastObject(caster), skillCastObject(gm)
	l.broadcastCreatureFrame(target, func() wire.Frame {
		return serverpackets.FrameMagicSkillUse(from, to, id, 1, 1, 0, false)
	})
}

// showAdminEffects opens page of target's effect list on gm, buffs then
// debuffs, each a link that removes its skill's effects, with the seconds
// left in its current period, or "toggle". A page number below 1 opens
// nothing while target holds effects.
func (l *GameClientLink) showAdminEffects(gm *livePlayer, target world.Tracked, page int) {
	effects := adminEffectList(target).All()
	pg, ok := handleradmin.Paginate(len(effects), page, adminEffectPageLimit)
	if !ok {
		l.log.Warn().Int("page", page).Msg("admin: //effect page out of range")
		return
	}
	now := gm.Queue().Now()
	var b strings.Builder
	for row, e := range effects[pg.Start:pg.End] {
		if row%2 == 0 {
			b.WriteString("<table width=280 bgcolor=000000><tr>")
		} else {
			b.WriteString("<table width=280><tr>")
		}
		left := "toggle"
		if !e.Skill.Toggle {
			left = strconv.Itoa(e.PeriodRemaining(now)) + "s"
		}
		fmt.Fprintf(&b, `<td width=220><a action="bypass -h admin_effect remove %d">%s</a></td><td width=60>%s</td>`, e.Skill.ID, e.Skill.Name, left)
		b.WriteString(`</tr></table><img src="L2UI.SquareGray" width=280 height=1>`)
	}
	pg.Space(&b, 20)
	pg.Links(&b, "bypass admin_effect %page%")
	l.sendAdminSkillPage(gm, "char_effects.htm", creatureName(target), b.String())
}
