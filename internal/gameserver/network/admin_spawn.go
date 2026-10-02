package network

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// systemMessageApplicantInformationIncorrect answers a //spawn whose NPC
// cannot be placed.
const systemMessageApplicantInformationIncorrect = 12

// adminListSpawnsPageLimit is how many NPCs a page of //list_spawns shows.
const adminListSpawnsPageLimit = 8

// SetNpcSpawns hands the link the live NPC population the admin spawn
// commands place NPCs into and delete them from. It is set once at boot,
// after the population is built; until then those commands find no spawn.
func (l *GameClientLink) SetNpcSpawns(npcs *manager.Npcs) {
	l.npcSpawns.Store(npcs)
}

// adminSpawn answers //spawn <id|name> [respawn]: one NPC of the template
// with that id, or that name with underscores for spaces, placed at the
// position of gm's selection (gm itself without one) as a standalone spawn
// that never respawns. The respawn delay is read and checked, but such a
// spawn has no respawn for it to set. Missing or unreadable arguments open
// the spawn panel; a template that is unknown or cannot be placed answers
// that the information is incorrect.
func (l *GameClientLink) adminSpawn(gm *livePlayer, line string) {
	args := handleradmin.Args(line)
	if len(args) == 0 {
		l.sendAdminFile(gm, "spawns.htm")
		return
	}
	if len(args) > 1 {
		if _, ok := parseJavaInt(args[1]); !ok {
			l.sendAdminFile(gm, "spawns.htm")
			return
		}
	}
	var tmpl *npc.Template
	ok := false
	if idOrName := args[0]; isDigits(idOrName) {
		id, valid := parseJavaInt(idOrName)
		if !valid {
			l.sendAdminFile(gm, "spawns.htm")
			return
		}
		if l.npcs != nil {
			tmpl, ok = l.npcs.Get(int(id))
		}
	} else if l.npcs != nil {
		tmpl, ok = l.npcs.GetByName(strings.ReplaceAll(idOrName, "_", " "))
	}
	if !ok || !l.placeAdminSpawn(tmpl, adminSpawnPoint(gm)) {
		gm.SendFrame(serverpackets.FrameSystemMessage(systemMessageApplicantInformationIncorrect))
		return
	}
	sendText(gm, "You spawned "+tmpl.Name+". - Cmd: admin_spawn")
}

// positioned is a world object with a position and a facing.
type positioned interface {
	Position() (x, y, z int)
	Heading() int
}

// adminSpawnPoint is where //spawn places its NPC: at gm's selection, else
// at gm, facing the same way.
func adminSpawnPoint(gm *livePlayer) positioned {
	if target, ok := gm.Target().(positioned); ok {
		return target
	}
	return gm
}

// placeAdminSpawn places one NPC of tmpl at at's position on the ground
// below it, facing at's heading. A decoration is placed on its own; every
// other NPC joins the live population as a standalone spawn. It reports
// false when the NPC cannot be placed.
func (l *GameClientLink) placeAdminSpawn(tmpl *npc.Template, at positioned) bool {
	x, y, z := at.Position()
	heading := at.Heading()
	if npc.InstanceKind(tmpl.Type) == npc.InstanceKind("ChristmasTree") {
		return l.ids != nil && l.placeDecoration(tmpl, tmpl.Title, x, y, z, heading)
	}
	npcs := l.npcSpawns.Load()
	if npcs == nil {
		return false
	}
	if err := npcs.SpawnFixed(tmpl, x, y, z, heading); err != nil {
		l.log.Warn().Err(err).Int("npc_id", tmpl.ID).Msg("admin: //spawn placed nothing")
		return false
	}
	return true
}

// adminDelete answers //delete: gm's selected NPC is removed with its
// spawn, never to respawn, when that spawn is a standalone one — placed by
// //spawn or an item. Any other selection is an invalid target.
func (l *GameClientLink) adminDelete(gm *livePlayer, _ string) {
	target := gm.Target()
	name := ""
	deleted := false
	switch t := target.(type) {
	case *npc.Decoration:
		l.world.Despawn(t)
		name, deleted = t.Name(), true
	case *npc.Hostile, *npc.Folk:
		if npcs := l.npcSpawns.Load(); npcs != nil && npcs.DeleteFixed(t.ObjectID()) {
			name, deleted = npcInstanceOf(t).Template.Name, true
		}
	}
	if !deleted {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
		return
	}
	sendText(gm, "You deleted "+name+".")
}

