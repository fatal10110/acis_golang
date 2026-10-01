package network

import (
	"context"
	"time"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
)

// accessLevelStore stores a character's access level.
type accessLevelStore interface {
	SetAccessLevel(ctx context.Context, objectID int32, level int, title string) error
	SetAccessLevelByName(ctx context.Context, name string, level int) (bool, error)
}

// accessLevelWriteTimeout bounds one access-level write.
const accessLevelWriteTimeout = 5 * time.Second

// titleMaxLength is the longest title kept, in UTF-16 code units.
const titleMaxLength = 16

// accessLevel is the access level p plays under.
func (p *livePlayer) accessLevel() admin.AccessLevel {
	if a := p.access.Load(); a != nil {
		return *a
	}
	return admin.AccessLevel{}
}

// applyAccessAppearance gives c what playing under access at level shows: a
// level above 0 takes the access level's name as title, even a level the
// table does not define and so reads as the user level, and the access
// level colors the name and title.
func applyAccessAppearance(c *player.Character, level int, access admin.AccessLevel) {
	if level > 0 {
		c.SetTitle(trimTitle(access.Name))
	}
	c.SetColors(access.Colors())
}

// resolveAccessLevel returns the access level a character given level plays
// under, logging a level the table does not define, which reads as the
// user level, and the master level.
func (l *GameClientLink) resolveAccessLevel(c *player.Character, level int) admin.AccessLevel {
	if !l.admin.DefinesLevel(level) {
		l.log.Warn().Int("level", level).Str("player", c.Name).Msg("access level: undefined level granted, reset to the user level")
	}
	if level > 0 && level == l.admin.MasterLevel() {
		l.log.Info().Str("player", c.Name).Msg("access level: logged in with master access level")
	}
	return l.admin.Resolve(level)
}

// applyLoadedAccessLevel gives a character just read from its row the title
// and colors its stored access level shows, before CharSelected carries
// them.
func (l *GameClientLink) applyLoadedAccessLevel(c *player.Character) {
	applyAccessAppearance(c, c.AccessLevel, l.resolveAccessLevel(c, c.AccessLevel))
}

// setAccessLevel moves live, on live's queue, to the access level level
// reads as. The damage permission, title, colors and game-master roster
// follow, live and its observers are shown the change, and the level is
// stored.
func (l *GameClientLink) setAccessLevel(live *livePlayer, level int) {
	access := l.resolveAccessLevel(live.Character, level)
	live.access.Store(&access)
	live.SetCanGiveDamage(access.GiveDamage)
	applyAccessAppearance(live.Character, level, access)
	if access.IsGM {
		// A GM already listed keeps its hidden state.
		if !l.gms.Contains(live) {
			l.gms.Add(live, false)
		}
	} else {
		l.gms.Remove(live)
	}
	l.broadcastCharacterInfo(live)
	l.storeAccessLevel(live.ObjectID(), access.Level, live.Title())
}

// storeLeavingAccessLevel stores the level setAccessLevel would have stored
// for target, whose queue closed as it left the world and so dropped the
// change: the stored level must not depend on the target still playing.
func (l *GameClientLink) storeLeavingAccessLevel(target *livePlayer, level int) {
	access := l.resolveAccessLevel(target.Character, level)
	title := target.Title()
	if level > 0 {
		title = trimTitle(access.Name)
	}
	l.storeAccessLevel(target.ObjectID(), access.Level, title)
}

// storeAccessLevel writes objectID's access level, and the title the change
// left it, on its persistence lane, ahead of any save its logout queues
// after.
func (l *GameClientLink) storeAccessLevel(objectID int32, level int, title string) {
	if l.accessLevels == nil {
		return
	}
	store, log := l.accessLevels, l.log
	if !l.persist.Enqueue(objectID, func() {
		ctx, cancel := context.WithTimeout(context.Background(), accessLevelWriteTimeout)
		defer cancel()
		if err := store.SetAccessLevel(ctx, objectID, level, title); err != nil {
			log.Error().Err(err).Int32("object_id", objectID).Msg("store access level")
		}
	}) {
		log.Error().Int32("object_id", objectID).Msg("store access level: persistence closed")
	}
}

// storeAccessLevelByName writes the access level of the character named
// name, online or not, and reports whether one has that name. ok is false
// when the write failed; it is logged.
func (l *GameClientLink) storeAccessLevelByName(name string, level int) (found, ok bool) {
	if l.accessLevels == nil {
		return false, true
	}
	ctx, cancel := context.WithTimeout(context.Background(), accessLevelWriteTimeout)
	defer cancel()
	found, err := l.accessLevels.SetAccessLevelByName(ctx, name, level)
	if err != nil {
		l.log.Error().Err(err).Str("name", name).Msg("store access level")
		return false, false
	}
	return found, true
}

// sendAccountAccessLevel asks the login server to give account level; a
// request the link cannot send is dropped.
func (l *GameClientLink) sendAccountAccessLevel(account string, level int32) {
	if l.loginLink == nil {
		return
	}
	link := l.loginLink()
	if link == nil {
		return
	}
	if err := link.SendAccessLevelChange(account, level); err != nil {
		l.log.Debug().Err(err).Str("account", account).Msg("send account access level")
	}
}

// trimTitle cuts title to the longest title kept.
func trimTitle(title string) string {
	units := utf16.Encode([]rune(title))
	if len(units) <= titleMaxLength {
		return title
	}
	return string(utf16.Decode(units[:titleMaxLength]))
}
