package npc

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"

// Say shows text as h's chat line to the players around it, named by h's
// npc id (Npc.broadcastNpcSay).
func (h *Hostile) Say(text string) {
	h.emit(event.NpcSay{NpcID: h.Instance.Template.ID, Text: text})
}

// Say shows text as f's chat line to the players around it, named by f's
// npc id (Npc.broadcastNpcSay).
func (f *Folk) Say(text string) {
	f.emit(event.NpcSay{NpcID: f.Instance.Template.ID, Text: text})
}

// SocialAction plays the social animation id on f for the players around
// it.
func (f *Folk) SocialAction(id int) {
	f.emit(event.SocialAction{ID: int32(id)})
}