// adminListSpawns answers //list_spawns [id|name] [page]: a page of every
// NPC in the world of that template id or name (gm's selected NPC's without
// one), eight to a page, each a link that teleports gm to it, naming its
// spawn. An unknown name, id 0 or a selection that is no NPC is an invalid
// target. A page or id that does not read as an int, or a page below 1 when
// there are NPCs to list, ends the command with nothing sent.
func (l *GameClientLink) adminListSpawns(gm *livePlayer, line string) {
	args := handleradmin.Args(line)
	page := int32(1)
	if len(args) > 1 {
		p, ok := parseJavaInt(args[1])
		if !ok {
			return
		}
		page = p
	}
	var npcID int32
	switch {
	case len(args) == 0:
		inst := npcInstanceOf(gm.Target())
		if inst == nil {
			gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
			return
		}
		npcID = int32(inst.Template.ID)
	case isDigits(args[0]):
		id, ok := parseJavaInt(args[0])
		if !ok {
			return
		}
		npcID = id
	case l.npcs != nil:
		if tmpl, ok := l.npcs.GetByName(args[0]); ok {
			npcID = int32(tmpl.ID)
		}
	}
	if npcID == 0 {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
		return
	}

	var found []world.Tracked
	for _, obj := range l.world.Objects() {
		if inst := npcInstanceOf(obj); inst != nil && int32(inst.Template.ID) == npcID {
			found = append(found, obj)
		}
	}
	slices.SortFunc(found, func(a, b world.Tracked) int { return cmp.Compare(a.ObjectID(), b.ObjectID()) })
	pg, ok := handleradmin.Paginate(len(found), int(page), adminListSpawnsPageLimit)
	if !ok {
		l.log.Warn().Int32("page", page).Msg("admin: //list_spawns page out of range")
		return
	}

	var b strings.Builder
	b.WriteString("<html><body>")
	// Rows count from the page asked for, even past the last page shown.
	row := adminListSpawnsPageLimit * (page - 1)
	for _, obj := range found[pg.Start:pg.End] {
		if row%2 == 0 {
			b.WriteString("<table width=280 height=41 bgcolor=000000><tr>")
		} else {
			b.WriteString("<table width=280 height=41><tr>")
		}
		at := obj.(positioned)
		x, y, z := at.Position()
		fmt.Fprintf(&b, `<td><a action="bypass -h admin_teleport %d %d %d">%d`, x, y, z, row)
		if label, desc, ok := l.describeSpawn(obj, npcID); ok {
			b.WriteString(" - " + label + "</a><br1>" + desc)
		} else {
			fmt.Fprintf(&b, " - (%d, %d, %d, %d)</a>", x, y, z, at.Heading())
		}
		b.WriteString(`</td></tr></table><img src="L2UI.SquareGray" width=280 height=1>`)
		row++
	}
	pg.Space(&b, 42)
	pg.Links(&b, "bypass admin_list_spawns "+strconv.Itoa(int(npcID))+" %page%")
	b.WriteString("</body></html>")
	sendValidatedHTML(gm, 0, b.String(), 0)
}

// describeSpawn names the spawn of obj, an NPC of template npcID, and
// describes where it stands: a standalone spawn by its spawn point, a
// maker's by the maker, a private's by its master. ok is false for an NPC
// no spawn placed.
func (l *GameClientLink) describeSpawn(obj world.Tracked, npcID int32) (label, desc string, ok bool) {
	standalone := fmt.Sprintf("Spawn [id=%d]", npcID)
	if d, isDecoration := obj.(*npc.Decoration); isDecoration {
		x, y, z := d.Position()
		return standalone, fmt.Sprintf("Location: %d, %d, %d, %d", x, y, z, d.Heading()), true
	}
	npcs := l.npcSpawns.Load()
	if npcs == nil {
		return "", "", false
	}
	rec, ok := npcs.SpawnOf(obj.ObjectID())
	switch {
	case !ok:
		return "", "", false
	case rec.Fixed:
		return standalone, fmt.Sprintf("Location: %d, %d, %d, %d", rec.At.X, rec.At.Y, rec.At.Z, rec.Heading), true
	case rec.MasterID != 0:
		name := ""
		if master, found := l.world.Object(rec.MasterID); found {
			if inst := npcInstanceOf(master); inst != nil {
				name = inst.Template.Name
			}
		}
		return standalone, fmt.Sprintf("Master: %s [objId=%d]", trimAndDress(name, 20), rec.MasterID), true
	default:
		return fmt.Sprintf("MultiSpawn [id=%d]", npcID), "NpcMaker: " + rec.Maker, true
	}
}

// npcInstanceOf returns the NPC instance obj is, nil when obj is no NPC.
func npcInstanceOf(obj world.Tracked) *npc.Instance {
	switch o := obj.(type) {
	case *npc.Hostile:
		return o.Instance
	case *npc.Folk:
		return o.Instance
	case *npc.Decoration:
		return o.Instance
	case *npc.EffectPoint:
		return o.Instance
	}
	return nil
}

// trimAndDress cuts s longer than maxWidth characters to its first
// maxWidth-3, followed by "...".
func trimAndDress(s string, maxWidth int) string {
	r := []rune(s)
	if len(r) <= maxWidth {
		return s
	}
	return string(r[:maxWidth-3]) + "..."
}
