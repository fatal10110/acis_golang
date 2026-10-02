package bbs

import (
	"testing"
	"time"
)

// TestRenderFavoritesUnreadable pins the favorites board with a stored
// favorite that cannot be shown: no page at all, wherever it sits in the
// list, while a list of readable favorites renders each through the row
// template.
func TestRenderFavoritesUnreadable(t *testing.T) {
	const (
		page     = "LIST <?FAV_LIST?> END"
		template = "[<?fav_id?> <?arg_last?> <?bypass?>]"
	)
	at := time.Date(2026, 1, 5, 14, 7, 9, 0, time.Local)
	good := Favorite{ID: 1, PlayerID: 5, Title: "Mine", Bypass: "_bbsclan", Date: at}
	bad := Favorite{ID: 2, PlayerID: 5, Unreadable: true}

	if got, ok := RenderFavorites(page, template, []Favorite{good}); !ok || got != "LIST [1 Mine _bbsclan] END" {
		t.Fatalf("RenderFavorites(readable) = %q, %t", got, ok)
	}
	for _, favs := range [][]Favorite{{bad}, {good, bad}, {bad, good}} {
		if got, ok := RenderFavorites(page, template, favs); ok || got != "" {
			t.Fatalf("RenderFavorites(%+v) = %q, %t; want no page", favs, got, ok)
		}
	}
}
