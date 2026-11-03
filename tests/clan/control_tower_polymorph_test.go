package clan

import (
	"context"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// TestControlTowerAdminPolymorph pins //polymorph on a control tower
// (AdminPolymorph, Creature.polymorph and unpolymorph): every tower rule
// keys on the tower being shown in a form (LifeTower, FlameTower,
// Castle.getAliveLifeTowerCount). On a working tower the command is a
// toggle whatever follows: the tower falls dead with its HP as it is, no
// refill (only the siege's unpolymorph fills it), shown with its stationary
// view (which shows HP only to a viewer that may attack it) and attackable
// by no one, and a life tower so felled no longer
// counts as standing, with no one told. On a dead tower an NPC id makes it
// work as that NPC, shown with NpcInfo and attackable by the attackers.
func TestControlTowerAdminPolymorph(t *testing.T) {
	t.Parallel()
	data, err := gamexml.LoadAdminData(datapack.Path(t, "data", "xml"))
	if err != nil {
		t.Fatalf("load admin data: %v", err)
	}
	w := towerWorld(t, gameservertest.WithAdmin(data))
	if _, err := w.srv.DB.ExecContext(context.Background(), "UPDATE characters SET accesslevel = 7 WHERE obj_Id = ?", w.leaderID); err != nil {
		t.Fatalf("set access level: %v", err)
	}
	gludio, life1, _, flame := towers(t, w)
	full := flame.MaxHP()
	startInWorld(t, w.leader)
	startInWorld(t, w.member)
	gludio.Start()
	drainFrames(t, w.leader)
	drainFrames(t, w.member)

	founderObj, _ := w.srv.State.Object(w.leaderID)
	attacker, ok := founderObj.(attackable.Combatant)
	if !ok {
		t.Fatalf("founder is %T, no combatant", founderObj)
	}
	flame.TakeDamage(300, attacker)
	if !flame.TowerWorking() || flame.CurrentHP() != full-300 {
		t.Fatalf("flame tower after 300 damage working %v HP %d, want working at %d", flame.TowerWorking(), flame.CurrentHP(), full-300)
	}

	polymorph := func(tower *npc.Folk, line string) (founder, rival [][]byte) {
		t.Helper()
		w.leader.Send(encodeAction(tower.ObjectID(), w.at))
		drainFrames(t, w.leader)
		drainFrames(t, w.member)
		w.leader.Send(encodeBuildCmd(line))
		founder, rival = drainFrames(t, w.leader), drainFrames(t, w.member)
		for _, f := range founder {
			if f[0] == serverpackets.OpcodeSystemMessage {
				t.Fatalf("GM frames %x answer //%s", testsupport.FrameOpcodes(founder), line)
			}
		}
		return founder, rival
	}

	// A working flame tower falls dead, its HP as it is.
	founder, rival := polymorph(flame, "polymorph 99999")
	deadFlame := dead(false)
	if got := shown(t, founder, flame.ObjectID(), true); got != deadFlame {
		t.Fatalf("attacker sees the unpolymorphed flame tower as %+v, want %+v", got, deadFlame)
	}
	if got := shown(t, rival, flame.ObjectID(), true); got != deadFlame {
		t.Fatalf("owner sees the unpolymorphed flame tower as %+v, want %+v", got, deadFlame)
	}
	if flame.TowerWorking() || flame.Polymorph() != nil || flame.CurrentHP() != full-300 || flame.AlikeDead() {
		t.Fatalf("unpolymorphed flame tower working %v HP %d, want dead at %d", flame.TowerWorking(), flame.CurrentHP(), full-300)
	}

	// A working life tower so felled stops counting, with no one told.
	_, rival = polymorph(life1, "polymorph")
	if got, want := shown(t, rival, life1.ObjectID(), true), dead(true); got != want {
		t.Fatalf("owner sees the unpolymorphed life tower as %+v, want %+v", got, want)
	}
	if got := gludio.AliveLifeTowers(); got != 1 {
		t.Fatalf("alive life towers = %d, want 1", got)
	}
	if ids := messages(t, rival); len(ids) != 0 {
		t.Fatalf("owner told %v by an unpolymorphed life tower", ids)
	}

	// A dead tower works as the NPC the id names.
	founder, rival = polymorph(flame, "polymorph "+itoa(npc.LifeTowerWorkingNpcID))
	if got, want := shown(t, founder, flame.ObjectID(), true), (towerView{npcInfo: true, template: npc.LifeTowerWorkingNpcID, attackable: true}); got != want {
		t.Fatalf("attacker sees the polymorphed dead flame tower as %+v, want %+v", got, want)
	}
	if got, want := shown(t, rival, flame.ObjectID(), true), (towerView{npcInfo: true, template: npc.LifeTowerWorkingNpcID}); got != want {
		t.Fatalf("owner sees the polymorphed dead flame tower as %+v, want %+v", got, want)
	}
	if !flame.TowerWorking() {
		t.Fatal("polymorphed dead flame tower does not work")
	}
}
