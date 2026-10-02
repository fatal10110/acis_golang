package gameservertest

import (
	"cmp"
	"context"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/bbs"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
)

// restoreBoardForums restores the stored forums and favorites as the
// server's boot does, then gives each clan at level 2 or more, in
// ascending id order, the clan forums it lacks.
func restoreBoardForums(t *testing.T, forumStore *gamesql.ForumStore, favoriteStore *gamesql.FavoriteStore, forums *bbs.Forums, favorites *bbs.Favorites, clans *clan.Service) {
	t.Helper()
	forumRows, topics, posts, err := forumStore.Load(context.Background())
	if err != nil {
		t.Fatalf("load forums: %v", err)
	}
	forums.Restore(forumRows, topics, posts)
	all := clans.Table().Clans()
	slices.SortFunc(all, func(a, b *clan.Clan) int { return cmp.Compare(a.ID(), b.ID()) })
	for _, cl := range all {
		forums.EnsureClanForums(cl.ID(), cl.Level())
	}
	favs, err := favoriteStore.Load(context.Background())
	if err != nil {
		t.Fatalf("load favorites: %v", err)
	}
	favorites.Restore(favs)
}
