package network

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// adminPolymorphUsage is what //polymorph answers a command it cannot read.
const adminPolymorphUsage = "Usage: //polymorph npcId"

// adminPolymorph answers //polymorph <npcId>, a toggle on the selected
// creature, else on gm: a player or an NPC shown in a form is shown as
// itself again, whatever follows the command; any other is shown as the NPC
// npcId names. The player the creature acts for leaves its mount first. A
// summon or a door takes no form, and an npc id no NPC has is refused.
func (l *GameClientLink) adminPolymorph(gm *livePlayer, line string) {
	switch t := gm.Target().(type) {
	case *livePlayer:
		l.adminPolymorphPlayer(gm, t, line)
	case *summon.Actor:
		if owner, ok := l.livePlayerByID(t.OwnerID()); ok {
			onPlayer(gm, owner, func() { owner.Character.Dismount() })
		}
		if _, ok := l.adminPolymorphForm(gm, line); ok {
			gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageApplicantInformationIncorrect))
		}
	case *door.Object:
		if _, ok := l.adminPolymorphForm(gm, line); ok {
			gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageApplicantInformationIncorrect))
		}
	case morphingNPC:
		// A working control tower is in its working form, so it falls dead
		// here, its HP as it is.
		if t.Polymorph() != nil {
			t.Morph(nil)
			return
		}
		if tmpl, ok := l.adminPolymorphTemplate(gm, line); ok {
			t.Morph(tmpl)
		}
	default:
		l.adminPolymorphPlayer(gm, gm, line)
	}
}

// adminPolymorphPlayer is //polymorph on target, on target's queue.
func (l *GameClientLink) adminPolymorphPlayer(gm, target *livePlayer, line string) {
	onPlayer(gm, target, func() {
		target.Character.Dismount()
		if target.Polymorph() != nil {
			l.showPolymorph(target, nil)
			return
		}
		if form, ok := l.adminPolymorphForm(gm, line); ok {
			l.showPolymorph(target, form)
		}
	})
}

// morphingNPC is an NPC //polymorph shows in a form.
type morphingNPC interface {
	Polymorph() *npc.Template
	Morph(form *npc.Template)
}

// adminPolymorphForm reads the player form the //polymorph line names, as
// adminPolymorphTemplate does.
func (l *GameClientLink) adminPolymorphForm(gm *livePlayer, line string) (*player.NPCForm, bool) {
	tmpl, ok := l.adminPolymorphTemplate(gm, line)
	if !ok {
		return nil, false
	}
	return &player.NPCForm{
		NPCID: int32(tmpl.ID), Name: tmpl.Name,
		RightHand: int32(tmpl.RightHand), LeftHand: int32(tmpl.LeftHand),
		CollisionRadius: tmpl.CollisionRadius, CollisionHeight: tmpl.CollisionHeight,
	}, true
}

// adminPolymorphTemplate reads the NPC the //polymorph line names. A
// missing or unreadable id gets gm the usage, an id no NPC has the refusal.
func (l *GameClientLink) adminPolymorphTemplate(gm *livePlayer, line string) (*npc.Template, bool) {
	args := strings.FieldsFunc(line, func(r rune) bool { return strings.ContainsRune(" \t\n\r\f", r) })
	if len(args) < 2 {
		sendText(gm, adminPolymorphUsage)
		return nil, false
	}
	id, err := strconv.ParseInt(args[1], 10, 32)
	if err != nil {
		sendText(gm, adminPolymorphUsage)
		return nil, false
	}
	var tmpl *npc.Template
	ok := false
	if l.npcs != nil {
		tmpl, ok = l.npcs.Get(int(id))
	}
	if !ok {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageApplicantInformationIncorrect))
		return nil, false
	}
	return tmpl, true
}

// showPolymorph shows live in form, or as itself again for a nil form: the
// players around forget live and see it again as it now is, then live
// reads its own new name and title.
func (l *GameClientLink) showPolymorph(live *livePlayer, form *player.NPCForm) {
	live.SetPolymorph(form)
	l.leaveGrid(live)
	l.rejoinGrid(live)
	live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
}
