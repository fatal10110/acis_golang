package bbs

import (
	"cmp"
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// The favorites board's pages, under the board's page folder.
const (
	FavoritesPage        = "favorite/favorite-get.htm"
	FavoriteTemplatePage = "favorite/template.htm"
)

// The favorite a favorites add stores: the add button records no page of
// its own yet, so every add stores this one, the home board.
const (
	addedFavoriteTitle  = "Testing favorites"
	addedFavoriteBypass = "_bbshome"
)

// Favorite is a page a character keeps on its favorites board. Unreadable
// marks a stored row whose title, bypass or date is missing: the
// favorites board of its owner then cannot be shown.
type Favorite struct {
	ID         int32
	PlayerID   int32
	Title      string
	Bypass     string
	Date       time.Time
	Unreadable bool
}

// FavoriteStore writes the bbs_favorite rows.
type FavoriteStore interface {
	InsertFavorite(ctx context.Context, f Favorite) error
	DeleteFavorite(ctx context.Context, id int32) error
}

// Favorites holds every character's favorites. mu guards byPlayer and
// lastID. Rows are written through writes on their owner's lane.
type Favorites struct {
	mu       sync.Mutex
	byPlayer map[int32][]Favorite
	lastID   int32

	store  FavoriteStore
	writes Writer
	log    zerolog.Logger
}

// NewFavorites returns empty favorites writing through store on writes.
func NewFavorites(store FavoriteStore, writes Writer, log zerolog.Logger) *Favorites {
	return &Favorites{byPlayer: map[int32][]Favorite{}, store: store, writes: writes, log: log}
}

// Restore files rows, the stored favorites, once at boot before any
// player connects. A new favorite's id follows the highest stored one.
func (f *Favorites) Restore(rows []Favorite) {
	rows = slices.Clone(rows)
	slices.SortStableFunc(rows, func(a, b Favorite) int { return cmp.Compare(a.ID, b.ID) })
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range rows {
		f.byPlayer[r.PlayerID] = append(f.byPlayer[r.PlayerID], r)
		f.lastID = max(f.lastID, r.ID)
	}
}

// List returns playerID's favorites in ascending id order.
func (f *Favorites) List(playerID int32) []Favorite {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.byPlayer[playerID])
}

// favoriteLimit is how many favorites a character keeps: the most added
// favorites the shipped favorites board can show, since with 22 rows the
// page is too long for the board window and shows nothing. The reference
// has no limit, so one player could grow the favorite table without bound
// (#3262).
const favoriteLimit = 21

// Add stores the home board as a new favorite of playerID, dated now. A
// character keeping favoriteLimit favorites gets no new one.
func (f *Favorites) Add(playerID int32, now time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.byPlayer[playerID]) >= favoriteLimit {
		return
	}
	f.lastID++
	fav := Favorite{ID: f.lastID, PlayerID: playerID, Title: addedFavoriteTitle, Bypass: addedFavoriteBypass, Date: now}
	f.byPlayer[playerID] = append(f.byPlayer[playerID], fav)
	f.write(playerID, "insert favorite", func(ctx context.Context, st FavoriteStore) error { return st.InsertFavorite(ctx, fav) })
}

// Delete removes playerID's favorite id, and from the store; another
// character's favorite is left alone.
func (f *Favorites) Delete(playerID, id int32) {
	f.mu.Lock()
	defer f.mu.Unlock()
	list := f.byPlayer[playerID]
	i := slices.IndexFunc(list, func(fav Favorite) bool { return fav.ID == id })
	if i < 0 {
		return
	}
	f.byPlayer[playerID] = slices.Delete(list, i, i+1)
	f.write(playerID, "delete favorite", func(ctx context.Context, st FavoriteStore) error { return st.DeleteFavorite(ctx, id) })
}

// write runs fn against the store on playerID's lane; f.mu is held.
func (f *Favorites) write(playerID int32, what string, fn func(context.Context, FavoriteStore) error) {
	if f.store == nil {
		return
	}
	store, log := f.store, f.log
	job := func() {
		ctx, cancel := context.WithTimeout(context.Background(), forumWriteTimeout)
		defer cancel()
		if err := fn(ctx, store); err != nil {
			log.Error().Err(err).Int32("player_id", playerID).Msg("bbs: " + what)
		}
	}
	if f.writes == nil {
		job()
		return
	}
	if !f.writes.Enqueue(playerID, job) {
		log.Error().Int32("player_id", playerID).Msg("bbs: " + what + ": write dropped")
	}
}

// RenderFavorites fills page, the favorites board, with favs, each shown
// through template. It reports false when a favorite cannot be shown.
func RenderFavorites(page, template string, favs []Favorite) (string, bool) {
	var b strings.Builder
	for _, fav := range favs {
		if fav.Unreadable {
			return "", false
		}
		row := strings.ReplaceAll(template, "<?sDate?>", fav.Date.Local().Format(fullDateLayout))
		row = strings.ReplaceAll(row, "<?fav_id?>", strconv.Itoa(int(fav.ID)))
		row = strings.ReplaceAll(row, "<?bypass?>", fav.Bypass)
		row = strings.ReplaceAll(row, "<?arg_last?>", fav.Title)
		b.WriteString(row)
	}
	return strings.ReplaceAll(page, "<?FAV_LIST?>", b.String()), true
}
