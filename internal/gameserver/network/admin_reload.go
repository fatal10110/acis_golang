package network

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
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
	// BoatRoutes reads the boat itineraries anew, which then replace the
	// fleet's.
	BoatRoutes func() ([]route.BoatItinerary, error)
	// Config reads the game server properties files anew into the player
	// settings, which then replace the ones in force, all at once. Each read
	// of a setting after the swap sees the reloaded value, but some take it
	// later or not at all:
	//   - the settings a player or summon is built with (PlayerConfig's
	//     player.Rules fields and a summon's MaxBuffsAmount and
	//     RespawnRestoreHP) reach the players entering the world, and the
	//     summons called, after the reload;
	//   - Enchant, MagicFailures, CraftingDisabled and
	//     KeepMaintainedIngredients keep their boot value: the enchant,
	//     skill, craft and multisell services are built with them;
	//   - PartyRange and MultipleItemDrop keep their boot value: the kill
	//     rewards and the scripts read their own copies of both, so the
	//     reload leaves the ones in force for party loot too;
	//   - every setting outside PlayerConfig keeps its boot value; the
	//     composition root that builds this hook lists them.
	Config func() (PlayerConfig, error)
	// BuyLists reads the buylists anew, which the merchant then sells
	// from, its limited stock rebuilt from the saved rows
	// (BuyListManager.reload).
	BuyLists func() (*buylist.Table, error)
	// Crests reloads the crest images (CrestCache.reload).
	Crests func() error
	// CursedWeapons reads the cursed weapons anew (CursedWeaponManager.load),
	// which the link then takes in place of the ones it had.
	CursedWeapons func() (*entity.CursedWeaponTable, error)
	// Doors reads the doors anew, which then replace the doors in the
	// world (DoorData.reload's load).
	Doors func() (*door.Table, error)
	// HTML reloads the HTML pages (HtmCache.reload).
	HTML func() error
	// Items reloads the item templates. Tables that resolved or checked
	// item ids against the templates when they were read (multisells,
	// buylists, NPC drops) are not read again; each has its own reload.
	Items func() error
	// Multisells reloads the multisell lists (MultisellData.reload).
	Multisells func() error
	// NPCs reloads the NPC templates (NpcData.reload); NPCs already in the
	// world keep theirs, and so do the respawns of spawns already in place
	// (ASpawn's template is fixed when the spawn is built).
	NPCs func() error
	// Skills reloads the skill definitions. Tables that resolved or
	// checked skills against them when they were read (NPC templates,
	// cursed weapons, the scheme buffer) are not read again; NPC templates
	// and cursed weapons have their own reload.
	Skills func() error
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
	// Zones reads the zones anew, set up as at boot, which then replace the
	// ones the server holds (ZoneManager.reload's load).
	Zones func() (*zone.Index, error)
}

// errReloadNotPorted answers a //reload type whose table has no reload
// hook yet.
var errReloadNotPorted = errors.New("reload not ported yet")

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
	// runFor, when set, reloads in place of run for the game master gm,
	// on whose queue the command runs.
	runFor func(l *GameClientLink, gm *livePlayer) error
}

