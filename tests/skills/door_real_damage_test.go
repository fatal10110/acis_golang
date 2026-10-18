package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// REAL_DAMAGE on a door (RealDamage.useSkill, Door is a Creature): the
// skill's power comes straight off the door's HP, setHp broadcasting the
// HP left, or, once HP - power <= 0, the door dies (Door.doDie), its zero
// HP shown as it is set and again for the death.

const doorRealDamageSkillID = 9303

// readDoorStatusHPs reads c's frames for up to 5s and returns the HP of
// each DoorStatusUpdate about gate, stopping once want of them arrived.
func readDoorStatusHPs(t *testing.T, c *testsupport.ScriptedClient, gate *door.Object, want int) []int32 {
	t.Helper()
	var hps []int32
	for end := c.Now().Add(5 * time.Second); c.Now().Before(end) && len(hps) < want; {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil || frame[0] != serverpackets.OpcodeDoorStatusUpdate {
			continue
		}
		r := wireReader(frame[1:])
		if r.ReadInt32() != gate.ObjectID() {
			continue
		}
		for range 5 { // closed, damage stage, show HP, door id, max HP
			r.ReadInt32()
		}
		hps = append(hps, r.ReadInt32())
	}
	return hps
}

func TestRealDamageOnDoor(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		power int
		want  []int32
		dead  bool
	}{
		{name: "HP left", power: 300, want: []int32{combatDoorMaxHP - 300}},
		{name: "breaks the door", power: combatDoorMaxHP, want: []int32{0, 0}, dead: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			def := doorSkill(doorRealDamageSkillID, "REAL_DAMAGE", true, tc.power)
			srv, objID, gate := bootDoorCombat(t, combatDoorTemplate(door.KindDoor), []modelskill.Definition{def})
			srv.BeginDoorSiege(t, gate.DoorID())
			c := srv.Client
			selectTarget(t, c, gate.ObjectID())

			c.Send(encodeRequestMagicSkillUse(doorRealDamageSkillID, false, false))
			readCastStartFrames(t, c, objID, doorRealDamageSkillID, 1, doorSkillHit, doorSkillReuse, gate.ObjectID())
			got := readDoorStatusHPs(t, c, gate, len(tc.want))
			if len(got) != len(tc.want) {
				t.Fatalf("DoorStatusUpdate HPs = %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("DoorStatusUpdate HPs = %v, want %v", got, tc.want)
				}
			}
			if gate.Dead() != tc.dead || gate.HP() != int(tc.want[len(tc.want)-1]) {
				t.Fatalf("door dead %t HP %d, want dead %t HP %d", gate.Dead(), gate.HP(), tc.dead, tc.want[len(tc.want)-1])
			}
		})
	}
}
