package sql

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
)

// CastleStore reads and writes the castle table and the castle column of
// clan_data.
type CastleStore struct {
	db *sql.DB
}

// NewCastleStore returns a CastleStore backed by db.
func NewCastleStore(db *sql.DB) *CastleStore {
	return &CastleStore{db: db}
}

// Load returns every castle row, by id.
func (s *CastleStore) Load(ctx context.Context) ([]castle.Row, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, currentTaxPercent, nextTaxPercent, treasury, taxRevenue, seedIncome,
		siegeDate, regTimeOver, certificates FROM castle ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("load castles: %w", err)
	}
	var out []castle.Row
	for rows.Next() {
		var r castle.Row
		var regTimeOver string
		if err := rows.Scan(&r.ID, &r.CurrentTaxPercent, &r.NextTaxPercent, &r.Treasury, &r.TaxRevenue, &r.SeedIncome,
			&r.SiegeDate, &regTimeOver, &r.LeftCertificates); err != nil {
			rows.Close()
			return nil, fmt.Errorf("load castles: %w", err)
		}
		r.RegTimeOver = strings.EqualFold(regTimeOver, "true")
		out = append(out, r)
	}
	if err := closeRows(rows); err != nil {
		return nil, fmt.Errorf("load castles: %w", err)
	}
	return out, nil
}

// LoadOwners returns every clan_data row holding a castle, by clan id.
func (s *CastleStore) LoadOwners(ctx context.Context) ([]castle.Owner, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT clan_id, hasCastle FROM clan_data WHERE hasCastle > 0 ORDER BY clan_id`)
	if err != nil {
		return nil, fmt.Errorf("load castle owners: %w", err)
	}
	var out []castle.Owner
	for rows.Next() {
		var o castle.Owner
		if err := rows.Scan(&o.ClanID, &o.CastleID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("load castle owners: %w", err)
		}
		out = append(out, o)
	}
	if err := closeRows(rows); err != nil {
		return nil, fmt.Errorf("load castle owners: %w", err)
	}
	return out, nil
}

func (s *CastleStore) exec(ctx context.Context, what, query string, args ...any) error {
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	return nil
}

// UpdateTreasury stores castleID's treasury.
func (s *CastleStore) UpdateTreasury(ctx context.Context, castleID int32, treasury int64) error {
	return s.exec(ctx, "update castle treasury", `UPDATE castle SET treasury=? WHERE id=?`, treasury, castleID)
}

// UpdateTaxRevenue stores castleID's tax revenue.
func (s *CastleStore) UpdateTaxRevenue(ctx context.Context, castleID int32, revenue int64) error {
	return s.exec(ctx, "update castle tax revenue", `UPDATE castle SET taxRevenue=? WHERE id=?`, revenue, castleID)
}

// UpdateSeedIncome stores castleID's seed income.
func (s *CastleStore) UpdateSeedIncome(ctx context.Context, castleID int32, income int64) error {
	return s.exec(ctx, "update castle seed income", `UPDATE castle SET seedIncome=? WHERE id=?`, income, castleID)
}

// UpdateCertificates stores castleID's left certificates.
func (s *CastleStore) UpdateCertificates(ctx context.Context, castleID int32, n int) error {
	return s.exec(ctx, "update castle certificates", `UPDATE castle SET certificates=? WHERE id=?`, n, castleID)
}

// UpdateCurrentTax stores castleID's tax rate in force.
func (s *CastleStore) UpdateCurrentTax(ctx context.Context, castleID int32, percent int) error {
	return s.exec(ctx, "update castle current tax", `UPDATE castle SET currentTaxPercent=? WHERE id=?`, percent, castleID)
}

// UpdateNextTax stores castleID's next tax rate.
func (s *CastleStore) UpdateNextTax(ctx context.Context, castleID int32, percent int) error {
	return s.exec(ctx, "update castle next tax", `UPDATE castle SET nextTaxPercent=? WHERE id=?`, percent, castleID)
}

// UpdateFinances stores castleID's money and both tax rates.
func (s *CastleStore) UpdateFinances(ctx context.Context, castleID int32, f castle.Finances) error {
	return s.exec(ctx, "update castle finances",
		`UPDATE castle SET treasury=?, taxRevenue=?, seedIncome=?, currentTaxPercent=?, nextTaxPercent=? WHERE id=?`,
		f.Treasury, f.TaxRevenue, f.SeedIncome, f.CurrentTaxPercent, f.NextTaxPercent, castleID)
}

// UpdateSiegeInfo stores castleID's siege date and whether it is no longer
// open to change, as the column's 'true' or 'false'.
func (s *CastleStore) UpdateSiegeInfo(ctx context.Context, castleID int32, date int64, regTimeOver bool) error {
	return s.exec(ctx, "update castle siege info", `UPDATE castle SET siegeDate=?, regTimeOver=? WHERE id=?`,
		date, strconv.FormatBool(regTimeOver), castleID)
}

// UpdateOwner clears castleID from every clan_data row holding it, then
// gives it to clanID's row.
func (s *CastleStore) UpdateOwner(ctx context.Context, castleID, clanID int32) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("update castle owner: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE clan_data SET hasCastle=0 WHERE hasCastle=?`, castleID); err != nil {
		return fmt.Errorf("update castle owner: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE clan_data SET hasCastle=? WHERE clan_id=?`, castleID, clanID); err != nil {
		return fmt.Errorf("update castle owner: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("update castle owner: %w", err)
	}
	return nil
}

// UnequipCirclets moves ownerID's equipped circletID and Lord's Crown
// (6841) back to its inventory.
func (s *CastleStore) UnequipCirclets(ctx context.Context, circletID, ownerID int32) error {
	return s.exec(ctx, "unequip castle circlets",
		`UPDATE items SET loc='INVENTORY' WHERE item_id IN (?,6841) AND owner_id=? AND loc='PAPERDOLL'`, circletID, ownerID)
}
