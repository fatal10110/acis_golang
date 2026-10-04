package network

import (
	"context"
	"errors"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// DataReloads re-read the data tables //reload names (AdminReload.java)
// and the spawn list //respawnall reloads. A hook that returns only an error
// reads its files and swaps the loaded table in place for every holder,
// keeping the table as it was when the files cannot be read; a nil hook
// leaves its table as booted, and //reload answers its type as not ported
// yet.
type DataReloads struct {
	// Admin reloads the access levels and admin commands
	// (AdminData.reload).
	Admin func() error
	// Crests reloads the crest images (CrestCache.reload).
	Crests func() error
	// HTML reloads the HTML pages (HtmCache.reload).
	HTML func() error
	// Multisells reloads the multisell lists (MultisellData.reload).
	Multisells func() error
	// NPCs reloads the NPC templates (NpcData.reload); NPCs already in the
	// world keep theirs.
	NPCs func() error
	// WalkerRoutes reloads the walker routes (WalkerRouteData.reload).
	WalkerRoutes func() error
	// Teleports reads the gatekeeper destinations, which the link's
	// gatekeeper then holds (TeleportData.reload and
	// InstantTeleportData.reload).
	Teleports func() (travel.TeleportTable, travel.InstantTable, error)
	// SpawnList saves current's database-tracked spawn rows, then reads the
	// spawn list and those rows anew, for //respawnall
	// (SpawnManager.reload's save and load).
	SpawnList func(ctx context.Context, current *manager.Spawns) (*manager.Spawns, error)
}

// errReloadNotPorted answers a //reload type whose table has no reload
// hook yet.
var errReloadNotPorted = errors.New("reload not ported yet")

// reloadNotPorted answers a type no reload exists for yet, each tracked by
// its issue: boat (#3404), buylist (#3405), config (#3406), cw (#3357), door
// (#3407), item (#3408), skill (#3409), zone (#3410), and script with the
// scripts half of npc (#3411).
func reloadNotPorted(*GameClientLink) error { return errReloadNotPorted }

// reloadHook runs hook, errReloadNotPorted without one.
func reloadHook(hook func() error) error {
	if hook == nil {
		return errReloadNotPorted
	}
	return hook()
}

// reloadType is one type //reload takes.
type reloadType struct {
	// word names the type: a token naming it starts with word, or equals
	// it when exact is set.
	word  string
	exact bool
	// done is what the game master is told once the type has reloaded.
	done string
	run  func(l *GameClientLink) error
}

// reloadTypes are the types //reload takes, in the order AdminReload.java
// tests a token against them: the first that matches reloads.
var reloadTypes = []reloadType{
	{word: "admin", done: "Admin data has been reloaded.", run: func(l *GameClientLink) error { return reloadHook(l.reloads.Admin) }},
	{word: "announcement", done: "The content of announcements.xml has been reloaded.", run: func(l *GameClientLink) error { return l.announcements.Load() }},
	{word: "boat", done: "Boat have been reloaded.", run: reloadNotPorted},
	{word: "buylist", done: "Buylists have been reloaded.", run: reloadNotPorted},
	{word: "config", done: "Configs files have been reloaded.", run: reloadNotPorted},
	{word: "crest", done: "Crests have been reloaded.", run: func(l *GameClientLink) error { return reloadHook(l.reloads.Crests) }},
	{word: "cw", done: "Cursed weapons have been reloaded.", run: reloadNotPorted},
	{word: "door", done: "Doors instance has been reloaded.", run: reloadNotPorted},
	{word: "htm", done: "The HTM cache has been reloaded.", run: func(l *GameClientLink) error { return reloadHook(l.reloads.HTML) }},
	{word: "item", done: "Items' templates have been reloaded.", run: reloadNotPorted},
	{word: "multisell", exact: true, done: "The multisell instance has been reloaded.", run: func(l *GameClientLink) error { return reloadHook(l.reloads.Multisells) }},
	{word: "npc", exact: true, done: "NPCs templates and Scripts have been reloaded.", run: (*GameClientLink).reloadNPCs},
	{word: "npcwalker", done: "Walking routes have been reloaded.", run: func(l *GameClientLink) error { return reloadHook(l.reloads.WalkerRoutes) }},
	{word: "script", exact: true, done: "Scripts have been reloaded.", run: reloadNotPorted},
	{word: "skill", done: "Skills' XMLs have been reloaded.", run: reloadNotPorted},
	{word: "teleport", done: "Teleport locations have been reloaded.", run: (*GameClientLink).reloadTeleports},
	{word: "zone", done: "Zones have been reloaded.", run: reloadNotPorted},
}

// reloadUsage is what //reload answers a type it does not take.
var reloadUsage = []string{
	"Usage : //reload <admin|announcement|buylist|config>",
	"Usage : //reload <crest|cw|door|htm|item|multisell|npc>",
	"Usage : //reload <npcwalker|script|skill|teleport|zone>",
}

// adminReload answers //reload <type>...: each type in turn reloads its data
// and tells gm so; a token naming no type answers the usage and the next
// one is read. Without a type, or when a reload fails, the usage is sent and
// the command ends there. A type whose reload is not ported yet is logged
// and answered with ActionFailed, as an unported command is.
func (l *GameClientLink) adminReload(gm *livePlayer, line string) {
	args := handleradmin.Args(line)
	if len(args) == 0 {
		sendReloadUsage(gm)
		return
	}
	for _, token := range args {
		typ, ok := reloadTypeOf(token)
		if !ok {
			sendReloadUsage(gm)
			continue
		}
		err := typ.run(l)
		switch {
		case errors.Is(err, errReloadNotPorted):
			l.log.Warn().Str("type", typ.word).Msg("admin: //reload type not implemented yet")
			gm.SendFrame(serverpackets.FrameActionFailed())
		case err != nil:
			l.log.Error().Err(err).Str("type", typ.word).Msg("admin: //reload failed; the data stays as it was")
			sendReloadUsage(gm)
			return
		default:
			sendText(gm, typ.done)
		}
	}
}

// reloadTypeOf returns the type token names.
func reloadTypeOf(token string) (reloadType, bool) {
	for _, typ := range reloadTypes {
		if token == typ.word || !typ.exact && strings.HasPrefix(token, typ.word) {
			return typ, true
		}
	}
	return reloadType{}, false
}

// sendReloadUsage sends live the //reload usage.
func sendReloadUsage(live *livePlayer) {
	for _, text := range reloadUsage {
		sendText(live, text)
	}
}

// reloadNPCs reloads the NPC templates: NpcData.reload. The reference then
// reloads the scripts (ScriptData.reload), which Go does not have yet
// (#3411).
func (l *GameClientLink) reloadNPCs() error {
	return reloadHook(l.reloads.NPCs)
}

// reloadTeleports reads the gatekeeper destinations anew and hands them to
// the gatekeeper.
func (l *GameClientLink) reloadTeleports() error {
	if l.reloads.Teleports == nil {
		return errReloadNotPorted
	}
	teleports, instants, err := l.reloads.Teleports()
	if err != nil {
		return err
	}
	l.gatekeeper.SetTables(teleports, instants)
	return nil
}
