package gameservertest

import (
	"context"
	"database/sql"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
)

// hallSiegeOptions are the hall siege settings: the configuration
// (WithHallSiegeConfig) and the scripts standing in for the hall siege
// scripts (WithHallSiegeScript).
type hallSiegeOptions struct {
	cfg     *clanhall.SiegeConfig
	scripts []hallSiegeScript
}

type hallSiegeScript struct {
	hallID int32
	script clanhall.SiegeScript
}

// WithHallSiegeScript makes script run siegable hall hallID's siege from
// boot, as the hall's siege script does.
func WithHallSiegeScript(hallID int32, script clanhall.SiegeScript) Option {
	return func(o *options) {
		o.hallSieges.scripts = append(o.hallSieges.scripts, hallSiegeScript{hallID: hallID, script: script})
	}
}

// WithHallSiegeConfig runs the hall sieges with cfg rather than the
// shipped defaults.
func WithHallSiegeConfig(cfg clanhall.SiegeConfig) Option {
	return func(o *options) { o.hallSieges.cfg = &cfg }
}

// restoreHallSieges loads the stored hall siege attackers and Rainbow
// Springs decrees, as the boot path does once the halls are restored.
func restoreHallSieges(t *testing.T, db *sql.DB, halls *clanhall.Halls) {
	t.Helper()
	if err := halls.RestoreSieges(context.Background(), gamesql.NewClanHallStore(db)); err != nil {
		t.Fatalf("restore clan hall sieges: %v", err)
	}
}

// startHallSieges binds the WithHallSiegeScript scripts, then runs the hall
// sieges over the WithZones siege zones, as the boot path does once the
// halls run.
func startHallSieges(t *testing.T, o *options, halls *clanhall.Halls, gcl *network.GameClientLink) {
	t.Helper()
	for _, s := range o.hallSieges.scripts {
		if _, ok := halls.BindSiege(s.hallID, s.script); !ok {
			t.Fatalf("bind hall %d siege: not a siegable hall, or bound twice", s.hallID)
		}
	}
	cfg := clanhall.DefaultSiegeConfig()
	if o.hallSieges.cfg != nil {
		cfg = *o.hallSieges.cfg
	}
	var fields []*zone.Siege
	if o.zones != nil {
		fields = zone.OfKind[*zone.Siege](o.zones)
	}
	halls.RunSieges(cfg, network.ClanHallSiegeNotifier(gcl), fields)
}

// DoorAttackableBy reports whether the online player objID may attack
// doorID's door.
func (s *Server) DoorAttackableBy(tb testing.TB, doorID int, objID int32) bool {
	tb.Helper()
	return s.spawnedDoor(tb, doorID).AttackableBy(s.onlineCharacter(tb, objID))
}

// DoorState returns doorID's door's HP and whether it stands open.
func (s *Server) DoorState(tb testing.TB, doorID int) (hp float64, open bool) {
	tb.Helper()
	d := s.spawnedDoor(tb, doorID)
	return d.CurrentHP(), d.Opened()
}

// PlayerPvPFlagged reports whether the online player objID carries the pvp
// flag.
func (s *Server) PlayerPvPFlagged(tb testing.TB, objID int32) bool {
	tb.Helper()
	return s.onlineCharacter(tb, objID).PvPFlagged()
}
