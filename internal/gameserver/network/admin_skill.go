package network

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// adminSkillPageLimit is how many skills a page of //skill's lists shows.
const adminSkillPageLimit = 12

// adminSkill answers //skill on the selected player, gm itself without
// one:
//
//   - //skill [page] opens the player's skill list on that page;
//   - //skill list [page] opens the server's skill table instead, and only
//     that;
//   - //skill set all, //skill set <id> <level> and //skill remove all|<id>
//     change the player's skills, then open its skill list on the page
//     that follows them.
//
// A malformed set or remove answers its usage instead of the change. A
// page number past the int range ends the command where it is read, after
// any change already made.
func (l *GameClientLink) adminSkill(gm *livePlayer, line string) {
	target := adminTargetPlayer(gm, true)
	args := handleradmin.Args(line)
	next := func() (string, bool) {
		if len(args) == 0 {
			return "", false
		}
		tok := args[0]
		args = args[1:]
		return tok, true
	}
	// readPage takes the next token as the page number when it is one, and
	// reports false when that number is past the int range.
	page := 1
	readPage := func() bool {
		tok, ok := next()
		if !ok || !isDigits(tok) {
			return true
		}
		n, ok := parseJavaInt(tok)
		page = int(n)
		return ok
	}

	if param, ok := next(); ok {
		switch param {
		case "list":
			if readPage() {
				onPlayer(gm, target, func() { l.showAdminSkillTable(gm, target, page) })
			}
			return
		case "set":
			if !l.adminSkillSet(gm, target, next) || !readPage() {
				return
			}
		case "remove":
			if !l.adminSkillRemove(gm, target, next) || !readPage() {
				return
			}
		default:
			if isDigits(param) {
				n, ok := parseJavaInt(param)
				if !ok {
					return
				}
				page = int(n)
			} else if !readPage() {
				return
			}
		}
	}
	onPlayer(gm, target, func() { l.showAdminSkills(gm, target, page) })
}

// adminSkillSet runs //skill set: "all" grants target every skill its level
// makes available; <id> <level> gives it that one skill, stored. It
// reports whether the command goes on to the skill page: a well-formed id
// and level naming no skill ends it with the usage alone.
func (l *GameClientLink) adminSkillSet(gm, target *livePlayer, next func() (string, bool)) bool {
	const usage = "Usage: //skill set id level [page]"
	param, ok := next()
	if !ok {
		sendText(gm, usage)
		return true
	}
	if param == "all" {
		return onPlayer(gm, target, func() { l.rewardAdminSkills(gm, target) })
	}
	id, ok := parseJavaInt(param)
	if !ok {
		sendText(gm, usage)
		return true
	}
	levelParam, _ := next()
	level, ok := parseJavaInt(levelParam)
	if !ok {
		sendText(gm, usage)
		return true
	}
	def, ok := l.skills.KeyedDefinition(id, level)
	if !ok {
		sendText(gm, usage)
		return false
	}
	return onPlayer(gm, target, func() { l.giveAdminSkill(gm, target, def) })
}

// adminSkillRemove runs //skill remove: "all" takes every skill away from
// target, <id> that one skill. It reports whether the command goes on to
// the skill page.
func (l *GameClientLink) adminSkillRemove(gm, target *livePlayer, next func() (string, bool)) bool {
	const usage = "Usage: //skill remove id [page]"
	param, ok := next()
	if !ok {
		sendText(gm, usage)
		return true
	}
	if param == "all" {
		return onPlayer(gm, target, func() { l.removeAdminSkills(gm, target) })
	}
	id, ok := parseJavaInt(param)
	if !ok {
		sendText(gm, usage)
		return true
	}
	return onPlayer(gm, target, func() {
		l.removeLiveSkill(target, int(id), true)
		sendText(gm, "You removed "+strconv.Itoa(int(id))+" skillId from "+target.Name+".")
	})
}

// rewardAdminSkills runs //skill set all on target's queue: target is
// granted every skill its level makes available, the bought ones stored, a
// shortcut to a skill that went up follows its new level, and target gets
// its new skill list. gm is told.
func (l *GameClientLink) rewardAdminSkills(gm, target *livePlayer) {
	if l.skills != nil {
		before := target.SkillLevels()
		if err := l.skills.RewardSkills(target.Character, target.Template()); err != nil {
			l.log.Error().Err(err).Int32("object_id", target.ObjectID()).Msg("admin: reward skills")
		}
		l.sendSkillChanges(target, before, raisedSkill)
	}
	sendText(gm, "You gave all available skills to "+target.Name+".")
}

// giveAdminSkill runs //skill set <id> <level> on target's queue: target
// learns def, stored, unless it already knows that very level. A changed
// skill drops the skill its previous level brought along and re-points its
// shortcuts at the new level; target gets its skill list either way, and
// gm is told.
func (l *GameClientLink) giveAdminSkill(gm, target *livePlayer, def modelskill.Definition) {
	before := target.SkillLevels()
	id := int(def.ID)
	if old := before[id]; old != def.Level {
		if err := l.skills.SetKnownSkill(target.Character, id, def.Level); err != nil {
			l.log.Error().Err(err).Int32("object_id", target.ObjectID()).Int("skill_id", id).Msg("admin: give skill")
		}
		if prev, ok := l.skills.Definition(modelskill.Ref{ID: def.ID, Level: old}); ok && old > 0 && prev.TriggeredID > 1 {
			l.removeLiveSkill(target, prev.TriggeredID, false)
		}
	}
	l.sendSkillChanges(target, before, changedSkill)
	sendText(gm, "You gave "+def.Name+" skill to "+target.Name+".")
}

