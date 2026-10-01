package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// clanSkillsPage answers a clan skill list asked by anyone but a clan's
// leader, or when the clan has nothing left to learn.
const clanSkillsPage = "data/html/script/feature/Clan/9000-09-no.htm"

// clanSkillItemRequirement tags the item a clan skill's learning takes in
// an AcquireSkillInfo requirement entry.
const clanSkillItemRequirement = 1

// clanSkillMinClass reads a clan skill's minimum clan rank off its
// definition.
func (l *GameClientLink) clanSkillMinClass(sk clan.Skill) (int, bool) {
	if l.skills == nil {
		return 0, false
	}
	def, ok := l.skills.Definition(modelskill.Ref{ID: modelskill.ID(sk.ID), Level: sk.Level})
	return def.MinPledgeClass, ok
}

// clanSkillRefs is skills as skill references.
func clanSkillRefs(skills []clan.Skill) []modelskill.Ref {
	refs := make([]modelskill.Ref, len(skills))
	for i, sk := range skills {
		refs[i] = modelskill.Ref{ID: modelskill.ID(sk.ID), Level: sk.Level}
	}
	return refs
}

// giveClanSkills gives live, on its queue, the skills of cl a member of
// clan rank pledgeClass holds: none while the clan's reputation is 0 or
// less or live fights in the Olympiad.
func (l *GameClientLink) giveClanSkills(live *livePlayer, cl *clan.Clan, pledgeClass int) {
	skills, ok := cl.SkillsFor(pledgeClass, live.OlympiadMode(), l.clanSkillMinClass)
	if ok {
		l.grantClanSkills(live, skills)
	}
}

// grantClanSkills adds skills to live's known skills for as long as it
// stays in its clan; nothing is stored.
func (l *GameClientLink) grantClanSkills(live *livePlayer, skills []clan.Skill) {
	if len(skills) == 0 || l.skills == nil {
		return
	}
	if err := l.skills.GrantTransientSkills(live.Character, clanSkillRefs(skills)); err != nil {
		l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("clan: grant clan skills")
	}
}

// takeClanSkills removes every skill of cl from live, on its queue.
func (l *GameClientLink) takeClanSkills(live *livePlayer, cl *clan.Clan) {
	skills := cl.Skills()
	ids := make([]modelskill.ID, len(skills))
	for i, sk := range skills {
		ids[i] = modelskill.ID(sk.ID)
	}
	l.skills.RevokeSkills(live.Character, ids)
}

// framePledgeSkillList builds cl's clan skill list.
func framePledgeSkillList(cl *clan.Clan) wire.Frame {
	skills := cl.Skills()
	entries := make([]serverpackets.SkillListEntry, len(skills))
	for i, sk := range skills {
		entries[i] = serverpackets.SkillListEntry{ID: int32(sk.ID), Level: int32(sk.Level)}
	}
	return serverpackets.FramePledgeSkillList(entries)
}

// onMemberQueue runs fn for member: inline when member is actor, whose
// queue the caller runs on, else posted to member's own queue so its skill
// state and the frames fn sends keep their order there.
func onMemberQueue(actor, member *livePlayer, fn func()) {
	if member == actor {
		fn()
		return
	}
	postLive(member, fn)
}

// queuedMember receives a broadcast frame on member's queue, behind the
// clan skill changes already posted there; actor's arrive inline.
type queuedMember struct{ actor, member *livePlayer }

func (q queuedMember) BroadcastFrame(frame wire.Frame) bool {
	if q.member == q.actor {
		return q.member.SendFrame(frame)
	}
	if !postLive(q.member, func() { q.member.SendFrame(frame) }) {
		frame.Release()
		return false
	}
	return true
}

// broadcastToClanQueued is broadcastToClan for an operation that changes
// the members' clan skills on their own queues: each member gets the
// frames there, in order behind those changes. actor runs the caller's
// queue and gets its frames inline.
func (l *GameClientLink) broadcastToClanQueued(cl *clan.Clan, actor *livePlayer, builds ...func() wire.Frame) {
	recipients := l.onlineClanMembers(cl, 0)
	for _, build := range builds {
		broadcastFrame(build, func(send func(frameReceiver)) {
			for _, member := range recipients {
				send(queuedMember{actor: actor, member: member})
			}
		})
	}
}

// showPledgeSkillList opens the clan skills live's clan can learn now. A
// player who does not lead a clan, or whose clan has nothing left to
// learn, gets the refusal page instead. Both end with ActionFailed.
func (l *GameClientLink) showPledgeSkillList(live *livePlayer) {
	var nodes []modelskill.ClanSkill
	if cl, ok := l.clanService().ClanOf(live.Character); ok && cl.IsLeader(live.ObjectID()) {
		nodes = cl.LearnableSkills(l.skillTrees)
	}
	if len(nodes) == 0 {
		l.sendClanSkillsPage(live)
	} else {
		entries := make([]serverpackets.AcquireSkillListEntry, len(nodes))
		for i, n := range nodes {
			entries[i] = serverpackets.AcquireSkillListEntry{ID: int32(n.ID), Level: int32(n.Level), Cost: int32(n.Cost)}
		}
		live.SendFrame(serverpackets.FrameAcquireSkillList(serverpackets.AcquireSkillTypeClan, entries))
	}
	live.SendFrame(serverpackets.FrameActionFailed())
}

// sendClanSkillsPage sends the clan skill refusal page, from no NPC.
func (l *GameClientLink) sendClanSkillsPage(live *livePlayer) {
	if l.html == nil {
		return
	}
	page, ok := l.html.Get(clanSkillsPage)
	if !ok {
		l.log.Warn().Str("path", clanSkillsPage).Msg("clan: skills page missing")
		return
	}
	sendValidatedHTML(live, 0, page, 0)
}

