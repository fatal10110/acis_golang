package network

import (
	"context"
	"errors"
	"strconv"
	"strings"

	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/bookmark"
)

// bookmarkStore reads and writes the bookmarks rows of one game master.
type bookmarkStore interface {
	ListByOwner(ctx context.Context, ownerID int32) ([]bookmark.Bookmark, error)
	Insert(ctx context.Context, ownerID int32, m bookmark.Bookmark) error
	Delete(ctx context.Context, ownerID int32, name string) error
}

// adminBookmarkPageLimit is how many bookmarks a page of //bk shows.
const adminBookmarkPageLimit = 15

// adminBookmark answers //bk [name|page]: a name saves gm's position under
// it, then the first page of gm's bookmarks opens; digits open that page.
func (l *GameClientLink) adminBookmark(gm *livePlayer, line string) {
	page := int32(1)
	if args := spaceTokens(line)[1:]; len(args) > 0 {
		param := args[0]
		if isDigits(param) {
			n, ok := parseInt32(param)
			if !ok {
				l.log.Warn().Str("page", param).Msg("admin: //bk page out of range")
				return
			}
			page = n
		} else {
			x, y, z := gm.Position()
			m := bookmark.Bookmark{Name: param, X: int32(x), Y: int32(y), Z: int32(z)}
			switch err := l.bookmarkBook(gm).Add(m); {
			case errors.Is(err, bookmark.ErrNameTooLong):
				sendText(gm, "The bookmark name is too long.")
				return
			case errors.Is(err, bookmark.ErrExists):
				sendText(gm, "The bookmark name already exists.")
				return
			}
			if l.bookmarks != nil {
				l.queueRowWrite(gm.ObjectID(), "save bookmark", func(ctx context.Context, ownerID int32) error {
					return l.bookmarks.Insert(ctx, ownerID, m)
				})
			}
		}
	}
	l.sendBookmarks(gm, int(page))
}

// adminDeleteBookmark answers //delbk name: it deletes gm's bookmark so
// called, then opens the first page of gm's bookmarks.
func (l *GameClientLink) adminDeleteBookmark(gm *livePlayer, line string) {
	args := spaceTokens(line)[1:]
	if len(args) == 0 {
		sendText(gm, "The command delbk must be followed by a valid name.")
		return
	}
	name := args[0]
	if err := l.bookmarkBook(gm).Remove(name); err != nil {
		sendText(gm, "That bookmark doesn't exist.")
		return
	}
	if l.bookmarks != nil {
		l.queueRowWrite(gm.ObjectID(), "delete bookmark", func(ctx context.Context, ownerID int32) error {
			return l.bookmarks.Delete(ctx, ownerID, name)
		})
	}
	l.sendBookmarks(gm, 1)
}

// bookmarkBook returns gm's bookmark book, read from the store on first
// use. A failed read is logged and gm works on an empty book until the
// next command reads again.
func (l *GameClientLink) bookmarkBook(gm *livePlayer) *bookmark.Book {
	if gm.bookmarks != nil {
		return gm.bookmarks
	}
	if l.bookmarks == nil {
		gm.bookmarks = bookmark.NewBook(nil)
		return gm.bookmarks
	}
	ctx, cancel := context.WithTimeout(context.Background(), characterLookupTimeout)
	defer cancel()
	marks, err := l.bookmarks.ListByOwner(ctx, gm.ObjectID())
	if err != nil {
		l.log.Error().Err(err).Int32("object_id", gm.ObjectID()).Msg("admin: list bookmarks")
		return bookmark.NewBook(nil)
	}
	gm.bookmarks = bookmark.NewBook(marks)
	return gm.bookmarks
}

// sendBookmarks opens page of gm's bookmarks, fifteen to a page, each
// teleporting there or removing it.
func (l *GameClientLink) sendBookmarks(gm *livePlayer, page int) {
	marks := l.bookmarkBook(gm).All()
	pg, ok := handleradmin.Paginate(len(marks), page, adminBookmarkPageLimit)
	if !ok {
		// No page below the first exists; the command answers nothing.
		l.log.Warn().Int("page", page).Msg("admin: //bk page out of range")
		return
	}
	var b strings.Builder
	for row, m := range marks[pg.Start:pg.End] {
		if row%2 == 0 {
			b.WriteString("<table width=280 bgcolor=000000><tr>")
		} else {
			b.WriteString("<table width=280><tr>")
		}
		at := strconv.Itoa(int(m.X)) + " " + strconv.Itoa(int(m.Y)) + " " + strconv.Itoa(int(m.Z))
		b.WriteString(`<td width=230><a action="bypass -h admin_teleport ` + at + `">` + m.Name + " (" + at + `)</a></td>`)
		b.WriteString(`<td width=50><a action="bypass -h admin_delbk ` + m.Name + `">Remove</a></td>`)
		b.WriteString(`</tr></table><img src="L2UI.SquareGray" width=280 height=1>`)
	}
	pg.Space(&b, 20)
	pg.Links(&b, "bypass admin_bk %page%")
	html := strings.ReplaceAll(l.adminHTML("bk.htm"), "%content%", b.String())
	l.sendFilePage(gm, 0, adminFile("bk.htm"), html, 0)
}
