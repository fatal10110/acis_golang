package network

import (
	skillhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/schemebuffer"
)

// schemeBufferBypass runs scheme buffer f's own command for live: what it
// does to live and live's summon, then its notice, then its page, opened
// with no NPC object id. It reports false when the command was too
// malformed to answer, so that nothing more is sent.
func (l *GameClientLink) schemeBufferBypass(live *livePlayer, f *npc.Folk, command string) bool {
	talker := schemebuffer.Talker{ObjectID: live.ObjectID(), MaxBuffCount: live.MaxBuffCount()}
	reply := l.schemeBuffer.Bypass(l.setPage, f.NpcID(), f.ObjectID(), talker, command)
	if reply.Aborted {
		return false
	}
	pet, _ := l.summonOf(live).(*summon.Actor)
	switch reply.Action {
	case schemebuffer.ActionCleanup:
		live.EffectList().StopAllExceptThoseThatLastThroughDeath()
		l.broadcastCharacterInfo(live)
		if pet != nil {
			pet.EffectList().StopAllExceptThoseThatLastThroughDeath()
		}
	case schemebuffer.ActionHeal:
		live.SetMaxCpHpMp()
		if pet != nil {
			pet.SetMaxHpMp()
		}
	case schemebuffer.ActionGive:
		l.giveScheme(live, f, pet, reply)
	case schemebuffer.ActionNone:
	}
	if reply.Message != "" {
		sendText(live, reply.Message)
	}
	if reply.Page != "" {
		sendFilledHTML(live, 0, reply.Page, 0)
	}
	return true
}

// giveScheme lands reply's buffs from f on live, or on pet, once live has
// paid the fee. A fee of 0 takes nothing; a missing pet is told so.
func (l *GameClientLink) giveScheme(live *livePlayer, f *npc.Folk, pet *summon.Actor, reply schemebuffer.Reply) {
	var target skillhandler.Actor
	switch {
	case reply.GiveTo == schemebuffer.GiveSelf:
		target = live.Character
	case reply.GiveTo == schemebuffer.GivePet && pet != nil:
		target = pet
	}
	if target == nil {
		sendText(live, schemebuffer.NoPetMessage)
		return
	}
	if reply.Cost != 0 && !reduceAdena(live, int(reply.Cost)) {
		return
	}
	for _, def := range reply.Buffs {
		skillhandler.LandEffects(f, target, def)
	}
}
