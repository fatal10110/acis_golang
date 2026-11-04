package network

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

const (
	makerUsage = "Usage: //maker [delete|respawn|scripts|spawn|switch <name>]"
	// makerSpawnEvent is the maker script event a //maker spawn sends.
	makerSpawnEvent = "1001"
	// territoryTestPoints is how many points //terr test draws.
	territoryTestPoints = 10000
	// territoryDrawing is the debug drawing the territory commands draw.
	territoryDrawing = "TERR"
)

// adminMaker answers //maker: the npcmakers whose territory holds gm, a
// page of them by number, one maker's panel by name, or a subcommand on a
// named maker. A page number that shows no page answers nothing, as the
// command has always done; an admin page link leaves no client action
// pending.
func (l *GameClientLink) adminMaker(gm *livePlayer, line string) {
	npcs := l.npcSpawns.Load()
	if npcs == nil {
		l.log.Warn().Msg("admin: //maker without an npc population")
		gm.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	args := handleradmin.Args(line)
	switch len(args) {
	case 0:
		l.sendMakersPage(gm, npcs, 1)
		return
	case 1:
		if isDigits(args[0]) {
			if page, err := strconv.Atoi(args[0]); err == nil {
				l.sendMakersPage(gm, npcs, page)
			}
			return
		}
		m, ok := npcs.Maker(args[0])
		if !ok {
			sendText(gm, `Maker "`+args[0]+`" doesn't exist.`)
			return
		}
		l.sendMakerPage(gm, m)
		return
	}
	name := args[1]
	m, ok := npcs.Maker(name)
	if !ok {
		sendText(gm, `Maker "`+name+`" doesn't exist.`)
		return
	}
	switch args[0] {
	case "delete":
		npcs.DeleteMakerNPCs(m.Maker.Name)
	case "respawn":
		npcs.RespawnMakerNPCs(m.Maker.Name)
	case "scripts":
		l.sendMakerScriptsPage(gm, m)
		return
	case "spawn":
		npcs.MakerEvent(m.Maker.Name, makerSpawnEvent, 0, 0)
	case "switch":
	default:
		sendText(gm, makerUsage)
		return
	}
	if m, ok = npcs.Maker(m.Maker.Name); ok {
		l.sendMakerPage(gm, m)
	}
}

// adminTerritory answers //terr: the makers page, the territory drawing
// cleared, or gm sent into, sampled over or outlined in a named
// territory, each answering the makers page after. Any other single word,
// or an unknown subcommand on a territory, does nothing, as the command
// has always done; an admin page link leaves no client action pending.
func (l *GameClientLink) adminTerritory(gm *livePlayer, line string) {
	npcs := l.npcSpawns.Load()
	if npcs == nil {
		l.log.Warn().Msg("admin: //terr without an npc population")
		gm.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	args := handleradmin.Args(line)
	switch len(args) {
	case 0:
		l.sendMakersPage(gm, npcs, 1)
		return
	case 1:
		if args[0] == "visual_clear" {
			d := gm.drawing(territoryDrawing)
			d.Reset()
			sendDrawing(gm, d)
			l.sendMakersPage(gm, npcs, 1)
		}
		return
	}
	t, ok := npcs.Territory(args[1])
	if !ok {
		sendText(gm, `Territory "`+args[1]+`" doesn't exist.`)
		return
	}
	switch args[0] {
	case "goto":
		loc, ok := npcs.TerritoryLocation(t)
		if !ok {
			return
		}
		l.teleportLivePlayer(gm, loc, 0)
	case "test":
		d := gm.drawing(territoryDrawing)
		d.Reset()
		for range territoryTestPoints {
			loc, ok := npcs.TerritoryLocation(t)
			if !ok {
				return
			}
			d.AddPoint(serverpackets.PrimitivePoint{Color: serverpackets.PrimitiveGreen, X: int32(loc.X), Y: int32(loc.Y), Z: int32(loc.Z)})
		}
		sendDrawing(gm, d)
	case "visual":
		d := gm.drawing(territoryDrawing)
		d.Reset()
		drawTerritory(d, t, gm.CurrentLocation().Z)
		sendDrawing(gm, d)
	default:
		return
	}
	l.sendMakersPage(gm, npcs, 1)
}

// adminSpawnEvent answers //spawn_event <event>: the makers of event start
// and gm is told how many NPCs they then have.
func (l *GameClientLink) adminSpawnEvent(gm *livePlayer, line string) {
	l.eventNPCs(gm, line, "Usage: //spawn_event <event_name>", "Spawned", (*manager.Npcs).SpawnEventNPCs)
}

// adminDespawnEvent answers //despawn_event <event>: the NPCs of event's
// makers are deleted and gm is told how many.
func (l *GameClientLink) adminDespawnEvent(gm *livePlayer, line string) {
	l.eventNPCs(gm, line, "Usage: //despawn_event <event_name>", "Despawned", (*manager.Npcs).DespawnEventNPCs)
}

func (l *GameClientLink) eventNPCs(gm *livePlayer, line, usage, done string, run func(*manager.Npcs, string) int) {
	args := handleradmin.Args(line)
	if len(args) == 0 {
		sendText(gm, usage)
		return
	}
	total := 0
	if npcs := l.npcSpawns.Load(); npcs != nil {
		total = run(npcs, args[0])
	}
	sendText(gm, done+" "+strconv.Itoa(total)+` "`+args[0]+`" NPCs.`)
}

// drawTerritory outlines every triangle of t at height z in yellow, each
// side labelled with t's name.
func drawTerritory(d *serverpackets.Drawing, t *spawn.Territory, z int) {
	line := func(a, b [2]int) {
		d.AddLine(serverpackets.PrimitiveLine{
			PrimitivePoint: serverpackets.PrimitivePoint{Name: t.Name, Color: serverpackets.PrimitiveYellow, NameColored: true, X: int32(a[0]), Y: int32(a[1]), Z: int32(z)},
			X2:             int32(b[0]), Y2: int32(b[1]), Z2: int32(z),
		})
	}
	for _, tri := range t.Triangles() {
		c := tri.Corners()
		a, b, cc := [2]int{c[0].X, c[0].Y}, [2]int{c[1].X, c[1].Y}, [2]int{c[2].X, c[2].Y}
		line(a, b)
		line(a, cc)
		line(b, cc)
	}
}

// sendMakersPage sends gm page number of the territories of the npcmakers
// around it, one territory a page with links to go to, sample and outline
// it and to each of those makers' panels.
func (l *GameClientLink) sendMakersPage(gm *livePlayer, npcs *manager.Npcs, number int) {
	makers := npcs.MakersAt(gm.CurrentLocation())
	var territories []*spawn.Territory
	for _, m := range makers {
		if m.Territory != nil && !containsTerritory(territories, m.Territory) {
			territories = append(territories, m.Territory)
		}
	}
	pg, ok := handleradmin.Paginate(len(territories), number, 1)
	if !ok {
		return
	}
	var b strings.Builder
	for _, t := range territories[pg.Start:pg.End] {
		b.WriteString("<table><tr><td width=280><font color=LEVEL>" + t.Name + "</font></td></tr></table>")
		b.WriteString("<table><tr><td width=170>min=" + strconv.Itoa(t.MinZ) + ", max=" + strconv.Itoa(t.MaxZ) +
			`</td><td width=110><a action="bypass -h admin_terr goto ` + t.Name + `">Go To</a> | <a action="bypass -h admin_terr test ` + t.Name +
			`">Test</a> | <a action="bypass -h admin_terr visual ` + t.Name + `">Visualize</a></td></tr></table>`)
		row := 0
		for _, m := range makers {
			if m.Territory == nil || m.Territory.Name != t.Name {
				continue
			}
			if row%2 == 0 {
				b.WriteString("<table width=280 bgcolor=000000><tr>")
			} else {
				b.WriteString("<table width=280><tr>")
			}
			b.WriteString(`<td width=10></td><td width=270><a action="bypass -h admin_maker ` + m.Maker.Name + `">` + m.Maker.Name + "</a></td>")
			b.WriteString(`</tr></table><img src="L2UI.SquareGray" width=280 height=1>`)
			row++
		}
	}
	pg.Links(&b, "bypass admin_maker %page%")
	const file = "spawnlist/maker_check.htm"
	l.sendFilePage(gm, 0, adminFile(file), fillPage(l.adminHTML(file), "%TERRITORIESLIST%", b.String()), 0)
}

func containsTerritory(list []*spawn.Territory, t *spawn.Territory) bool {
	for _, have := range list {
		if have == t {
			return true
		}
	}
	return false
}

// sendMakerPage sends gm the panel of npcmaker m: its territories, maker
// script, event, NPC counts and spawn entries.
func (l *GameClientLink) sendMakerPage(gm *livePlayer, m manager.MakerState) {
	var list string
	if len(m.Spawns) == 0 {
		list = "<br><br><center>No NPCs at this NPC maker.</center>"
	} else {
		var b strings.Builder
		b.WriteString(`<table><tr><td width=35><font color="LEVEL">ID:</font></td><td width=130><font color="LEVEL">Name:</font></td><td width=35><font color="LEVEL">NPCs:</font></td><td width=70><font color="LEVEL">Resp / Rnd:</font></td></tr>`)
		for _, s := range m.Spawns {
			delay, random := makerRespawnTimes(s.Entry)
			b.WriteString("<tr><td>" + strconv.Itoa(int(s.Entry.NPCID)) + "</td><td>" + s.Name + "</td><td>" +
				strconv.Itoa(s.Spawned) + " / " + strconv.Itoa(s.Total) + "</td><td>" +
				spawnTimeStamp(int(delay/time.Second)) + " / " + spawnTimeStamp(int(random/time.Second)) + "</td></tr>")
		}
		b.WriteString("</table>")
		list = b.String()
	}
	page := fillPage(l.adminHTML("spawnlist/maker_info.htm"),
		"%TERRNAME%", territoryName(m.Territory),
		"%BANTERRNAME%", territoryName(m.Banned),
		"%MAKERNAME%", m.Maker.Name,
		"%MAKERSPAWN%", m.Script,
		"%MAKEREVENT%", m.Maker.EventName(),
		"%MAKERSTART%", strconv.FormatBool(m.Maker.OnStart()),
		"%MAKERRESPAWN%", m.Script,
		"%MAKERCNTA%", strconv.Itoa(m.Alive),
		"%MAKERCNTD%", strconv.Itoa(m.Decayed),
		"%MAKERCNTT%", strconv.Itoa(m.Maker.MaximumNPCs),
		"%NPCSLIST%", list,
	)
	l.sendFilePage(gm, 0, adminFile("spawnlist/maker_info.htm"), page, 0)
}

// sendMakerScriptsPage sends gm the scripts npcmaker m tells when its last
// NPC is killed. No script listens for that, so the list is always empty.
func (l *GameClientLink) sendMakerScriptsPage(gm *livePlayer, m manager.MakerState) {
	page := fillPage(l.adminHTML("spawnlist/maker_scripts.htm"),
		"%MAKERNAME%", m.Maker.Name,
		"%MAKERSPAWN%", m.Script,
		"%MAKEREVENT%", m.Maker.EventName(),
		"%MAKERSTART%", strconv.FormatBool(m.Maker.OnStart()),
		"%SCRIPTS%", "<br><br><center>This NPC maker isn't affected by scripts.</center>",
	)
	l.sendFilePage(gm, 0, adminFile("spawnlist/maker_scripts.htm"), page, 0)
}

func territoryName(t *spawn.Territory) string {
	if t == nil {
		return ""
	}
	return t.Name
}

// makerRespawnTimes are the respawn delay and random part an npcmaker's
// spawn entry e waits: the delay floored at zero, the random part floored
// at zero and capped at the declared delay.
func makerRespawnTimes(e spawn.Entry) (delay, random time.Duration) {
	return max(e.RespawnDelay, 0), min(max(e.RespawnRandom, 0), e.RespawnDelay)
}
