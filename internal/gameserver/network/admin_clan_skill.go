package network

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// The clan skills are the skill ids from firstClanSkillID to
// lastClanSkillID, each at its highest level.
const (
	firstClanSkillID = 370
	lastClanSkillID  = 391
)

// adminClanSkillPageLimit is how many skills a page of //clan_skill shows.
const adminClanSkillPageLimit = 15

// adminClanSkill answers //clan_skill on the selected player, gm itself
// without one, who must lead a clan; otherwise gm is told so and shown
// the player's skill page instead.
//
//   - //clan_skill [page] opens the clan skill page of its clan;
//   - //clan_skill set all, //clan_skill set <id> <level>, //clan_skill
//     remove all and //clan_skill remove <id> change its clan's skills,
//     then open the page that follows them.
//
// The command runs on the leader's queue, where its clan's skill learning
// runs too, so a change never lands between a learn's payment and its
// check that the skill is still to learn. A page number past the int range
// ends the command where it is read: after a set or remove, gm is still
// told the change or the usage, but gets no page.
func (l *GameClientLink) adminClanSkill(gm *livePlayer, line string) {
	target := adminTargetPlayer(gm, true)
	args := handleradmin.Args(line)
	onPlayer(gm, target, func() { l.runAdminClanSkill(gm, target, args) })
}

// runAdminClanSkill runs //clan_skill with args on target's queue.
func (l *GameClientLink) runAdminClanSkill(gm, target *livePlayer, args []string) {
	cl, ok := l.clanService().ClanOf(target.Character)
	if !ok || !cl.IsLeader(target.ObjectID()) {
		gm.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1IsNotAClanLeader, target.Name))
		l.showAdminSkills(gm, target, 1)
		return
	}
	next := func() (string, bool) {
		if len(args) == 0 {
			return "", false
		}
		tok := args[0]
		args = args[1:]
		return tok, true
	}
	page := 1
	// readPage takes tok as the page number when it is one, and reports
	// false when that number is past the int range.
	readPage := func(tok string) bool {
		if !isDigits(tok) {
			return true
		}
		n, ok := parseJavaInt(tok)
		page = int(n)
		return ok
	}

	var report string
	if param, ok := next(); ok {
		if isDigits(param) {
			if !readPage(param) {
				return
			}
		} else {
			switch param {
			case "set":
				var listed bool
				if report, listed = l.adminClanSkillSet(target, cl, next); listed {
					l.replyAdminClanSkills(gm, target, cl, report, page, true)
					return
				}
			case "remove":
				report = l.adminClanSkillRemove(target, cl, next)
			}
			if tok, ok := next(); ok && !readPage(tok) {
				l.replyAdminClanSkills(gm, target, cl, report, 0, false)
				return
			}
		}
	}
	l.replyAdminClanSkills(gm, target, cl, report, page, true)
}

// adminClanSkillSet runs //clan_skill set on leader's queue: "all" has cl
// know every clan skill at its highest level, <id> <level> that one skill.
// It returns what gm is told. listed is true when an id and level out of
// the clan skill range, or naming no skill, end the command with its usage
// and the first page.
func (l *GameClientLink) adminClanSkillSet(leader *livePlayer, cl *clan.Clan, next func() (string, bool)) (report string, listed bool) {
	const usage = "Usage: //clan_skill set id level [page]"
	param, ok := next()
	if !ok {
		return usage, false
	}
	if param == "all" {
		catalog := l.clanSkillCatalog()
		skills := make([]clan.Skill, len(catalog))
		for i, def := range catalog {
			skills[i] = clan.Skill{ID: int(def.ID), Level: def.Level}
		}
		if !l.clanService().RaiseSkills(cl, skills) {
			return "", false
		}
		l.sendClanSkillsRaised(cl, leader)
		return "You gave all available skills to " + cl.Name() + " clan.", false
	}
	id, ok := parseJavaInt(param)
	if !ok {
		return usage, false
	}
	levelParam, _ := next()
	level, ok := parseJavaInt(levelParam)
	if !ok {
		return usage, false
	}
	if id < firstClanSkillID || id > lastClanSkillID || level < 1 || level > 3 || l.skills == nil {
		return usage, true
	}
	def, ok := l.skills.Definition(modelskill.Ref{ID: modelskill.ID(id), Level: int(level)})
	if !ok {
		return usage, true
	}
	sk := clan.Skill{ID: int(id), Level: int(level)}
	if !l.clanService().SetSkill(cl, sk) {
		return "", false
	}
	l.sendClanSkillLearned(cl, clan.SkillLearned{Skill: sk}, leader)
	return "You gave " + def.Name + " skill to " + cl.Name() + " clan.", false
}

