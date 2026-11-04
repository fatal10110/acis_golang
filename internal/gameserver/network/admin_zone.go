package network

import (
	"strconv"
	"strings"

	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// drawingAnchorZ is the height a player's debug drawings are anchored at.
const drawingAnchorZ = -16000

// zoneDrawing names the drawing //zone show draws in.
const zoneDrawing = "ZONE"

const zoneShowInvalid = "Invalid parameter for //zone show."

// zoneFlagNames are the flag names the zone panel lists, by zone.Flag.
var zoneFlagNames = [zone.FlagCount]string{
	"PVP", "PEACE", "SIEGE", "MOTHER_TREE", "CLAN_HALL", "NO_LANDING", "WATER",
	"JAIL", "MONSTER_TRACK", "CASTLE", "SWAMP", "NO_SUMMON_FRIEND", "NO_STORE",
	"TOWN", "HQ", "DANGER_AREA", "CAST_ON_ARTIFACT", "NO_RESTART", "SCRIPT", "BOSS",
}

// adminZone answers //zone [show all|clear|<id>]. show all draws every zone
// near gm, show clear blanks the drawing, both then opening the zone panel;
// show <id> draws that zone alone. Any other argument, or none, opens the
// panel. A show without a known id or keyword is refused after the drawing
// was emptied, the client keeping what it shows until the next draw.
func (l *GameClientLink) adminZone(gm *livePlayer, line string) {
	args := handleradmin.Args(line)
	if len(args) == 0 || strings.ToLower(args[0]) != "show" {
		if !l.adminZonePanel(gm) {
			// The panel needs the restart point gm's position resolves
			// to; without one there is nothing to show.
			l.log.Warn().Str("player", gm.CharacterName()).Msg("admin: no restart point for the zone panel")
			gm.SendFrame(serverpackets.FrameActionFailed())
		}
		return
	}
	drawing := gm.drawing(zoneDrawing)
	drawing.Reset()
	if len(args) < 2 {
		sendText(gm, zoneShowInvalid)
		return
	}
	switch param := strings.ToLower(args[1]); param {
	case "all":
		pos := gm.CurrentLocation()
		for _, k := range l.zonesAround(pos.X, pos.Y) {
			drawZone(drawing, k)
		}
		sendDrawing(gm, drawing)
		l.adminZonePanelOrRefuse(gm)
	case "clear":
		sendDrawing(gm, drawing)
		l.adminZonePanelOrRefuse(gm)
	default:
		id, err := strconv.ParseInt(param, 10, 32)
		if err != nil || l.zones == nil {
			sendText(gm, zoneShowInvalid)
			return
		}
		k, ok := l.zones.ByID(int(id))
		if !ok {
			sendText(gm, zoneShowInvalid)
			return
		}
		drawZone(drawing, k)
		sendDrawing(gm, drawing)
	}
}

// adminZonePanelOrRefuse opens the zone panel after a show, which is
// refused instead when the panel has no restart point to show.
func (l *GameClientLink) adminZonePanelOrRefuse(gm *livePlayer) {
	if !l.adminZonePanel(gm) {
		sendText(gm, zoneShowInvalid)
	}
}

// adminZonePanel opens the zone panel: the zones gm stands in, its map
// region, its restart area and points, its position and the zone flags on
// it. It reports false, opening nothing, when no restart point applies to
// gm where it stands.
func (l *GameClientLink) adminZonePanel(gm *livePlayer) bool {
	if l.restarts == nil {
		return false
	}
	pos := gm.CurrentLocation()
	calculated, ok := l.restarts.CalculatedPoint(pos, gm.Race)
	if !ok {
		return false
	}
	_, inArea := l.restarts.AreaAt(pos)
	current := "N/A"
	if p, ok := l.restarts.PointAt(pos); ok {
		current = p.Name
	}
	var flags, ids strings.Builder
	if gm.zoneActor != nil {
		on := gm.zoneActor.ZoneFlags()
		for f := range zone.FlagCount {
			if on.Has(f) {
				flags.WriteString(zoneFlagNames[f] + "<br1>")
			}
		}
		if l.zones != nil {
			for _, k := range l.zones.At(pos.X, pos.Y) {
				if k.Core().Inside(gm.zoneActor) {
					ids.WriteString(strconv.Itoa(k.Core().ID()) + " ")
				}
			}
		}
	}
	const file = "data/html/admin/zone.htm"
	page, found := l.html.Get(file)
	if !found {
		page = "<html><body>My html is missing:<br>" + file + "</body></html>"
	}
	rx := (pos.X-world.MinX)/world.TileSize + world.TileXMin
	ry := (pos.Y-world.MinY)/world.TileSize + world.TileYMin
	page = strings.NewReplacer(
		"%GEOREGION%", strconv.Itoa(rx)+"_"+strconv.Itoa(ry),
		"%RA%", strconv.FormatBool(inArea),
		"%CALCULATED_RP%", calculated.Name,
		"%CURRENT_RP%", current,
		"%CURRENTLOC%", strconv.Itoa(pos.X)+", "+strconv.Itoa(pos.Y)+", "+strconv.Itoa(pos.Z),
		"%ZONES%", flags.String(),
		"%ZLIST%", ids.String(),
	).Replace(page)
	l.sendValidatedFile(gm, 0, file, page, 0)
	return true
}

// zonesAround returns the zones of the world region holding x, y and of
// the regions around it.
func (l *GameClientLink) zonesAround(x, y int) []zone.Kind {
	if l.zones == nil {
		return nil
	}
	return l.zones.Around(x, y)
}

// drawing returns live's debug drawing name, anchored where it entered the
// world.
func (live *livePlayer) drawing(name string) *serverpackets.Drawing {
	if d, ok := live.drawings[name]; ok {
		return d
	}
	if live.drawings == nil {
		live.drawings = map[string]*serverpackets.Drawing{}
	}
	a := live.drawingAnchor
	d := serverpackets.NewDrawing(name, int32(a.X), int32(a.Y), int32(a.Z))
	live.drawings[name] = d
	return d
}

// sendDrawing sends live every frame of d, in order.
func sendDrawing(live *livePlayer, d *serverpackets.Drawing) {
	for _, f := range d.Frames() {
		live.SendFrame(f)
	}
}

// drawZone adds k's outline to d, labelled with its kind and id.
func drawZone(d *serverpackets.Drawing, k zone.Kind) {
	d.AddZoneOutline(zoneKindLabel(k)+"["+strconv.Itoa(k.Core().ID())+"]", zone.Outline(k.Core().Form()))
}

// zoneKindLabel is the kind name a zone's outline is labelled with.
func zoneKindLabel(k zone.Kind) string {
	switch k.(type) {
	case *zone.Arena:
		return "ArenaZone"
	case *zone.Boss:
		return "BossZone"
	case *zone.CastleTeleport:
		return "CastleTeleportZone"
	case *zone.Castle:
		return "CastleZone"
	case *zone.ClanHall:
		return "ClanHallZone"
	case *zone.Damage:
		return "DamageZone"
	case *zone.DerbyTrack:
		return "DerbyTrackZone"
	case *zone.Effect:
		return "EffectZone"
	case *zone.Fishing:
		return "FishingZone"
	case *zone.HQ:
		return "HqZone"
	case *zone.Jail:
		return "JailZone"
	case *zone.MotherTree:
		return "MotherTreeZone"
	case *zone.NoLanding:
		return "NoLandingZone"
	case *zone.NoRestart:
		return "NoRestartZone"
	case *zone.NoStore:
		return "NoStoreZone"
	case *zone.NoSummonFriend:
		return "NoSummonFriendZone"
	case *zone.Olympiad:
		return "OlympiadStadiumZone"
	case *zone.Peace:
		return "PeaceZone"
	case *zone.Prayer:
		return "PrayerZone"
	case *zone.Script:
		return "ScriptZone"
	case *zone.Siege:
		return "SiegeZone"
	case *zone.Swamp:
		return "SwampZone"
	case *zone.Town:
		return "TownZone"
	case *zone.Water:
		return "WaterZone"
	default:
		return "Zone"
	}
}
