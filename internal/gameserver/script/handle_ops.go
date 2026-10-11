package script

import (
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/npcstring"
)

// ItemCount returns how many units of itemID the player holds, worn ones
// included: a stack's count, or the number of instances of an item that
// does not stack.
func (p *Player) ItemCount(itemID int32) int32 {
	return int32(p.character().ItemCount(int(itemID)))
}

// npcStringParam is the placeholder each parameter of an NpcString text
// replaces, first to last.
const npcStringParam = "%s"

// NpcString returns the client text of the NpcString id, each of params
// replacing the next placeholder in turn; a text short of params keeps its
// placeholders. An id with no text panics, so the invocation aborts there.
func NpcString(id int32, params ...any) string {
	text, ok := npcstring.Text(id)
	if !ok {
		panic(fmt.Sprintf("script: no NpcString %d", id))
	}
	for _, param := range params {
		text = strings.Replace(text, npcStringParam, fmt.Sprint(param), 1)
	}
	return text
}

// Say shows text as the NPC's chat line to the players around it. A handle
// on nothing says nothing.
func (n *NPC) Say(text string) {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		o.Say(text)
	case *npc.Folk:
		o.Say(text)
	}
}

// scratch returns the NPC's script memory; a handle on nothing panics.
func (n *NPC) scratch() *npc.Scratch {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		return o.Scratch()
	case *npc.Folk:
		return o.Scratch()
	}
	panic("script: the script value of a handle on nothing")
}

// ScriptValue returns the NPC's script value: 0 at each spawn until a
// script sets it.
func (n *NPC) ScriptValue() int32 { return n.scratch().ScriptValue() }

// SetScriptValue sets the NPC's script value to v for the rest of its life.
func (n *NPC) SetScriptValue(v int32) { n.scratch().SetScriptValue(v) }

// IsScriptValue reports whether the NPC's script value is v.
func (n *NPC) IsScriptValue(v int32) bool { return n.ScriptValue() == v }

// The beginner shots and what comes with them.
const (
	newbieShotsQuestionMark = 26
	soulshotNoGrade         = 5789
	spiritshotNoGrade       = 5790
	voiceSoulshots          = "tutorial_voice_026"
	voiceSpiritshots        = "tutorial_voice_027"
	classOrcMystic          = 49
	classOrcShaman          = 50
)

// RewardNewbieShots gives p, once per script, beginner shots for its
// class: spsCount spiritshots to a mystic, other than an orc mystic or orc
// shaman, ssCount soulshots to anyone else, each with question mark 26 and
// its tutorial voice; a count of 0 gives nothing. Only a newbie of level 6
// or higher gets anything, and only while the script's one-time memo is
// unset; the memo is set whether or not shots were given.
func (s *Script) RewardNewbieShots(p *Player, ssCount, spsCount int32) {
	c := p.character()
	flag := s.Name + "_OneTimeQuestFlag"
	if !c.IsNewbie(true) {
		return
	}
	if _, done := c.Memos().Get(flag); done {
		return
	}
	class := c.ClassID()
	mage := player.ClassMage(class) && class != classOrcMystic && class != classOrcShaman
	switch {
	case spsCount > 0 && mage:
		p.ShowQuestionMark(newbieShotsQuestionMark)
		s.RewardItems(p, spiritshotNoGrade, spsCount)
		p.PlayTutorialVoice(voiceSpiritshots)
	case ssCount > 0 && !mage:
		p.ShowQuestionMark(newbieShotsQuestionMark)
		s.RewardItems(p, soulshotNoGrade, ssCount)
		p.PlayTutorialVoice(voiceSoulshots)
	}
	c.Memos().Set(flag, "true")
}

// SocialAction plays the social animation id on the player, for the
// player and every player around it.
func (p *Player) SocialAction(id int32) { p.character().BroadcastSocialAction(id) }

// SystemMessageNumbers shows the player the system message id with
// numbers as its parameters, in order.
func (p *Player) SystemMessageNumbers(id int, numbers ...int32) {
	p.character().NotifySystemMessageNumbers(id, numbers...)
}

// clanCharacter returns the player, which must be in a clan: a clanless
// player panics, so the invocation aborts there.
func (p *Player) clanCharacter() *player.Character {
	c := p.character()
	if c.ClanID() == 0 {
		panic("script: the clan of a player in no clan")
	}
	return c
}

// ClanLevel returns the level of the player's clan. A player in no clan
// panics.
func (p *Player) ClanLevel() int32 {
	level, _ := p.clanCharacter().ClanLevel()
	return int32(level)
}

// AddClanReputation adds points to the reputation of the player's clan.
// Its members in the world see the new score as on any reputation change;
// a clan below level 5 gains nothing. A player leaving the world adds
// nothing, as no script gives it anything. A player in no clan panics.
func (p *Player) AddClanReputation(points int32) {
	p.clanCharacter().AddScriptClanReputation(int(points))
}

// BroadcastClanInfo shows every member of the player's clan in the world
// the clan's header. A player in no clan panics.
func (p *Player) BroadcastClanInfo() { p.clanCharacter().BroadcastClanInfo() }

// SocialAction plays the social animation id on the NPC for the players
// around it. A handle on nothing plays nothing.
func (n *NPC) SocialAction(id int32) {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		o.SocialAction(int(id))
	case *npc.Folk:
		o.SocialAction(int(id))
	}
}
