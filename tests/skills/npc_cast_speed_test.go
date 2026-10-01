package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const npcSpeedSkill = modelskill.ID(9104)

// TestNPCCastingSpeedStartsFrom333 pins CreatureStatus.getMAtkSpd
// (CreatureStatus.java:636-639, which NpcStatus does not override) for a
// monster whose template P.Atk.Spd is 253: its casting speed finalizes from
// 333 times the WIT bonus, and both the NpcInfo a player is shown and the
// non-static hit time and reuse of its magic cast use that value
// (Formulas.calcAtkSpd: hitTime * 333 / MAtkSpd; CreatureCast.doCast:
// reuse *= 333.0 / MAtkSpd). Expected values are worked by hand from
// WIT_BONUS = floor(1.05^(WIT-20) * 100 + .5) / 100.
func TestNPCCastingSpeedStartsFrom333(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name                string
		wit                 int
		mAtkSpd, hit, reuse int32
	}{
		// WIT 20: bonus 1.00, so the cast runs at the configured times.
		{name: "WIT 20", wit: 20, mAtkSpd: 333, hit: 2000, reuse: 10000},
		// WIT 30: bonus 1.63; int(333*1.63) = 542, int(2000*333/542) = 1228,
		// int(10000 * (333.0/542)) = 6143.
		{name: "WIT 30", wit: 30, mAtkSpd: 542, hit: 1228, reuse: 6143},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
			)
			c := srv.Client
			startInWorld(t, c)
			drainUntilQuiet(t, c)
			hostile, aiCtl := srv.SpawnCastingHostileNPC(t, &npc.Template{
				ID: 100, TemplateID: 100, Type: "Monster", Level: 1, HPMax: 1000, WIT: tt.wit,
				AtkSpd: 253, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
			}, modelskill.NewTable([]modelskill.Definition{{
				ID: npcSpeedSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				Magic: true, SkillType: "BUFF", HitTime: 2000, ReuseDelay: 10000,
			}}))

			info := readUntil(t, c, serverpackets.OpcodeNPCInfo)
			r := wireReader(info[1:])
			if id := r.ReadInt32(); id != hostile.ObjectID() {
				t.Fatalf("NpcInfo object = %d, want monster %d", id, hostile.ObjectID())
			}
			for range 7 { // template id, attackable, x, y, z, heading, 0
				r.ReadInt32()
			}
			if got := r.ReadInt32(); got != tt.mAtkSpd {
				t.Fatalf("NpcInfo MAtkSpd = %d, want %d", got, tt.mAtkSpd)
			}
			// P.Atk.Spd stays on the template base: int(253 * DEX_BONUS[0] 0.84).
			if got := r.ReadInt32(); got != 212 {
				t.Fatalf("NpcInfo PAtkSpd = %d, want 212", got)
			}
			drainUntilQuiet(t, c)

			onNPCQueue(t, hostile, func() { aiCtl.Cast(hostile, modelskill.Ref{ID: npcSpeedSkill, Level: 1}) })
			use := readUntil(t, c, serverpackets.OpcodeMagicSkillUse)
			r = wireReader(use[1:])
			if caster := r.ReadInt32(); caster != hostile.ObjectID() {
				t.Fatalf("MagicSkillUse caster = %d, want monster %d", caster, hostile.ObjectID())
			}
			r.ReadInt32() // target
			if sid := r.ReadInt32(); sid != int32(npcSpeedSkill) {
				t.Fatalf("MagicSkillUse skill = %d, want %d", sid, npcSpeedSkill)
			}
			r.ReadInt32() // level
			if hit, reuse := r.ReadInt32(), r.ReadInt32(); hit != tt.hit || reuse != tt.reuse {
				t.Fatalf("MagicSkillUse hit/reuse = %d/%d, want %d/%d", hit, reuse, tt.hit, tt.reuse)
			}
		})
	}
}