// sendClanAcquireSkillInfo answers a clan leader's question about a clan
// skill its clan can learn now: its reputation cost and, when required,
// its item. Anyone else, or a skill the clan cannot learn now, gets no
// answer, as specified; the learn window asks again on its next click.
func (l *GameClientLink) sendClanAcquireSkillInfo(live *livePlayer, req clientpackets.RequestAcquireSkillInfo) {
	node, needsItem, ok := l.clanService().SkillOffer(live.Character, l.skillTrees, int(req.SkillID), int(req.Level))
	if !ok {
		return
	}
	var reqs []serverpackets.SkillRequirement
	if needsItem {
		reqs = []serverpackets.SkillRequirement{{Type: clanSkillItemRequirement, ItemID: node.ItemID, Count: 1}}
	}
	live.SendFrame(serverpackets.FrameAcquireSkillInfo(req.SkillID, req.Level, int32(node.Cost), int32(serverpackets.AcquireSkillTypeClan), reqs))
}

// learnClanSkill has live's clan learn a clan skill for its reputation
// and, when required, one of its item, then reopens the clan skill list.
// A player who does not lead a clan, or a skill that is not the clan's
// next learnable level, gets no answer, as specified.
func (l *GameClientLink) learnClanSkill(live *livePlayer, req clientpackets.RequestAcquireSkill) {
	cl, _ := l.clanService().ClanOf(live.Character)
	payer := clanLevelPayer{l: l, live: live}
	pay := func(itemID int32) bool { return payer.PayItem(itemID, 1) }
	for _, n := range l.clanService().LearnSkill(live.Character, l.skillTrees, int(req.SkillID), int(req.Level), pay) {
		switch n := n.(type) {
		case clan.SkillUnavailable:
			return
		case clan.SkillLowReputation:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageAcquireSkillFailedBadClanRepScore))
			l.showPledgeSkillList(live)
			return
		case clan.SkillMissingItem:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageItemMissingToLearnSkill))
			l.showPledgeSkillList(live)
			return
		case clan.ReputationChanged:
			l.sendReputationChange(cl, n, live)
		case clan.ReputationDeducted:
			live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageS1DeductedFromClanRep, int32(n.Points)))
		case clan.SkillLearned:
			l.sendClanSkillLearned(cl, n, live)
		}
	}
	l.showPledgeSkillList(live)
}

// sendClanSkillLearned shows every online member of cl, actor included,
// the skill its clan learnt: a member whose rank reaches it is given it
// with its refreshed skill list, unless the price just turned the clan's
// skills off; then each gets the clan skill list's addition and the notice
// naming the skill.
func (l *GameClientLink) sendClanSkillLearned(cl *clan.Clan, learned clan.SkillLearned, actor *livePlayer) {
	sk := learned.Skill
	for _, member := range l.onlineClanMembers(cl, 0) {
		onMemberQueue(actor, member, func() {
			if !cl.IsMember(member.ObjectID()) {
				return
			}
			if !learned.Refresh && cl.GivesSkill(member.PledgeClass(), member.OlympiadMode(), sk, l.clanSkillMinClass) {
				l.grantClanSkills(member, []clan.Skill{sk})
				member.SendFrame(serverpackets.FrameSkillList(skillListEntries(member.Character, l.skills)))
			}
			member.SendFrame(serverpackets.FramePledgeSkillListAdd(int32(sk.ID), int32(sk.Level)))
			// The notice names the skill at level 1, whatever level was
			// learnt.
			member.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageClanSkillS1Added, int32(sk.ID), 1))
		})
	}
}

// sendReputationChange shows cl's online members its new reputation, on
// their own queues (actor's inline). When the score crossed 0, each member
// first learns its clan skills turned off or on, loses or regains them, and
// gets its skill list, then the clan's header. Otherwise each just gets the
// header.
func (l *GameClientLink) sendReputationChange(cl *clan.Clan, change clan.ReputationChanged, actor *livePlayer) {
	if change.Crossed == 0 {
		l.broadcastToClanQueued(cl, actor, func() wire.Frame { return framePledgeShowInfoUpdate(cl) })
		return
	}
	message := serverpackets.SystemMessageClanSkillsActivatedReputation
	if change.Crossed < 0 {
		message = serverpackets.SystemMessageReputationLowClanSkillsDeactivated
	}
	for _, member := range l.onlineClanMembers(cl, 0) {
		onMemberQueue(actor, member, func() {
			if !cl.IsMember(member.ObjectID()) {
				return
			}
			member.SendFrame(serverpackets.FrameSystemMessage(message))
			l.applyReputationCrossing(member, cl, change.Crossed)
			member.SendFrame(serverpackets.FrameSkillList(skillListEntries(member.Character, l.skills)))
			member.SendFrame(framePledgeShowInfoUpdate(cl))
		})
	}
}

// applyReputationCrossing takes cl's skills from member as the score fell
// to 0 or below, or gives back those its rank reaches as it rose above 0.
// Each side only applies while the score still stands on it when the
// member's queue runs it, so crossings delivered out of order still leave
// the member as the current score says.
func (l *GameClientLink) applyReputationCrossing(member *livePlayer, cl *clan.Clan, crossed int) {
	positive := cl.Reputation() > 0
	switch {
	case crossed < 0 && !positive:
		l.takeClanSkills(member, cl)
	case crossed > 0 && positive:
		l.grantClanSkills(member, cl.ReachedSkills(member.PledgeClass(), l.clanSkillMinClass))
	}
}