// removeAdminSkills runs //skill remove all on target's queue: every skill
// target knows is taken away, stored, then gm is told and target gets its
// now empty skill list.
func (l *GameClientLink) removeAdminSkills(gm, target *livePlayer) {
	levels := target.SkillLevels()
	ids := make([]int, 0, len(levels))
	for id := range levels {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		l.removeLiveSkill(target, id, true)
	}
	sendText(gm, "You removed all skills from "+target.Name+".")
	l.sendSkillChanges(target, nil, nil)
}

// removeLiveSkill takes skillID away from live, on its queue, and reports
// whether live knew it. The skill its level brought along goes with it, a
// cast of it in flight stops, its passive stats and effects end, and with
// store set its row is deleted and, unless it is passive, every shortcut
// bound to it.
func (l *GameClientLink) removeLiveSkill(live *livePlayer, skillID int, store bool) bool {
	level := live.SkillLevel(skillID)
	if level <= 0 {
		return false
	}
	def, _ := l.skills.Definition(modelskill.Ref{ID: modelskill.ID(skillID), Level: level})
	l.skills.ForgetSkill(live.Character, skillID, store)
	if def.TriggeredID > 1 {
		l.removeLiveSkill(live, def.TriggeredID, false)
	}
	if live.cast != nil {
		if cur, ok := live.cast.CurrentSkill(); ok && int(cur.ID) == skillID {
			live.stopCastInFlight()
		}
	}
	live.StopSkillEffectsByID(modelskill.ID(skillID))
	if store && def.Activation != modelskill.ActivationPassive {
		l.deleteTargetShortcuts(live, shortcut.Skill, int32(skillID))
	}
	return true
}

// showAdminSkills opens page of target's skill list on gm, by skill id,
// each a link that removes it. A page number below 1 opens nothing while
// target knows skills.
func (l *GameClientLink) showAdminSkills(gm, target *livePlayer, page int) {
	levels := target.SkillLevels()
	ids := make([]int, 0, len(levels))
	for id, level := range levels {
		if level > 0 {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	pg, ok := handleradmin.Paginate(len(ids), page, adminSkillPageLimit)
	if !ok {
		l.log.Warn().Int("page", page).Msg("admin: //skill page out of range")
		return
	}
	var b strings.Builder
	for row, id := range ids[pg.Start:pg.End] {
		name := ""
		if def, ok := l.skills.Definition(modelskill.Ref{ID: modelskill.ID(id), Level: levels[id]}); ok {
			name = def.Name
		}
		adminSkillRow(&b, row, id, fmt.Sprintf(`<a action="bypass -h admin_skill remove %d">%s</a>`, id, name), levels[id])
	}
	pg.Space(&b, 20)
	pg.Links(&b, "bypass admin_skill %page%")
	l.sendAdminSkillPage(gm, "char_skills.htm", target.Name, b.String())
}

// showAdminSkillTable opens page of the server's skill table on gm, every
// skill at its highest regular level, by id, under target's name. A page
// number below 1 opens nothing.
func (l *GameClientLink) showAdminSkillTable(gm, target *livePlayer, page int) {
	defs := l.skills.TopLevelDefinitions()
	pg, ok := handleradmin.Paginate(len(defs), page, adminSkillPageLimit)
	if !ok {
		l.log.Warn().Int("page", page).Msg("admin: //skill list page out of range")
		return
	}
	var b strings.Builder
	for row, def := range defs[pg.Start:pg.End] {
		adminSkillRow(&b, row, int(def.ID), def.Name, def.Level)
	}
	pg.Space(&b, 20)
	pg.Links(&b, "bypass admin_skill list %page%")
	l.sendAdminSkillPage(gm, "char_skills_list.htm", target.Name, b.String())
}

// adminSkillRow writes one row of a //skill page: id, label and level, on
// a black band every other row.
func adminSkillRow(b *strings.Builder, row, id int, label string, level int) {
	if row%2 == 0 {
		b.WriteString("<table width=280 bgcolor=000000><tr>")
	} else {
		b.WriteString("<table width=280><tr>")
	}
	fmt.Fprintf(b, `<td width=35>%d</td><td width=220>%s</td><td width=25>%d</td>`, id, label, level)
	b.WriteString(`</tr></table><img src="L2UI.SquareGray" width=280 height=1>`)
}

// sendAdminSkillPage opens the admin page file on gm with name and content
// filled in.
func (l *GameClientLink) sendAdminSkillPage(gm *livePlayer, file, name, content string) {
	html := l.adminHTML(file)
	html = strings.ReplaceAll(html, "%name%", name)
	html = strings.ReplaceAll(html, "%content%", content)
	sendValidatedHTML(gm, 0, html, 0)
}