// adminClanSkillRemove runs //clan_skill remove on leader's queue: "all"
// has cl forget every skill, <id> that one skill. It returns what gm is
// told.
func (l *GameClientLink) adminClanSkillRemove(leader *livePlayer, cl *clan.Clan, next func() (string, bool)) string {
	const usage = "Usage: //clan_skill remove id|all [page]"
	param, ok := next()
	if !ok {
		return usage
	}
	if param == "all" {
		removed, ok := l.clanService().RemoveAllSkills(cl)
		if !ok {
			return ""
		}
		ids := make([]int, len(removed))
		for i, sk := range removed {
			ids[i] = sk.ID
		}
		l.sendClanSkillsRemoved(cl, ids, leader)
		return "You removed all skills from " + cl.Name() + " clan."
	}
	id, ok := parseJavaInt(param)
	if !ok {
		return usage
	}
	if !l.clanService().RemoveSkill(cl, int(id)) {
		return ""
	}
	l.sendClanSkillsRemoved(cl, []int{int(id)}, leader)
	return "You removed " + strconv.Itoa(int(id)) + " skillId from " + cl.Name() + " clan."
}

// sendClanSkillsRaised shows every online member of cl, actor included,
// its clan's raised skills: a member that holds clan skills is given those
// its rank reaches, with its refreshed skill list, then each gets the full
// clan skill list.
func (l *GameClientLink) sendClanSkillsRaised(cl *clan.Clan, actor *livePlayer) {
	for _, member := range l.onlineClanMembers(cl, 0) {
		onMemberQueue(actor, member, func() {
			if !cl.IsMember(member.ObjectID()) {
				return
			}
			if skills, ok := cl.SkillsFor(member.PledgeClass(), member.OlympiadMode(), l.clanSkillMinClass); ok {
				l.grantClanSkills(member, skills)
				member.SendFrame(serverpackets.FrameSkillList(skillListEntries(member.Character, l.skills)))
			}
			member.SendFrame(framePledgeSkillList(cl))
		})
	}
}

// sendClanSkillsRemoved shows every online member of cl, actor included,
// the skills its clan forgot: each member loses them, then gets its skill
// list and the clan skill list without them.
func (l *GameClientLink) sendClanSkillsRemoved(cl *clan.Clan, ids []int, actor *livePlayer) {
	for _, member := range l.onlineClanMembers(cl, 0) {
		onMemberQueue(actor, member, func() {
			if !cl.IsMember(member.ObjectID()) {
				return
			}
			for _, id := range ids {
				l.removeLiveSkill(member, id, false)
			}
			member.SendFrame(serverpackets.FrameSkillList(skillListEntries(member.Character, l.skills)))
			member.SendFrame(framePledgeSkillList(cl))
		})
	}
}

// replyAdminClanSkills tells gm report, when there is one, then, when
// showPage is set, opens page of cl's clan skill page on it. A gm other
// than leader gets both on its own queue, behind the clan skill changes
// posted there when it belongs to cl.
func (l *GameClientLink) replyAdminClanSkills(gm, leader *livePlayer, cl *clan.Clan, report string, page int, showPage bool) {
	if report == "" && !showPage {
		return
	}
	onMemberQueue(leader, gm, func() {
		if report != "" {
			sendText(gm, report)
		}
		if showPage {
			l.showAdminClanSkills(gm, cl, page)
		}
	})
}

// clanSkillCatalog returns every clan skill at its highest level, by id.
func (l *GameClientLink) clanSkillCatalog() []modelskill.Definition {
	if l.skills == nil {
		return nil
	}
	var out []modelskill.Definition
	for id := modelskill.ID(firstClanSkillID); id <= lastClanSkillID; id++ {
		if def, ok := l.skills.Definition(modelskill.Ref{ID: id, Level: l.skills.MaxLevel(id)}); ok {
			out = append(out, def)
		}
	}
	return out
}

// showAdminClanSkills opens page of the clan skill page on gm: every clan
// skill by id, at its highest level, or at the level cl knows it with a
// link that removes it. A page number below 1 opens nothing.
func (l *GameClientLink) showAdminClanSkills(gm *livePlayer, cl *clan.Clan, page int) {
	catalog := l.clanSkillCatalog()
	pg, ok := handleradmin.Paginate(len(catalog), page, adminClanSkillPageLimit)
	if !ok {
		l.log.Warn().Int("page", page).Msg("admin: //clan_skill page out of range")
		return
	}
	known := map[int]int{}
	for _, sk := range cl.Skills() {
		known[sk.ID] = sk.Level
	}
	var b strings.Builder
	b.WriteString("<table width=270><tr><td width=220>Name</td><td width=20>Lvl</td><td width=30>Id</td></tr>")
	for _, def := range catalog[pg.Start:pg.End] {
		if level, ok := known[int(def.ID)]; ok {
			fmt.Fprintf(&b, `<tr><td><a action="bypass -h admin_clan_skill remove %d">%s</a></td><td>%d</td><td>%d</td></tr>`, def.ID, def.Name, level, def.ID)
		} else {
			fmt.Fprintf(&b, "<tr><td>%s</td><td>%d</td><td>%d</td></tr>", def.Name, def.Level, def.ID)
		}
	}
	b.WriteString("</table><br>")
	pg.Space(&b, 17)
	pg.Links(&b, "bypass admin_clan_skill %page%")
	l.sendAdminSkillPage(gm, "clan_skills.htm", cl.Name(), b.String())
}
