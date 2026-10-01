package gameservertest

import (
	"context"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/merchant"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/rs/zerolog"
)

// merchantOptions are the merchant settings Boot wires.
type merchantOptions struct {
	lists []buylist.List
	cfg   *merchant.Config
	now   func() time.Time
	rows  []merchant.StockRow
}

// WithBuyLists loads lists as the server's buylists (default: none).
func WithBuyLists(lists ...buylist.List) Option {
	return func(o *options) { o.merchant.lists = append(o.merchant.lists, lists...) }
}

// WithMerchantConfig sets the server.properties shop settings (default:
// merchant.DefaultConfig).
func WithMerchantConfig(cfg merchant.Config) Option {
	return func(o *options) { o.merchant.cfg = &cfg }
}

// WithBuyListClock sets the clock the limited stock reads its restock
// times from (default: time.Now).
func WithBuyListClock(now func() time.Time) Option {
	return func(o *options) { o.merchant.now = now }
}

// WithBuyListRows saves rows into the buylists table before the stock
// restores from it, as a previous run would have left them.
func WithBuyListRows(rows ...merchant.StockRow) Option {
	return func(o *options) { o.merchant.rows = append(o.merchant.rows, rows...) }
}

// bootMerchant builds the merchant service as the production boot does:
// the stock restored from the buylists rows. The restock task is not
// started; a suite drives it with BuyListStock.Restock.
func bootMerchant(t *testing.T, o merchantOptions, store *gamesql.BuyListStore, worker *persist.Worker, ids merchant.IDs, log zerolog.Logger) (*merchant.Service, *merchant.Stock) {
	t.Helper()
	ctx := context.Background()
	for _, row := range o.rows {
		if err := store.SaveStock(ctx, row); err != nil {
			t.Fatalf("seed buylists row: %v", err)
		}
	}
	now := o.now
	if now == nil {
		now = time.Now
	}
	lists := buylist.NewTable(o.lists)
	stock := merchant.NewStock(lists, store, worker, now, log)
	if err := stock.Restore(ctx); err != nil {
		t.Fatalf("restore buylist stock: %v", err)
	}
	cfg := merchant.DefaultConfig()
	if o.cfg != nil {
		cfg = *o.cfg
	}
	return merchant.NewService(lists, stock, ids, cfg), stock
}
