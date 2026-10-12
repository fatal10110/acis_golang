package network

import (
	"cmp"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// The tab rows of the //info ai pages 1 and 2.
const (
	npcAITemplateTabs = `<a action="bypass -h admin_info ai 0">AI Path</a></td><td width=70>[Template]</a></td><td width=70><a action="bypass -h admin_info ai 2">Spawn</a></td><td width=70><a action="bypass -h admin_info ai 3">Npc</a></td></tr></table></center><br>`
	npcAISpawnTabs    = `<a action="bypass -h admin_info ai 0">AI Path</a></td><td width=70><a action="bypass -h admin_info ai 1">Template</a></td><td width=70>[Spawn]</td><td width=70><a action="bypass -h admin_info ai 3">Npc</a></td></tr></table></center><br>`
)

// npcNoSpawnField fills a spawn page field of an NPC with no spawn.
const npcNoSpawnField = "<font color=FF0000>--</font>"

// npcTemplateParamsContent is //info ai page 1 of an NPC of template t:
// the template's AI parameters, in the order its memo, a copy of the one
// they were loaded into, lists them.
func npcTemplateParamsContent(t *npc.Template) string {
	var b strings.Builder
	b.WriteString("<center><table width=240><tr><td width=70>" + npcAITemplateTabs)
	if len(t.AIParams) == 0 {
		b.WriteString("This NPC's template doesn't hold any AI parameters.")
		return b.String()
	}
	writeParamRows(&b, t.AIParams, memoCopyOrder(paramKeys(t.AIParams, t.AIParamKeys)))
	return b.String()
}

// npcSpawnParamsContent is //info ai page 2 of NPC obj: the AI parameters
// its spawn declares, in the order the spawn's memo lists them.
func (l *GameClientLink) npcSpawnParamsContent(obj world.Tracked) string {
	var b strings.Builder
	b.WriteString("<center><table width=240><tr><td width=70>" + npcAISpawnTabs)
	rec, ok := l.npcSpawnOf(obj)
	switch {
	case !ok:
		b.WriteString("This NPC doesn't have any Spawn.")
	case len(rec.Entry.AIParams) == 0:
		b.WriteString("This NPC Spawn doesn't hold any memos.")
	default:
		params := rec.Entry.AIParams
		writeParamRows(&b, params, memoOrder(paramKeys(params, rec.Entry.AIParamKeys)))
	}
	return b.String()
}

// paramKeys is every name of params: those of declared in that order, then
// any other in ascending order.
func paramKeys(params map[string]string, declared []string) []string {
	keys := make([]string, 0, len(params))
	for _, k := range declared {
		if _, ok := params[k]; ok {
			keys = append(keys, k)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(params)) {
		if !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	return keys
}

// writeParamRows writes one row of params to b for each name of keys.
func writeParamRows(b *strings.Builder, params map[string]string, keys []string) {
	for _, k := range keys {
		b.WriteString(`<font color="LEVEL">[` + k + `]</font> ` + params[k] + "<br1>")
	}
}

// npcSkillContent is //info skill of an NPC of template t: each typed
// skill of the template under its type, by type, a skill whose effect is
// not done greyed.
func (l *GameClientLink) npcSkillContent(t *npc.Template) string {
	if len(t.SkillsByType) == 0 {
		return "This NPC doesn't hold any skill."
	}
	var b strings.Builder
	for _, typ := range slices.Sorted(maps.Keys(t.SkillsByType)) {
		ref := t.SkillsByType[typ]
		def, _ := l.skills.Definition(ref)
		name := def.Name
		if def.SkillType == "NOTDONE" {
			name = `<font color="777777">` + name + "</font>"
		}
		b.WriteString(`<br><font color="LEVEL">` + typ.String() + "</font><br1>")
		b.WriteString(name + " [" + strconv.Itoa(int(ref.ID)) + "-" + strconv.Itoa(ref.Level) + "]<br1>")
	}
	return b.String()
}

// npcShopContent is //info shop of the NPC inst: the tax rate of its
// castle, when it has one, and a link to each buylist its template sells
// from.
func (l *GameClientLink) npcShopContent(inst *npc.Instance) string {
	lists, total := l.merchant.ListsForNPC(inst.Template.ID)
	if len(lists) == 0 {
		return "This NPC doesn't hold any buyList."
	}
	var b strings.Builder
	if c, ok := l.npcCastle(inst); ok {
		b.WriteString("Tax rate: " + strconv.Itoa(c.CurrentTaxPercent()) + "%<br>")
	}
	b.WriteString(`<table width="100%">`)
	for _, list := range buyListOrder(lists, total) {
		id := strconv.Itoa(list.ID)
		b.WriteString(`<tr><td><a action="bypass -h admin_buy ` + id + ` 1">Buylist id: ` + id + "</a></td></tr>")
	}
	b.WriteString("</table>")
	return b.String()
}

// buyListOrder orders lists, given in load order, as the hash table
// holding total lists by id iterates them: by slot, the id's hash modulo
// the table size, then in load order within a slot. The table starts at 16
// slots and doubles whenever it holds more than three quarters of them.
func buyListOrder(lists []buylist.List, total int) []buylist.List {
	size := 16
	for total > size*3/4 {
		size *= 2
	}
	slot := func(id int) int {
		h := uint32(int32(id))
		return int((h ^ h>>16) & uint32(size-1))
	}
	out := slices.Clone(lists)
	slices.SortStableFunc(out, func(a, b buylist.List) int { return cmp.Compare(slot(a.ID), slot(b.ID)) })
	return out
}

// npcSpawnOf returns the spawn of the NPC obj, false when no spawn placed
// it. A decoration is its own standalone spawn, at the point it stands on.
func (l *GameClientLink) npcSpawnOf(obj world.Tracked) (manager.SpawnRecord, bool) {
	if d, ok := obj.(*npc.Decoration); ok {
		x, y, z := d.Position()
		return manager.SpawnRecord{
			Fixed: true, At: location.Location{X: x, Y: y, Z: z}, Heading: d.Heading(),
			Entry: spawn.Entry{NPCID: int32(d.Template.ID), RespawnDelay: d.RespawnDelay},
		}, true
	}
	npcs := l.npcSpawns.Load()
	if npcs == nil {
		return manager.SpawnRecord{}, false
	}
	return npcs.SpawnOf(obj.ObjectID())
}

// npcSpawnPoint is where rec, the spawn of the NPC obj that no maker
// declares, places its NPC.
func (l *GameClientLink) npcSpawnPoint(obj world.Tracked, rec manager.SpawnRecord) (location.Location, int) {
	if _, ok := obj.(*npc.Decoration); ok {
		return rec.At, rec.Heading
	}
	if npcs := l.npcSpawns.Load(); npcs != nil {
		if at, heading, ok := npcs.SpawnPoint(obj.ObjectID()); ok {
			return at, heading
		}
	}
	return rec.At, rec.Heading
}

// npcSpawnPage is npcinfo/spawn.htm for the NPC obj, the instance inst,
// as gm sees it: where it stands and how far from gm, its spawn, its
// respawn delays and privates, and its place in a master's party.
func (l *GameClientLink) npcSpawnPage(gm *livePlayer, obj world.Tracked, inst *npc.Instance) string {
	at, _ := obj.(positioned)
	x, y, z := at.Position()
	here := location.Location{X: x, Y: y, Z: z}
	gx, gy, gz := gm.Position()
	pairs := []string{
		"%loc%", strconv.Itoa(x) + " " + strconv.Itoa(y) + " " + strconv.Itoa(z),
		"%dist%", strconv.Itoa(int(distance3D(location.Location{X: gx, Y: gy, Z: gz}, here))),
		"%corpse%", spawnTimeStamp(inst.Template.CorpseTime),
	}
	rec, ok := l.npcSpawnOf(obj)
	if !ok {
		for _, key := range []string{"%spawn%", "%spawndesc%", "%spawninfo%", "%loc2d%", "%loc3d%", "%resp%", "%rand_resp%", "%privates%"} {
			pairs = append(pairs, key, npcNoSpawnField)
		}
	} else {
		label, desc := spawnDescription(rec, inst.Template.ID)
		var info string
		if rec.Maker == "" {
			info = "loc: " + spawnPoint(l.npcSpawnPoint(obj, rec))
		} else {
			desc = `<a action="bypass -h admin_maker ` + rec.Maker + `">` + desc + "</a>"
			switch pos := rec.Entry.Positions; len(pos) {
			case 0:
				info = "loc: anywhere"
			case 1:
				p := pos[0].Location
				info = "loc: fixed " + strconv.Itoa(p.X) + ", " + strconv.Itoa(p.Y) + ", " + strconv.Itoa(p.Z)
			default:
				info = "loc: fixed random 1 of " + strconv.Itoa(len(pos))
			}
		}
		home := here
		if inst.HasHome {
			home = inst.Home
		}
		delay := max(rec.Entry.RespawnDelay, 0)
		random := min(max(rec.Entry.RespawnRandom, 0), delay)
		pairs = append(pairs,
			"%spawn%", label,
			"%spawndesc%", desc,
			"%spawninfo%", info,
			"%loc2d%", strconv.Itoa(int(distance2D(here, home))),
			"%loc3d%", strconv.Itoa(int(distance3D(here, home))),
			"%resp%", spawnTimeStamp(int(delay/time.Second)),
			"%rand_resp%", spawnTimeStamp(int(random/time.Second)),
			"%privates%", strconv.FormatBool(len(rec.Entry.Privates) > 0),
		)
	}
	pairs = append(pairs, "%minion%", npcPartyLine(obj))
	return fillPage(l.adminHTML("npcinfo/spawn.htm"), pairs...)
}

// npcPartyLine tells whether the NPC obj leads privates, is one, or
// neither, with how many privates its party holds.
func npcPartyLine(obj world.Tracked) string {
	m, ok := obj.(npc.PartyMember)
	switch {
	case ok && m.IsMaster():
		return "I'm a master, holding " + strconv.Itoa(len(m.Minions())) + " crappy minions."
	case ok && m.Master() != nil:
		master := m.Master()
		return "I'm a crappy minion, my master <font color=LEVEL>" + master.CharacterName() + "</font> holds " + strconv.Itoa(len(master.Minions())) + " minions."
	}
	return "I'm a regular NPC, as the most random BR/RU leaker."
}

// spawnDescription names rec, the spawn of an NPC of template npcID, and
// describes it: a maker's spawn by its maker, a private's by its master,
// a standalone one by its point.
func spawnDescription(rec manager.SpawnRecord, npcID int) (label, desc string) {
	id := strconv.Itoa(npcID)
	switch {
	case rec.Maker != "":
		return "MultiSpawn [id=" + id + "]", "NpcMaker: " + rec.Maker
	case rec.MasterID != 0:
		return "Spawn [id=" + id + "]", "Master: " + trimAndDress(rec.MasterName, 20) + " [objId=" + strconv.Itoa(int(rec.MasterID)) + "]"
	}
	return "Spawn [id=" + id + "]", "Location: " + spawnPoint(rec.At, rec.Heading)
}

// spawnPoint formats a spawn point as "x, y, z, heading".
func spawnPoint(at location.Location, heading int) string {
	return strconv.Itoa(at.X) + ", " + strconv.Itoa(at.Y) + ", " + strconv.Itoa(at.Z) + ", " + strconv.Itoa(heading)
}

// spawnTimeStamp formats secs as hours, minutes and seconds, each only
// when not zero ("1h 30m", " 5m 10s"), and " 0s" for no time at all.
func spawnTimeStamp(secs int) string {
	hours, minutes, seconds := secs/3600, secs%3600/60, secs%60
	out := ""
	if hours > 0 {
		out += strconv.Itoa(hours) + "h"
	}
	if minutes > 0 {
		out += " " + strconv.Itoa(minutes) + "m"
	}
	if seconds > 0 || out == "" {
		out += " " + strconv.Itoa(seconds) + "s"
	}
	return out
}

func distance2D(a, b location.Location) float64 {
	dx, dy := float64(a.X-b.X), float64(a.Y-b.Y)
	return math.Sqrt(dx*dx + dy*dy)
}

func distance3D(a, b location.Location) float64 {
	dx, dy, dz := float64(a.X-b.X), float64(a.Y-b.Y), float64(a.Z-b.Z)
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}
