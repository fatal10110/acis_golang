package quest

import (
	"context"
	"database/sql"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// beastFarm is the Wild Beast Reserve, a siegable clan hall.
const beastFarm = 63

// clanSeed runs the clan seed statements qs on one connection, so a
// temporary table lives across them.
func clanSeed(t *testing.T, qs ...string) gameservertest.Option {
	return gameservertest.WithClanSeed(func(db *sql.DB) {
		ctx := context.Background()
		conn, err := db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		for _, q := range qs {
			if _, err := conn.ExecContext(ctx, q); err != nil {
				t.Fatalf("%s: %v", q, err)
			}
		}
	})
}

// TestPlayerClanLeaderAndHall reads the clan of a member of the Chiefs, who
// own the Wild Beast Reserve: no leader while Chief is offline, Chief once
// in the world, Chief itself when Chief asks; the hall for both.
func TestPlayerClanLeaderAndHall(t *testing.T) {
	t.Parallel()
	srv, member := bootJournal(t, clanSeed(t,
		`DROP TEMPORARY TABLE IF EXISTS chief`,
		`CREATE TEMPORARY TABLE chief SELECT * FROM characters WHERE char_name = 'Newbie'`,
		`UPDATE chief SET obj_Id = 900001, account_name = 'chief', char_name = 'Chief', clanid = 7001`,
		`INSERT INTO characters SELECT * FROM chief`,
		`UPDATE characters SET clanid = 7001 WHERE char_name = 'Newbie'`,
		`INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, leader_id) VALUES (7001, 'Chiefs', 4, 0, 900001)`,
		`INSERT INTO clanhall (id, ownerId, paid, paidUntil) VALUES (63, 7001, 1, 4102444800000)`,
		`DROP TEMPORARY TABLE chief`,
	))
	const chief = 900001
	enterWorld(t, srv)

	run(t, srv, member, func(_ *script.Script, p *script.Player) {
		if l := p.ClanLeader(); l != nil {
			t.Errorf("an offline leader = %d", l.ObjectID())
		}
		if got := p.ClanHallID(); got != beastFarm {
			t.Errorf("the member's clan hall = %d, want %d", got, beastFarm)
		}
	})

	chiefClient := srv.DialClient(t, "chief", 1)
	chiefClient.Send(encodeRequestGameStart(0))
	readUntil(t, chiefClient, serverpackets.OpcodeCharSelected)
	chiefClient.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
	srv.ReadQueued(t, chiefClient)

	run(t, srv, member, func(_ *script.Script, p *script.Player) {
		l := p.ClanLeader()
		if l == nil || l.ObjectID() != chief {
			t.Fatalf("the leader in the world = %v, want %d", l, chief)
		}
		// The handle is the world's player: its own reads work.
		if !l.IsClanLeader() || l.ClanHallID() != beastFarm {
			t.Errorf("the leader handle: leads %v, hall %d", l.IsClanLeader(), l.ClanHallID())
		}
	})
	run(t, srv, chief, func(_ *script.Script, p *script.Player) {
		if l := p.ClanLeader(); l == nil || l.ObjectID() != chief {
			t.Errorf("the leader's own leader = %v, want itself", l)
		}
	})
}

// TestPlayerClanHallOfAClanWithNoHall reads a clan that owns no hall: 0, and
// its leader is the player.
func TestPlayerClanHallOfAClanWithNoHall(t *testing.T) {
	t.Parallel()
	srv, objID := bootJournal(t, clanSeed(t,
		`UPDATE characters SET clanid = 7002 WHERE char_name = 'Newbie'`,
		`INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, leader_id) SELECT 7002, 'Loners', 1, 0, obj_Id FROM characters WHERE char_name = 'Newbie'`,
	))
	enterWorld(t, srv)
	run(t, srv, objID, func(_ *script.Script, p *script.Player) {
		if got := p.ClanHallID(); got != 0 {
			t.Errorf("clan hall = %d, want none", got)
		}
		if l := p.ClanLeader(); l == nil || l.ObjectID() != objID {
			t.Errorf("leader = %v, want the player", l)
		}
	})
}

// TestPlayerClanReadsOfAClanlessPlayerPanic: a player in no clan has no
// clan to read, so each read panics, as the reference throws on the null
// clan, and the invocation aborts there.
func TestPlayerClanReadsOfAClanlessPlayerPanic(t *testing.T) {
	t.Parallel()
	srv, objID := bootJournal(t, gameservertest.WithCapturedLog())
	enterWorld(t, srv)
	for name, read := range map[string]func(*script.Player){
		"ClanLeader": func(p *script.Player) { p.ClanLeader() },
		"ClanHallID": func(p *script.Player) { p.ClanHallID() },
	} {
		reached := false
		if srv.RunScript(t, objID, q001, func(_ *script.Script, p *script.Player) {
			read(p)
			reached = true
		}) || reached {
			t.Errorf("%s of a clanless player returned", name)
		}
	}
}
