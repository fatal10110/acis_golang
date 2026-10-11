package network

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// npcScriptInfo is what the //info NPC script pages read of the script
// registry.
type npcScriptInfo interface {
	NPCScripts(npcID int32) []script.EventScripts
	AIClasses(npcID int32) []string
	NPCTimers(n attackable.Combatant) []script.ScriptTimers
}

// npcScriptPage is npcinfo/script.htm for the NPC obj of template t: the
// scripts on each event list of the template, by event, and the pending
// timers bound to the NPC, by script.
func (l *GameClientLink) npcScriptPage(obj world.Tracked, t *npc.Template) string {
	var events []script.EventScripts
	var timers []script.ScriptTimers
	if l.scripts != nil {
		events = l.scripts.NPCScripts(int32(t.ID))
		if c, ok := obj.(attackable.Combatant); ok {
			timers = l.scripts.NPCTimers(c)
		}
	}
	var b strings.Builder
	if len(events) == 0 {
		b.WriteString("This NPC isn't affected by scripts.")
	}
	for _, e := range events {
		b.WriteString(`<br><font color="LEVEL">` + e.Event + "</font><br1>")
		for _, name := range e.Scripts {
			b.WriteString(name + "<br1>")
		}
	}
	page := strings.ReplaceAll(l.adminHTML("npcinfo/script.htm"), "%scripts%", b.String())

	b.Reset()
	for _, st := range timers {
		b.WriteString(`<br><font color="LEVEL">` + st.Script + "</font><br1>")
		for _, tm := range st.Timers {
			b.WriteString(tm.Name)
			if tm.Player != "" {
				b.WriteString(" affecting player " + tm.Player)
			}
			b.WriteString("<br1>")
		}
	}
	return strings.ReplaceAll(page, "%tasks%", b.String())
}

// The tab rows of the //info ai pages, one per page shown.
const (
	npcAIPathTabs = `[AI Path]</td><td width=70><a action="bypass -h admin_info ai 1">Template</a></td><td width=70><a action="bypass -h admin_info ai 2">Spawn</a></td><td width=70><a action="bypass -h admin_info ai 3">Npc</a></td></tr></table></center><br>`
	npcAINpcTabs  = `<a action="bypass -h admin_info ai 0">AI Path</a></td><td width=70><a action="bypass -h admin_info ai 1">Template</a></td><td width=70><a action="bypass -h admin_info ai 2">Spawn</a></td><td width=70>[Npc]</td></tr></table></center><br>`
)

// npcAIPathContent is //info ai page 0 of an NPC of template t: the class
// chain of the first script bound to the template that is not a real
// quest, its own class first.
func (l *GameClientLink) npcAIPathContent(t *npc.Template) string {
	var classes []string
	if l.scripts != nil {
		classes = l.scripts.AIClasses(int32(t.ID))
	}
	var b strings.Builder
	b.WriteString("<center><table width=240><tr><td width=70>" + npcAIPathTabs)
	if len(classes) == 0 {
		b.WriteString("This NPC doesn't hold any AI related script.")
	}
	for _, c := range classes {
		b.WriteString(c + "<br1>")
	}
	return b.String()
}

// scratchHolder is an NPC holding script memory.
type scratchHolder interface {
	Scratch() *npc.Scratch
}

// npcScratchContent is //info ai page 3 of NPC obj: the integer and
// creature slots of its script memory, its parameters, flag, respawn time
// and weight point.
func npcScratchContent(obj world.Tracked) string {
	mem := npc.NewScratch()
	if h, ok := obj.(scratchHolder); ok {
		if s := h.Scratch(); s != nil {
			mem = s
		}
	}
	num := func(slot npc.IntSlot) string { return strconv.Itoa(int(mem.Int(slot))) }
	who := func(slot npc.CreatureSlot) string { return creatureDescription(mem.Creature(slot)) }
	row := func(left, lv, right, rv string) string {
		return `<tr><td width=140><font color="LEVEL">[` + left + `]</font> ` + lv + `</td><td width=140 align=left><font color="LEVEL">[` + right + `]</font> ` + rv + `</td></tr>`
	}
	var b strings.Builder
	b.WriteString("<center><table width=240><tr><td width=70>" + npcAINpcTabs)
	b.WriteString("<table width=280>")
	for i := range 5 {
		n := strconv.Itoa(i)
		b.WriteString(row("i_ai"+n, num(npc.IntAI0+npc.IntSlot(i)), "i_quest"+n, num(npc.IntQuest0+npc.IntSlot(i))))
	}
	b.WriteString("</table><br><table width=280>")
	for i := range 5 {
		n := strconv.Itoa(i)
		b.WriteString(row("c_ai"+n, who(npc.CreatureAI0+npc.CreatureSlot(i)), "c_quest"+n, who(npc.CreatureQuest0+npc.CreatureSlot(i))))
	}
	b.WriteString("</table><br><table width=280>")
	b.WriteString(row("param1", num(npc.IntParam1), "flag", num(npc.IntFlag)))
	b.WriteString(row("param2", num(npc.IntParam2), "respawnTime", num(npc.IntRespawnTime)))
	b.WriteString(row("param3", num(npc.IntParam3), "weightPoint", num(npc.IntWeightPoint)))
	b.WriteString("</table>")
	return b.String()
}

// creatureDescription is a creature slot as the admin pages show it: a
// link teleporting to where the creature stands, named after it trimmed
// to 12 characters, or "none" for an empty slot.
func creatureDescription(c npc.ScratchCreature) string {
	cb, ok := c.(attackable.Combatant)
	if !ok {
		return "none"
	}
	x, y, z := cb.Position()
	loc := strconv.Itoa(x) + " " + strconv.Itoa(y) + " " + strconv.Itoa(z) + " " + strconv.Itoa(cb.Heading())
	return `<a action="bypass admin_teleport ` + loc + `">` + trimAndDress(attackerName(cb), 12) + "</a>"
}
