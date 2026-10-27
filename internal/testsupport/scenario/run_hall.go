package scenario

import (
	"context"
	"database/sql"
	"sync"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	hallmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// shippedClanHallData is the shipped clan hall data, loaded once per test
// binary.
var shippedClanHallData struct {
	once  sync.Once
	halls *hallmodel.Table
	decos *hallmodel.DecoTable
	err   error
}

// hallOptions are the boot options a hall line, or a clan line with a
// hall, asks for: the shipped clan halls and their decorations.
func hallOptions(t *testing.T) []gameservertest.Option {
	t.Helper()
	halls := datapack.Path(t, "data", "xml", "clanHalls.xml")
	decos := datapack.Path(t, "data", "xml", "clanHallDeco.xml")
	shippedClanHallData.once.Do(func() {
		d := &shippedClanHallData
		if d.halls, d.err = gamexml.LoadClanHalls(halls); d.err == nil {
			d.decos, d.err = gamexml.LoadClanHallDeco(decos)
		}
	})
	if shippedClanHallData.err != nil {
		t.Fatalf("load shipped clan halls: %v", shippedClanHallData.err)
	}
	return []gameservertest.Option{
		gameservertest.WithClanHalls(shippedClanHallData.halls, shippedClanHallData.decos),
		gameservertest.WithResidences(shippedClanHallData.halls, nil),
	}
}

// usesHalls reports whether the scenario runs the shipped clan halls.
func (sc *Scenario) usesHalls() bool { return len(sc.Halls) > 0 || sc.ClanHall != 0 }

// hallStatus is the siege status a hall line or hallstatus step names.
func hallStatus(name string) siege.Status {
	switch name {
	case "waiting":
		return siege.StatusRegistrationOver
	case "siege":
		return siege.StatusInProgress
	}
	return siege.StatusRegistrationOpened
}

// seedHalls stores the clanhall rows of the hall lines and of the clan
// line's hall, before the clan halls are restored: a hall owned by the
// clan line's clan has it for its owner. The rows are paid for far ahead,
// so no fee falls due during a run.
func seedHalls(t *testing.T, db *sql.DB, sc *Scenario) {
	t.Helper()
	const paidUntil = 4102444800000 // 2100-01-01 00:00 UTC
	owner := func(id int) int {
		if sc.Clan > 0 && sc.ClanHall == id {
			return scenarioClan
		}
		return 0
	}
	ctx := context.Background()
	var err error
	for _, h := range sc.Halls {
		if h.Date > 0 {
			_, err = db.ExecContext(ctx, "INSERT INTO clanhall (id, ownerId, paid, paidUntil, endDate) VALUES (?, ?, 1, ?, ?)", h.ID, owner(h.ID), paidUntil, h.Date)
		} else {
			_, err = db.ExecContext(ctx, "INSERT INTO clanhall (id, ownerId, paid, paidUntil) VALUES (?, ?, 1, ?)", h.ID, owner(h.ID), paidUntil)
		}
		if err != nil {
			t.Fatalf("seed hall %d: %v", h.ID, err)
		}
	}
	if sc.ClanHall != 0 && !hasHall(sc, sc.ClanHall) {
		if _, err := db.ExecContext(ctx, "INSERT INTO clanhall (id, ownerId, paid, paidUntil) VALUES (?, ?, 1, ?)", sc.ClanHall, owner(sc.ClanHall), paidUntil); err != nil {
			t.Fatalf("seed hall %d: %v", sc.ClanHall, err)
		}
	}
}

func hasHall(sc *Scenario, id int) bool {
	for _, h := range sc.Halls {
		if h.ID == id {
			return true
		}
	}
	return false
}

// setHalls puts each hall line's siege at its status, once the server has
// booted.
func (r *run) setHalls(t *testing.T) {
	t.Helper()
	for _, h := range r.sc.Halls {
		r.setHallStatus(t, h.ID, h.Status)
		if hs, _ := r.srv.Halls.Siege(int32(h.ID)); h.Date > 0 && hs.Date() != h.Date {
			t.Fatalf("%s: hall line: hall %d keeps next siege %d, want the stored %d", r.sc.Name, h.ID, hs.Date(), h.Date)
		}
	}
	if id := r.sc.ClanHall; id != 0 {
		var owner int32
		for _, v := range r.srv.Halls.All() {
			if int(v.ID) == id {
				owner = v.OwnerID
			}
		}
		if owner != scenarioClan {
			t.Fatalf("%s: clan line: hall %d is owned by clan %d, want %d", r.sc.Name, id, owner, scenarioClan)
		}
	}
}

// setHallStatus puts hall id's siege at status.
func (r *run) setHallStatus(t *testing.T, id int, status string) {
	t.Helper()
	hs, ok := r.srv.Halls.Siege(int32(id))
	if !ok {
		t.Fatalf("%s: hall %d has no siege: it is no siegable hall of the shipped data", r.sc.Name, id)
	}
	hs.SetStatus(hallStatus(status))
}
