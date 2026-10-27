package scenario

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// scenarioMateBase is the object id of the first mate line's character.
const scenarioMateBase = 900100

// mateObject is the object id the i-th mate line's character is seeded on.
func mateObject(i int) int32 { return int32(scenarioMateBase + i) }

// mateAccount is the account a mate line's character is seeded on.
func mateAccount(m Mate) string { return "mate_" + strings.ToLower(m.Name) }

// mateClient is one mate line's character: its client once it entered the
// world, nil before.
type mateClient struct {
	obj    int32
	client *testsupport.ScriptedClient
}

// seedMates stores the mate lines' characters as clan members, with the
// academy links the sponsor and apprentice flags give, and hands the clan
// to the mate that leads it. Each is a copy of the character's row on its
// own account, so the clan restores with them as members.
func seedMates(t *testing.T, db *sql.DB, sc *Scenario) {
	t.Helper()
	if len(sc.Mates) == 0 {
		return
	}
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("seed mates: %v", err)
	}
	defer conn.Close()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := conn.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("seed mates: %s: %v", query, err)
		}
	}
	for i, m := range sc.Mates {
		obj := mateObject(i)
		exec("DROP TEMPORARY TABLE IF EXISTS mate")
		exec("CREATE TEMPORARY TABLE mate SELECT * FROM characters WHERE char_name = ?", sc.Character)
		exec("UPDATE mate SET obj_Id = ?, account_name = ?, char_name = ?, level = ?, clanid = ?, apprentice = 0, sponsor = 0",
			obj, mateAccount(m), m.Name, m.Level, scenarioClan)
		exec("INSERT INTO characters SELECT * FROM mate")
		exec("DROP TEMPORARY TABLE mate")
		exec("UPDATE characters SET exp = ? WHERE obj_Id = ?", scenarioLevels(t).RequiredExpForLevel(m.Level), obj)
		switch m.Relation {
		case "sponsor":
			exec("UPDATE characters SET apprentice = (SELECT obj_Id FROM (SELECT obj_Id FROM characters WHERE char_name = ?) c) WHERE obj_Id = ?", sc.Character, obj)
			exec("UPDATE characters SET sponsor = ? WHERE char_name = ?", obj, sc.Character)
		case "apprentice":
			exec("UPDATE characters SET sponsor = (SELECT obj_Id FROM (SELECT obj_Id FROM characters WHERE char_name = ?) c) WHERE obj_Id = ?", sc.Character, obj)
			exec("UPDATE characters SET apprentice = ? WHERE char_name = ?", obj, sc.Character)
		}
		if m.Leader {
			exec("UPDATE clan_data SET leader_id = ? WHERE clan_id = ?", obj, scenarioClan)
		}
	}
}

// seedMateHolds gives each mate its hold lines' items, once the server has
// booted.
func (r *run) seedMateHolds(t *testing.T) {
	t.Helper()
	for i, m := range r.sc.Mates {
		r.roles[mateObject(i)] = m.Name
		r.mates = append(r.mates, mateClient{obj: mateObject(i)})
		for _, h := range m.Holds {
			tmpl, ok := r.items.Get(h.Item)
			switch {
			case !ok:
				t.Fatalf("%s: mate %s: hold %d: no such template in the catalog; add an item line", r.sc.Name, m.Name, h.Item)
			case tmpl.Stackable:
				r.srv.GiveItem(t, mateObject(i), h.Item, h.Count)
			default:
				for range h.Count {
					r.srv.GiveItem(t, mateObject(i), h.Item, 1)
				}
			}
		}
	}
}

// checkMates requires the server to have restored the clan as the mate
// lines wrote it: the leader, and the academy links of the character and
// of every mate.
func (r *run) checkMates(t *testing.T) {
	t.Helper()
	if len(r.sc.Mates) == 0 {
		return
	}
	cl, ok := r.srv.Clans.Table().Get(scenarioClan)
	if !ok {
		t.Fatalf("%s: mate line: the clan is not restored", r.sc.Name)
	}
	want := r.player
	if lm, ok := r.sc.LeaderMate(); ok {
		want = mateObject(slices.IndexFunc(r.sc.Mates, func(m Mate) bool { return m.Name == lm.Name }))
	}
	if cl.LeaderID() != want {
		t.Fatalf("%s: mate line: the clan is restored with leader %d, want %d", r.sc.Name, cl.LeaderID(), want)
	}
	wantLinks := map[int32][2]int32{} // object id -> sponsor, apprentice
	for i, m := range r.sc.Mates {
		switch m.Relation {
		case "sponsor":
			wantLinks[mateObject(i)] = [2]int32{0, r.player}
			wantLinks[r.player] = [2]int32{mateObject(i), 0}
		case "apprentice":
			wantLinks[mateObject(i)] = [2]int32{r.player, 0}
			wantLinks[r.player] = [2]int32{0, mateObject(i)}
		}
	}
	objs := []int32{r.player}
	for i := range r.sc.Mates {
		objs = append(objs, mateObject(i))
	}
	for _, obj := range objs {
		m, ok := cl.Member(obj)
		if !ok {
			t.Fatalf("%s: mate line: character %d is not a restored clan member", r.sc.Name, obj)
		}
		if want := wantLinks[obj]; m.Sponsor != want[0] || m.Apprentice != want[1] {
			t.Fatalf("%s: mate line: character %d is restored with sponsor %d and apprentice %d, want %d and %d",
				r.sc.Name, obj, m.Sponsor, m.Apprentice, want[0], want[1])
		}
	}
}

