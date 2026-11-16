package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestSignetPointSpawnsFacingZero casts a signet from a caster facing
// 16384: L2SkillSignet.useSkill (L2SkillSignet.java:48-49) places the
// effect point with spawnMe(Location), so it keeps heading 0 rather than
// taking its caster's facing, and its NpcInfo shows heading 0.
func TestSignetPointSpawnsFacingZero(t *testing.T) {
	t.Parallel()
	srv, objID := bootSignetSight(t, 5)
	c := srv.Client
	startInWorldAmongPlayers(t, c)
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.SetHeading(16384) })
	drainUntilQuiet(t, c)

	point := castSignetPoint(t, srv, c)
	if got := point.Heading(); got != 0 {
		t.Fatalf("signet point heading = %d, want 0", got)
	}
	infos := readObjectFrames(c, serverpackets.OpcodeNPCInfo, point.ObjectID())
	if len(infos) == 0 {
		t.Fatal("caster got no NpcInfo of the signet point")
	}
	for _, f := range infos {
		r := wire.NewReader(f[1:])
		for range 6 { // object id, template, attackable, x, y, z
			r.ReadInt32()
		}
		if got := r.ReadInt32(); got != 0 {
			t.Fatalf("signet point NpcInfo heading = %d, want 0", got)
		}
	}
}
