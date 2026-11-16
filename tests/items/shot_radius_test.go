package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// shotViewers are two players standing near a shot user at the shared spawn
// point: near inside the 600-unit radius of the charge visual, far outside
// it while still knowing the user.
type shotViewers struct {
	near, far *testsupport.ScriptedClient
}

// joinShotViewers logs near and far in on the user's x axis, 400 and 900
// units away, and checks far really knows the user, so its silence is the
// radius at work and not an unseen user.
func joinShotViewers(t *testing.T, srv *gameservertest.Server, userID int32, tag string) shotViewers {
	t.Helper()
	join := func(name string, dx int) (*testsupport.ScriptedClient, int32) {
		account := tag + name
		ch := srv.SeedCharacterForAt(t, account, account, 1, 0, location.Location{X: spawnX + dx, Y: spawnY, Z: spawnZ})
		c := srv.DialClient(t, account, 1)
		startInWorld(t, c)
		return c, ch.ID
	}
	near, _ := join("near", 400)
	far, farID := join("far", 900)
	v := shotViewers{near: near, far: far}
	user, ok := srv.State.Object(userID)
	if !ok {
		t.Fatal("shot user is not in the world")
	}
	knows := false
	for o := range srv.State.KnownPlayers(user) {
		knows = knows || o.ObjectID() == farID
	}
	if !knows {
		t.Fatal("far viewer does not know the shot user: the radius check would prove nothing")
	}
	drainUntilQuiet(t, srv.Client)
	drainUntilQuiet(t, v.near)
	drainUntilQuiet(t, v.far)
	return v
}

// selfCasts counts the MagicSkillUse frames caster casts on itself with
// skillID.
func selfCasts(t *testing.T, frames [][]byte, caster, skillID int32) int {
	t.Helper()
	n := 0
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeMagicSkillUse {
			continue
		}
		r := wire.NewReader(f[1:])
		if c, target, sid := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); c == caster && target == caster && sid == skillID {
			n++
		}
	}
	return n
}

// assertChargeVisualAudience checks the user and near each saw caster's
// charge visual once and far saw none.
func assertChargeVisualAudience(t *testing.T, user *testsupport.ScriptedClient, v shotViewers, caster, skillID int32) {
	t.Helper()
	for _, who := range []struct {
		name string
		c    *testsupport.ScriptedClient
		want int
	}{{"user", user, 1}, {"near viewer", v.near, 1}, {"far viewer", v.far, 0}} {
		if got := selfCasts(t, collectUntilQuiet(t, who.c), caster, skillID); got != who.want {
			t.Fatalf("%s saw %d charge visuals of %d (skill %d), want %d", who.name, got, caster, skillID, who.want)
		}
	}
}

// TestShotChargeVisualReachesOnlyPlayersWithin600 pins the audience of the
// shot charge visual (issue #4816): the user plus the players that know it
// within 600 units of it, for a direct use and for the charge an auto-use
// toggle makes at once. A beast shot's visual is cast by the summon, but its
// radius is still measured from the user.
func TestShotChargeVisualReachesOnlyPlayersWithin600(t *testing.T) {
	t.Parallel()
	weaponShot := func(t *testing.T, tag string, use func(c *testsupport.ScriptedClient, shot int32)) {
		srv, shot := bootShotRig(t, 1463, 10)
		objID := srv.SoleObjectID(t)
		v := joinShotViewers(t, srv, objID, tag)
		use(srv.Client, shot)
		assertChargeVisualAudience(t, srv.Client, v, objID, 2150)
	}
	beastShot := func(t *testing.T, tag string, use func(c *testsupport.ScriptedClient, shot int32)) {
		srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
		objID := srv.SoleObjectID(t)
		shot := srv.GiveItem(t, objID, 6645, 10)
		startInWorld(t, srv.Client)
		servitor, err := summon.NewServitor(summon.ServitorConfig{
			ObjectID: srv.NewObjectID(),
			Level:    44,
			Stats:    summon.CombatStats{MaxHP: 500, MaxMP: 200, SSCount: 5},
		})
		if err != nil {
			t.Fatal(err)
		}
		servitor.SetQueue(srv.PlayerQueue(t, objID))
		srv.State.AddSummon(objID, servitor)
		v := joinShotViewers(t, srv, objID, tag)
		use(srv.Client, shot)
		assertChargeVisualAudience(t, srv.Client, v, servitor.ObjectID(), 2033)
	}
	click := func(c *testsupport.ScriptedClient, shot int32) { c.Send(encodeUseItem(shot, false)) }
	for _, tc := range []struct {
		name string
		run  func(t *testing.T, tag string, use func(c *testsupport.ScriptedClient, shot int32))
		tag  string
		use  func(c *testsupport.ScriptedClient, shot int32)
	}{
		{"soulshot used", weaponShot, "ssuse", click},
		{"soulshot auto-use enabled", weaponShot, "ssauto", func(c *testsupport.ScriptedClient, _ int32) {
			c.Send(encodeRequestAutoSoulShot(1463, 1))
		}},
		{"beast soulshot used", beastShot, "bsuse", click},
		{"beast soulshot auto-use enabled", beastShot, "bsauto", func(c *testsupport.ScriptedClient, _ int32) {
			c.Send(encodeRequestAutoSoulShot(6645, 1))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.run(t, tc.tag, tc.use)
		})
	}
}
