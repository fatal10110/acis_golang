package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestSignetPointNpcInfoTitleFollowsTheTemplate: the title in an effect
// point's NpcInfo is its title only when its template uses a server-side
// title (AbstractNpcInfo.NpcInfo); otherwise the client is sent none, the
// template's own title notwithstanding.
func TestSignetPointNpcInfoTitleFollowsTheTemplate(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		serverSide bool
		want       string
	}{
		{"server-side title is shown", true, "Marker"},
		{"no server-side title shows none", false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			def := modelskill.Definition{
				ID:            454,
				Level:         1,
				Activation:    modelskill.ActivationActive,
				Target:        modelskill.TargetSelf,
				HitTime:       500,
				ReuseDelay:    60_000,
				StaticHitTime: true,
				StaticReuse:   true,
				SkillType:     "SIGNET",
				Radius:        180,
				Effects:       []modelskill.EffectTemplate{{Name: "Signet", Count: 1, Time: 5}},
			}.WithExtra(modelskill.Extra{
				EffectNpcID: 13018,
			})
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Mage", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithNPCs(npc.NewTable([]*npc.Template{{
					ID: 13018, TemplateID: 13018, Type: "EffectPoint", CollisionRadius: 5, RunSpeed: 20, WalkSpeed: 50,
					Title: "Marker", UsingServerSideTitle: tc.serverSide,
				}})),
				gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
			)
			objID := srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, 454, 1)
			c := srv.Client
			startInWorldAmongPlayers(t, c)
			drainUntilQuiet(t, c)

			point := castSignetPoint(t, srv, c)
			infos := readObjectFrames(c, serverpackets.OpcodeNPCInfo, point.ObjectID())
			if len(infos) == 0 {
				t.Fatal("caster got no NpcInfo of the signet point")
			}
			for _, f := range infos {
				r := wire.NewReader(f[1:])
				for range 18 { // object id .. heading, 0, C.Spd, P.Spd, speeds
					r.ReadInt32()
				}
				for range 4 { // multipliers, collision radius, height
					r.ReadFloat64()
				}
				for range 3 { // right hand, chest, left hand
					r.ReadInt32()
				}
				for range 2 + 3 { // name above char, running, in combat, alike dead, summon animation
					r.ReadUint8()
				}
				r.ReadString() // name
				if title := r.ReadString(); title != tc.want || r.Err() != nil {
					t.Fatalf("signet point title = %q (%v), want %q", title, r.Err(), tc.want)
				}
			}
		})
	}
}
