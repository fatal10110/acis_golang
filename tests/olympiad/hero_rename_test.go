package olympiad

import (
	"context"
	"database/sql"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// TestHeroElectionDethronesRenamedHero pins the parity exception of
// #5027: the election reaches the outgoing heroes by object id, so the
// ending era's active hero, renamed by //set name since its election and
// online, still loses its status and its hero skills.
func TestHeroElectionDethronesRenamedHero(t *testing.T) {
	t.Parallel()
	data, err := gamexml.LoadAdminData(datapack.Path(t, "data", "xml"))
	if err != nil {
		t.Fatalf("load admin data: %v", err)
	}
	opts := append(skillOptions(t), gameservertest.WithAdmin(data), gameservertest.WithReuseDelays(0, 0))
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Login", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithOlympiadSeed(func(db *sql.DB) {
			for _, stmt := range append(electionSeed(), `UPDATE characters SET accesslevel = 7 WHERE char_name = 'Login'`) {
				if _, err := db.Exec(stmt); err != nil {
					t.Fatalf("seed %q: %v", stmt, err)
				}
			}
		}),
	}, opts...)...)
	login := srv.SoleObjectID(t)
	c := srv.Client
	burst := startInWorld(t, c)
	if got := userInfoHero(firstFrame(t, burst, serverpackets.OpcodeUserInfo)); got != 1 {
		t.Fatalf("login UserInfo hero byte = %d, want 1", got)
	}

	c.Send(encodeBuildCmd("set name Renamed"))
	drainFrames(t, c)
	srv.FlushPersistence(t)
	var name string
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT char_name FROM characters WHERE obj_Id = ?", login).Scan(&name); err != nil {
		t.Fatalf("read name: %v", err)
	}
	if name != "Renamed" {
		t.Fatalf("stored name after //set name = %q, want Renamed", name)
	}

	srv.Olympiad.SelectHeroes()
	srv.Settle(t)
	srv.FlushPersistence(t)
	srv.Settle(t)
	frames := drainFrames(t, c)
	skills := skillListLevels(t, firstFrame(t, frames, serverpackets.OpcodeSkillList))
	for _, id := range heroSkills {
		if _, ok := skills[id]; ok {
			t.Errorf("SkillList after the election still has hero skill %d", id)
		}
	}
	if got := userInfoHero(lastFrame(t, frames, serverpackets.OpcodeUserInfo)); got != 0 {
		t.Errorf("UserInfo hero byte after the election = %d, want 0", got)
	}
	if srv.Heroes.IsActive(login) {
		t.Error("the renamed hero is still active after the election")
	}
}
