package gameservertest

import (
	"context"
	"database/sql"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

type scriptOptions struct {
	list    []script.Listing
	catalog script.Catalog
}

// WithScripts boots the script registry from list and catalog, as the
// production boot builds it from scripts.xml and its catalogs; without it
// the registry is empty. Every NPC id reads as having no template, so a
// script's NPC bindings are dropped: the scripts serve the quest journal.
func WithScripts(list []script.Listing, catalog script.Catalog) Option {
	return func(o *options) { o.scripts = &scriptOptions{list: list, catalog: catalog} }
}

// WithQuestLoadFault makes every quest journal read at a character
// selection fail with err.
func WithQuestLoadFault(err error) Option {
	return func(o *options) { o.questLoadErr = err }
}

// bootQuests returns the quest journal store and the script registry the
// link reads.
func bootQuests(db *sql.DB, o *options) (questStore, *script.Registry) {
	var list []script.Listing
	var catalog script.Catalog
	if o.scripts != nil {
		list, catalog = o.scripts.list, o.scripts.catalog
	}
	noTemplate := func(int32) (script.NPCKind, bool) { return script.KindOther, false }
	registry := script.Build(list, catalog, script.Config{KindOf: noTemplate, Log: o.log})
	var store questStore = gamesql.NewQuestStore(db)
	if o.questLoadErr != nil {
		store = failingQuestStore{err: o.questLoadErr}
	}
	return store, registry
}

// questStore is the journal store the link reads.
type questStore interface {
	ListByOwner(ctx context.Context, ownerID int32) ([]questlog.Row, error)
}

// failingQuestStore fails every read (WithQuestLoadFault).
type failingQuestStore struct{ err error }

func (s failingQuestStore) ListByOwner(context.Context, int32) ([]questlog.Row, error) {
	return nil, s.err
}
