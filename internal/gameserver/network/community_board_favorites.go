package network

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/bbs"
)

// boardFavorites runs a favorites command: _bbsgetfav shows the player's
// favorites; _bbsgetfav_add stores the home board as a favorite and
// _bbsgetfav_del_<id> deletes the player's favorite id, each then showing
// the favorites. Any other command shows the unknown-command page.
func (l *GameClientLink) boardFavorites(live *livePlayer, command string) {
	switch {
	case command == "_bbsgetfav":
		l.showFavorites(live)
	case strings.HasPrefix(command, "_bbsgetfav_add"):
		l.board.favorites.Add(live.ObjectID(), time.Now())
		l.showFavorites(live)
	case strings.HasPrefix(command, "_bbsgetfav_del"):
		tokens := bbs.Tokens(command, "_")
		if len(tokens) < 3 {
			return
		}
		id, ok := boardInt(tokens[2])
		if !ok {
			return
		}
		l.board.favorites.Delete(live.ObjectID(), id)
		l.showFavorites(live)
	default:
		l.sendBoard(live, bbs.NotImplemented(command))
	}
}

// showFavorites shows live's favorites, in the order they were added. A
// missing page, or a favorite that cannot be shown, shows nothing.
func (l *GameClientLink) showFavorites(live *livePlayer) {
	page, ok := l.boardPage(bbs.FavoritesPage)
	if !ok {
		return
	}
	favs := l.board.favorites.List(live.ObjectID())
	template := ""
	if len(favs) > 0 {
		if template, ok = l.boardPage(bbs.FavoriteTemplatePage); !ok {
			return
		}
	}
	html, ok := bbs.RenderFavorites(page, template, favs)
	if !ok {
		l.log.Debug().Int32("player", live.ObjectID()).Msg("community board: a stored favorite cannot be shown")
		return
	}
	l.sendBoard(live, html)
}
