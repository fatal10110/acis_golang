// Package primevalsummoner is Vervato of the Primeval Isle, who for a fee
// summons the bodies of a player's dead party members lying on the island
// to his side.
package primevalsummoner

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// vervato is the summoner.
const vervato = 32104

// corpseRevival is the skill Vervato summons the bodies with.
const corpseRevival = 5121

// New returns the teleporter.
func New() script.Script {
	return script.Script{
		Dir: "teleport",
		Bind: script.Bindings{
			script.EventTalked:           {vervato},
			script.EventCreated:          {vervato},
			script.EventUseSkillFinished: {vervato},
		},
		Hooks: script.Hooks{OnCreated: onCreated, OnTalk: onTalk, OnUseSkillFinished: onUseSkillFinished, OnTimer: onTimer},
	}
}

// onCreated leaves Vervato free: his first creature slot holds himself
// while no one's summon is under way, else the player who paid for it.
func onCreated(_ *script.Script, e script.Created) {
	e.NPC.SetSlot(script.SlotCreature0, e.NPC)
}

// onTalk takes the fee from a player with a dead party member on the
// island and starts the summon, answering nothing. A player with none is
// turned away, one short of the fee told so; one who pays while a summon
// is under way pays for nothing and is told to come back.
func onTalk(_ *script.Script, e script.Talk) string {
	htmltext := ""

	i2 := 0

	n, player := e.NPC, e.Player
	for _, partyMember := range player.PartyMembers() {
		if partyMember.Dead() && n.InSpawnTerritory(partyMember) {
			i2++
		}
	}

	if i2 == 0 {
		htmltext = "vervato002.htm"
	} else if player.ReduceAdena(200000) {
		if script.Same(n.Slot(script.SlotCreature0), n) {
			n.SetSlot(script.SlotCreature0, player)

			n.AddCastDesire(n, skill.Ref{ID: corpseRevival, Level: 1}, 1000000)
		} else {
			htmltext = "vervato005.htm"
		}
	} else {
		htmltext = "vervato003.htm"
	}

	return htmltext
}

// onUseSkillFinished brings every dead party member of the paying player
// lying on the island to Vervato's side, then readies the closing page.
func onUseSkillFinished(s *script.Script, e script.UseSkillFinished) {
	n := e.NPC
	if e.Skill.ID == corpseRevival && !script.Same(n.Slot(script.SlotCreature0), n) {
		if payer, ok := n.Slot(script.SlotCreature0).(*script.Player); ok {
			for _, partyMember := range payer.PartyMembers() {
				if partyMember.Dead() && n.InSpawnTerritory(partyMember) {
					partyMember.TeleportTo(script.Loc{X: 11320, Y: -23504, Z: -3640}, 0)
				}
			}
		}
		s.StartTimer("2005", n, nil, 3000*time.Millisecond)
	}
}

// onTimer shows the paying player the closing page, and leaves Vervato
// free again.
func onTimer(s *script.Script, e script.Timer) string {
	n := e.NPC
	if strings.EqualFold(e.Name, "2005") && !script.Same(n.Slot(script.SlotCreature0), n) {
		if payer, ok := n.Slot(script.SlotCreature0).(*script.Player); ok {
			payer.ShowPage(n.ObjectID(), "./data/html/script/"+s.Dir+"/"+s.Name+"/"+"vervato004.htm")
		}
		n.SetSlot(script.SlotCreature0, n)
	}
	var base script.Hooks
	return base.Timer(s, e)
}