// reloadTypes are the types //reload takes, in the order AdminReload.java
// tests a token against them: the first that matches reloads.
var reloadTypes = []reloadType{
	{word: "admin", done: "Admin data has been reloaded.", run: func(l *GameClientLink) error { return reloadHook(l.reloads.Admin) }},
	{word: "announcement", done: "The content of announcements.xml has been reloaded.", run: func(l *GameClientLink) error { return l.announcements.Load() }},
	{word: "boat", done: "Boat have been reloaded.", runFor: (*GameClientLink).reloadBoats},
	{word: "buylist", done: "Buylists have been reloaded.", run: (*GameClientLink).reloadBuyLists},
	{word: "config", done: "Configs files have been reloaded.", run: (*GameClientLink).reloadConfig},
	{word: "crest", done: "Crests have been reloaded.", run: func(l *GameClientLink) error { return reloadHook(l.reloads.Crests) }},
	{word: "cw", done: "Cursed weapons have been reloaded.", run: (*GameClientLink).reloadCursedWeapons},
	{word: "door", done: "Doors instance has been reloaded.", run: (*GameClientLink).reloadDoors},
	{word: "htm", done: "The HTM cache has been reloaded.", run: func(l *GameClientLink) error { return reloadHook(l.reloads.HTML) }},
	{word: "item", done: "Items' templates have been reloaded.", run: func(l *GameClientLink) error { return reloadHook(l.reloads.Items) }},
	{word: "multisell", exact: true, done: "The multisell instance has been reloaded.", run: func(l *GameClientLink) error { return reloadHook(l.reloads.Multisells) }},
	{word: "npc", exact: true, done: "NPCs templates have been reloaded; scripts were not reloaded.", run: (*GameClientLink).reloadNPCs},
	{word: "npcwalker", done: "Walking routes have been reloaded.", run: func(l *GameClientLink) error { return reloadHook(l.reloads.WalkerRoutes) }},
	{word: "script", exact: true, done: "Script reload is not supported; scripts load once, at server start.", run: reloadNothing},
	{word: "skill", done: "Skills' XMLs have been reloaded.", run: func(l *GameClientLink) error { return reloadHook(l.reloads.Skills) }},
	{word: "teleport", done: "Teleport locations have been reloaded.", run: (*GameClientLink).reloadTeleports},
	{word: "zone", done: "Zones have been reloaded.", run: (*GameClientLink).reloadZones},
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
		var err error
		if typ.runFor != nil {
			err = typ.runFor(l, gm)
		} else {
			err = typ.run(l)
		}
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

// reloadNothing answers a type that is never reloaded: scripts are built
// once, at boot, and their registry never changes.
func reloadNothing(*GameClientLink) error { return nil }

// reloadNPCs reloads the NPC templates: NpcData.reload. Scripts are not
// reloaded with them; their bindings are keyed by NPC id, so the new
// templates keep them.
func (l *GameClientLink) reloadNPCs() error {
	return reloadHook(l.reloads.NPCs)
}

// reloadConfig reads the properties files anew and puts the player settings
// read in force in place of the ones before; a file that cannot be read
// leaves them as they were. PartyRange and MultipleItemDrop keep the value
// in force: the kill rewards and the scripts hold their own boot copies of
// both, and party loot must agree with them.
func (l *GameClientLink) reloadConfig() error {
	if l.reloads.Config == nil {
		return errReloadNotPorted
	}
	settings, err := l.reloads.Config()
	if err != nil {
		return err
	}
	prev := l.playerConfig()
	settings.PartyRange, settings.MultipleItemDrop = prev.PartyRange, prev.MultipleItemDrop
	l.playerSettings.Store(&settings)
	return nil
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

// reloadBuyListsTimeout bounds the stock rows read of //reload buylist.
const reloadBuyListsTimeout = 30 * time.Second

// reloadBuyLists reads the buylists anew and hands them to the merchant,
// which rebuilds their limited stock from the saved rows.
func (l *GameClientLink) reloadBuyLists() error {
	if l.reloads.BuyLists == nil || l.merchant == nil {
		return errReloadNotPorted
	}
	lists, err := l.reloads.BuyLists()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), reloadBuyListsTimeout)
	defer cancel()
	return l.merchant.Reload(ctx, lists)
}

// reloadDoors reads the doors anew and puts them in the world in place of
// the ones standing there.
func (l *GameClientLink) reloadDoors() error {
	if l.reloads.Doors == nil || l.doors == nil {
		return errReloadNotPorted
	}
	doors, err := l.reloads.Doors()
	if err != nil {
		return err
	}
	return l.doors.ReloadDoors(doors)
}