// enterMate logs the i-th mate line's character in on its own client and
// enters it into the world, then puts it at its offset from where the
// character first entered. What the mate itself was sent while entering is
// dropped; what the other clients were sent stays queued for the step to
// compare.
func (r *run) enterMate(t *testing.T, i int) {
	t.Helper()
	m := r.sc.Mates[i]
	if r.mates[i].client != nil {
		t.Fatalf("%s: mate %s entered the world twice", r.sc.Name, m.Name)
	}
	c := r.srv.DialClient(t, mateAccount(m), 1)
	c.Send(gameStart())
	for n := 0; ; n++ {
		if n == 100 {
			t.Fatalf("%s: mate %s: no CharSelected within 100 frames", r.sc.Name, m.Name)
		}
		if c.Read()[0] == serverpackets.OpcodeCharSelected {
			break
		}
	}
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	r.srv.ReadQueued(t, c)
	if m.DX != 0 || m.DY != 0 || m.DZ != 0 {
		obj, ok := r.srv.State.Player(r.mates[i].obj)
		if !ok {
			t.Fatalf("%s: mate %s is not in the world", r.sc.Name, m.Name)
		}
		if err := r.srv.State.Move(obj, r.origin.X+m.DX, r.origin.Y+m.DY, r.origin.Z+m.DZ); err != nil {
			t.Fatalf("%s: mate %s: %v", r.sc.Name, m.Name, err)
		}
	}
	r.mates[i].client = c
}

// enterOnlineMates enters the mate lines that say online, right after the
// character's first entry.
func (r *run) enterOnlineMates(t *testing.T) {
	t.Helper()
	for i, m := range r.sc.Mates {
		if m.Online {
			r.enterMate(t, i)
		}
	}
}

// mateIndex is the index of the mate line of name.
func (r *run) mateIndex(t *testing.T, name string) int {
	t.Helper()
	i := slices.IndexFunc(r.sc.Mates, func(m Mate) bool { return m.Name == name })
	if i < 0 {
		t.Fatalf("%s: no mate line names %s", r.sc.Name, name)
	}
	return i
}

// actor is the client a step is played on: the character's, or the named
// mate's, which must be in the world; obj is its character.
func (r *run) actor(t *testing.T, as string) (c *testsupport.ScriptedClient, obj int32) {
	t.Helper()
	if as == "" {
		return r.srv.Client, r.player
	}
	i := r.mateIndex(t, as)
	if r.mates[i].client == nil {
		t.Fatalf("%s: mate %s is not in the world: it enters with online or an enter step", r.sc.Name, as)
	}
	return r.mates[i].client, r.mates[i].obj
}

// othersSent reads what every online client but acting was sent in the
// step: the rendered packets by the name of the character they went to,
// the character's as "player".
func (r *run) othersSent(t *testing.T, acting *testsupport.ScriptedClient) (map[string][]string, error) {
	t.Helper()
	got := map[string][]string{}
	read := func(name string, c *testsupport.ScriptedClient) error {
		if c == nil || c == acting {
			return nil
		}
		for _, f := range r.srv.ReadQueued(t, c) {
			line, ok, err := render(f, r.roles)
			if err != nil {
				return fmt.Errorf("frame %x sent to %s: %w", f, name, err)
			}
			if ok {
				got[name] = append(got[name], line)
			}
		}
		return nil
	}
	if err := read("player", r.srv.Client); err != nil {
		return nil, err
	}
	for i, m := range r.sc.Mates {
		if err := read(m.Name, r.mates[i].client); err != nil {
			return nil, err
		}
	}
	return got, nil
}

// drainMates drops what the mates in the world were sent.
func (r *run) drainMates(t *testing.T) {
	t.Helper()
	for _, m := range r.mates {
		if m.client != nil {
			r.srv.ReadQueued(t, m.client)
		}
	}
}
