package admin

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// actionPrivateStoreSell is the RequestActionUse id of the sell store
// set-up.
const actionPrivateStoreSell = 10

// storeItemID is a plain stackable item the sell store lists.
const storeItemID = 20

func encodeSetPrivateStoreListSell(objectID, count, price int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeSetPrivateStoreListSell)
	w.WriteInt32(0)
	w.WriteInt32(1)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	w.WriteInt32(price)
	return w.Bytes()
}

// bootPostureAdmin boots a GM and a second character, Player, both in the
// world, Player holding potions of storeItemID. It returns the server,
// Player's client, character and the potions' object id.
func bootPostureAdmin(t *testing.T) (*gameservertest.Server, *testsupport.ScriptedClient, *player.Character, int32) {
	t.Helper()
	srv, _ := bootAdmin(t, adminLevel, gameservertest.WithHTMLPages(shippedAdminPages(t, "charinfo.htm")))
	if !srv.DrivesClock() {
		t.Skip("holding a posture transition open needs the driven clock")
	}
	enterWorld(t, srv.Client)
	ch := srv.SeedCharacterFor(t, "player2", "Player", 1, 0)
	potions := srv.GiveItem(t, ch.ID, storeItemID, 5)
	user := srv.DialClient(t, "player2", 1)
	enterWorld(t, user)
	drain(t, user)
	drain(t, srv.Client)
	return srv, user, onlineCharacter(t, srv, ch.ID), potions
}

// onPlayerQueue runs fn on pc's own queue and waits for it.
func onPlayerQueue(t *testing.T, pc *player.Character, fn func()) {
	t.Helper()
	done := make(chan struct{})
	if !pc.Queue().Post(func() { fn(); close(done) }) {
		t.Fatal("post to the player queue: queue closed")
	}
	<-done
}

// TestAdminCharacterPagePostureCurrentIntention pins %ai% (AdminEditChar
// gatherPlayerInfo, getCurrentIntention().getType()) during a sit or stand
// request: PlayerAI doSitIntention / doStandIntention make SIT or STAND the
// current intention until onEvtSatDown / onEvtStoodUp settle it, a request
// clicked meanwhile queued behind it. A store set-up sits the player down
// without that intention. A seated player hit by an attack stands through
// doStandIntention (PlayerAI.onEvtAttacked), and a stand request while
// playing dead gets up through thinkStand's stopFakeDeath: both read STAND
// until the get-up ends.
func TestAdminCharacterPagePostureCurrentIntention(t *testing.T) {
	t.Parallel()

	expectAI := func(t *testing.T, srv *gameservertest.Server, when, want string) {
		t.Helper()
		if got := charInfoAI(t, srv); got != want {
			t.Fatalf("intentions %s = %q, want %q", when, got, want)
		}
	}

	t.Run("sit", func(t *testing.T) {
		t.Parallel()
		srv, user, userID, _, _ := bootNextAttackAdmin(t)
		user.Send(encodeChangeWaitType(false))
		if !slices.Contains(opcodes(settle(t, user)), serverpackets.OpcodeChangeWaitType) {
			t.Fatal("sit request did not sit Player down")
		}
		expectAI(t, srv, "during the sit-down", "SIT <> IDLE")
		srv.SettlePosture(t, userID)
		settle(t, user)
		expectAI(t, srv, "once seated", "IDLE <> IDLE")
	})

	t.Run("stand", func(t *testing.T) {
		t.Parallel()
		srv, user, userID, _, _ := bootNextAttackAdmin(t)
		standingUp(t, srv, user, userID)
		expectAI(t, srv, "during the stand-up", "STAND <> IDLE")
		srv.SettlePosture(t, userID)
		settle(t, user)
		expectAI(t, srv, "once standing", "IDLE <> IDLE")
	})

	t.Run("stand with an attack queued", func(t *testing.T) {
		t.Parallel()
		srv, user, userID, first, _ := bootNextAttackAdmin(t)
		standingUp(t, srv, user, userID)
		clickAttack(t, user, first)
		expectAI(t, srv, "with an attack queued mid stand-up", "STAND <> ATTACK")
	})

	t.Run("store sit", func(t *testing.T) {
		t.Parallel()
		srv, user, _, potions := bootPostureAdmin(t)
		user.Send(encodeActionUse(actionPrivateStoreSell))
		if !slices.Contains(opcodes(settle(t, user)), serverpackets.OpcodePrivateStoreManageListSell) {
			t.Fatal("sell command did not open the manage window")
		}
		user.Send(encodeSetPrivateStoreListSell(potions, 5, 10))
		frames := settle(t, user)
		if !slices.Contains(opcodes(frames), serverpackets.OpcodeChangeWaitType) {
			t.Fatalf("store set-up frames = %x, want it to sit Player down", opcodes(frames))
		}
		expectAI(t, srv, "during the store's sit-down", "IDLE <> IDLE")
	})

	t.Run("stand when hit while seated", func(t *testing.T) {
		t.Parallel()
		srv, user, pc, _ := bootPostureAdmin(t)
		tmpl := gameservertest.AttackingHostileTemplate()
		tmpl.BaseAttackRange = 40
		tmpl.AtkSpd = 1500
		x, y, z := pc.Position()
		hostile := srv.SpawnAttackingHostileNPCTemplate(t, tmpl, location.Location{X: x + 20, Y: y, Z: z})
		user.Send(encodeChangeWaitType(false))
		settle(t, user)
		srv.SettlePosture(t, pc.ObjectID())
		settle(t, user)
		expectAI(t, srv, "once seated", "IDLE <> IDLE")

		hostile.DoAttack(t, pc)
		srv.Settle(t)
		if !pc.StandingNow() {
			t.Fatal("the hit did not start a stand-up")
		}
		expectAI(t, srv, "during the stand-up the hit started", "STAND <> IDLE")
		srv.SettlePosture(t, pc.ObjectID())
		settle(t, user)
		expectAI(t, srv, "once standing", "IDLE <> IDLE")
	})

	t.Run("stand out of fake death", func(t *testing.T) {
		t.Parallel()
		srv, user, pc, _ := bootPostureAdmin(t)
		onPlayerQueue(t, pc, func() {
			e, err := effect.New(effect.Skill{ID: 1}, modelskill.EffectTemplate{Name: "FakeDeath"})
			if err != nil {
				t.Errorf("new FakeDeath effect: %v", err)
				return
			}
			e.Effected = pc
			pc.EffectList().Add(e)
		})
		srv.AdvanceUntil(t, "fake-death lie-down ended", pc.Seated)
		settle(t, user)
		expectAI(t, srv, "lying in fake death", "IDLE <> IDLE")

		user.Send(encodeChangeWaitType(true))
		settle(t, user)
		if !pc.StandingNow() {
			t.Fatal("the stand request did not start the get-up")
		}
		expectAI(t, srv, "during the get-up", "STAND <> IDLE")
		srv.SettlePosture(t, pc.ObjectID())
		settle(t, user)
		expectAI(t, srv, "once up", "IDLE <> IDLE")
	})
}
