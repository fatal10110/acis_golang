package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
)

// ClanHallStore reads and writes the clanhall and auctions rows and the
// clans' auction_bid_at.
type ClanHallStore struct {
	db *sql.DB
}

var _ clanhall.HallStore = (*ClanHallStore)(nil)

// NewClanHallStore returns a ClanHallStore backed by db.
func NewClanHallStore(db *sql.DB) *ClanHallStore {
	return &ClanHallStore{db: db}
}

// LoadHalls returns every clanhall row, by id.
func (s *ClanHallStore) LoadHalls(ctx context.Context) ([]clanhall.HallRow, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, ownerId, paidUntil, paid, sellerBid, sellerName, sellerClanName, endDate FROM clanhall ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("load clan halls: %w", err)
	}
	var out []clanhall.HallRow
	for rows.Next() {
		var r clanhall.HallRow
		var paid int
		if err := rows.Scan(&r.ID, &r.OwnerID, &r.PaidUntil, &paid, &r.SellerBid, &r.SellerName, &r.SellerClanName, &r.EndDate); err != nil {
			rows.Close()
			return nil, fmt.Errorf("load clan halls: %w", err)
		}
		r.Paid = paid != 0
		out = append(out, r)
	}
	if err := closeRows(rows); err != nil {
		return nil, fmt.Errorf("load clan halls: %w", err)
	}
	return out, nil
}

// LoadBids returns every auctions row, each hall's highest bid first.
func (s *ClanHallStore) LoadBids(ctx context.Context) ([]clanhall.BidRow, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT clanhall_id, clan_oid, bidder_name, clan_name, max_bid, time_bid FROM auctions ORDER BY clanhall_id, max_bid DESC, clan_oid")
	if err != nil {
		return nil, fmt.Errorf("load clan hall bids: %w", err)
	}
	var out []clanhall.BidRow
	for rows.Next() {
		var r clanhall.BidRow
		if err := rows.Scan(&r.HallID, &r.ClanID, &r.Name, &r.ClanName, &r.Bid, &r.Time); err != nil {
			rows.Close()
			return nil, fmt.Errorf("load clan hall bids: %w", err)
		}
		out = append(out, r)
	}
	if err := closeRows(rows); err != nil {
		return nil, fmt.Errorf("load clan hall bids: %w", err)
	}
	return out, nil
}

// LoadClanBids returns the hall each clan bid on, for the clans that did.
func (s *ClanHallStore) LoadClanBids(ctx context.Context) ([]clanhall.ClanBid, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT clan_id, auction_bid_at FROM clan_data WHERE auction_bid_at > 0")
	if err != nil {
		return nil, fmt.Errorf("load clan auction bids: %w", err)
	}
	var out []clanhall.ClanBid
	for rows.Next() {
		var r clanhall.ClanBid
		if err := rows.Scan(&r.ClanID, &r.HallID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("load clan auction bids: %w", err)
		}
		out = append(out, r)
	}
	if err := closeRows(rows); err != nil {
		return nil, fmt.Errorf("load clan auction bids: %w", err)
	}
	return out, nil
}

// UpdateHall writes every column of row.
func (s *ClanHallStore) UpdateHall(ctx context.Context, r clanhall.HallRow) error {
	paid := 0
	if r.Paid {
		paid = 1
	}
	if _, err := s.db.ExecContext(ctx,
		"UPDATE clanhall SET ownerId = ?, paidUntil = ?, paid = ?, sellerBid = ?, sellerName = ?, sellerClanName = ?, endDate = ? WHERE id = ?",
		r.OwnerID, r.PaidUntil, paid, r.SellerBid, r.SellerName, r.SellerClanName, r.EndDate, r.ID,
	); err != nil {
		return fmt.Errorf("store clan hall %d: %w", r.ID, err)
	}
	return nil
}

// UpdateEndDate writes hall hallID's auction end.
func (s *ClanHallStore) UpdateEndDate(ctx context.Context, hallID int32, endDate int64) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE clanhall SET endDate = ? WHERE id = ?", endDate, hallID); err != nil {
		return fmt.Errorf("store clan hall %d auction end: %w", hallID, err)
	}
	return nil
}

// UpdateSale writes hall hallID's registered sale and auction end.
func (s *ClanHallStore) UpdateSale(ctx context.Context, hallID int32, seller clanhall.Seller, endDate int64) error {
	if _, err := s.db.ExecContext(ctx,
		"UPDATE clanhall SET sellerBid = ?, sellerName = ?, sellerClanName = ?, endDate = ? WHERE id = ?",
		seller.Bid, seller.Name, seller.ClanName, endDate, hallID,
	); err != nil {
		return fmt.Errorf("store clan hall %d sale: %w", hallID, err)
	}
	return nil
}

// SaveBid stores r, replacing the bidder name, bid and time of the clan's
// earlier bid on the hall.
func (s *ClanHallStore) SaveBid(ctx context.Context, r clanhall.BidRow) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO auctions (clanhall_id, bidder_name, clan_oid, clan_name, max_bid, time_bid) VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE bidder_name = VALUES(bidder_name), max_bid = VALUES(max_bid), time_bid = VALUES(time_bid)`,
		r.HallID, r.Name, r.ClanID, r.ClanName, r.Bid, r.Time,
	); err != nil {
		return fmt.Errorf("store clan %d bid on clan hall %d: %w", r.ClanID, r.HallID, err)
	}
	return nil
}

// DeleteBids drops every bid on hall hallID.
func (s *ClanHallStore) DeleteBids(ctx context.Context, hallID int32) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM auctions WHERE clanhall_id = ?", hallID); err != nil {
		return fmt.Errorf("remove clan hall %d bids: %w", hallID, err)
	}
	return nil
}

// DeleteBid drops clan clanID's bid on hall hallID.
func (s *ClanHallStore) DeleteBid(ctx context.Context, hallID, clanID int32) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM auctions WHERE clanhall_id = ? AND clan_oid = ?", hallID, clanID); err != nil {
		return fmt.Errorf("remove clan %d bid on clan hall %d: %w", clanID, hallID, err)
	}
	return nil
}

// SetClanBid writes the hall clan clanID bid on, 0 for none.
func (s *ClanHallStore) SetClanBid(ctx context.Context, clanID, hallID int32) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE clan_data SET auction_bid_at = ? WHERE clan_id = ?", hallID, clanID); err != nil {
		return fmt.Errorf("store clan %d auction bid: %w", clanID, err)
	}
	return nil
}
