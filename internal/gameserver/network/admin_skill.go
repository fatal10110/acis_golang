package network

import (
	"fmt"
	"sort"
	"strings"

	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// adminSkillPageLimit is how many skills a page of //skill's list shows.
const adminSkillPageLimit = 12

// adminSkill answers //skill on the selected player, gm itself without
// one: //skill [page] opens the player's skill list on that page, and
// //skill set all [page] first grants it every skill its level makes
// available, as the server's automatic skill learning does.
//
// A page number past the int range ends the command before the list
// opens. //skill list, //skill set <id> <level> and //skill remove are
// not ported: they log the gap and release the client (#3234).
func (l *GameClientLink) adminSkill(gm *livePlayer, line string) {
	target := adminTargetPlayer(gm, true)
	args := handleradmin.Args(line)
	page := 1
	if len(args) > 0 {
		param, rest := args[0], args[1:]
		switch {
		case isDigits(param):
			n, ok := parseJavaInt(param)
			if !ok {
				return
			}
			page = int(n)
		case param == "set" && len(rest) == 0:
			sendText(gm, "Usage: //skill set id level [page]")
		case param == "set" && rest[0] == "all":
			rest = rest[1:]
			if !onPlayer(gm, target, func() { l.rewardAdminSkills(gm, target) }) {
				return
			}
		case param == "list" || param == "set" || param == "remove":
			l.log.Warn().Str("command", "admin_skill "+param).Msg("admin: command not implemented yet")
			gm.SendFrame(serverpackets.FrameActionFailed())
			return
		}
		if !isDigits(param) && len(rest) > 0 && isDigits(rest[0]) {
			n, ok := parseJavaInt(rest[0])
			if !ok {
				return
			}
			page = int(n)
		}
	}
	onPlayer(gm, target, func() { l.showAdminSkills(gm, target, page) })
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
		after := target.SkillLevels()
		ids := make([]int, 0, len(after))
		for id, level := range after {
			if level > before[id] {
				ids = append(ids, id)
			}
		}
		sort.Ints(ids)
		for _, id := range ids {
			l.refreshSkillShortcuts(target, int32(id), int32(after[id]))
		}
		target.SendFrame(serverpackets.FrameSkillList(skillListEntries(target.Character, l.skills)))
	}
	sendText(gm, "You gave all available skills to "+target.Name+".")
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
		if row%2 == 0 {
			b.WriteString("<table width=280 bgcolor=000000><tr>")
		} else {
			b.WriteString("<table width=280><tr>")
		}
		name := ""
		if l.skills != nil {
			if def, ok := l.skills.Definition(modelskill.Ref{ID: modelskill.ID(id), Level: levels[id]}); ok {
				name = def.Name
			}
		}
		fmt.Fprintf(&b, `<td width=35>%d</td><td width=220><a action="bypass -h admin_skill remove %d">%s</a></td><td width=25>%d</td>`, id, id, name, levels[id])
		b.WriteString(`</tr></table><img src="L2UI.SquareGray" width=280 height=1>`)
	}
	pg.Space(&b, 20)
	pg.Links(&b, "bypass admin_skill %page%")
	html := l.adminHTML("char_skills.htm")
	html = strings.ReplaceAll(html, "%name%", target.Name)
	html = strings.ReplaceAll(html, "%content%", b.String())
	sendValidatedHTML(gm, 0, html, 0)
}
