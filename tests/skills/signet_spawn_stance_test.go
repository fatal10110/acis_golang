package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// signetStanceFrame is one NpcInfo or ChangeMoveType frame of a signet
// point: its opcode and its run flag.
type signetStanceFrame struct {
	opcode  byte
	running bool
}

// TestSignetPointSpawnsWalkingThenTakesItsRunStance casts a signet: its
// effect point, like every NPC, enters the world walking and then takes its
// run stance (Npc.onSpawn -> setWalkOrRun(true)). The caster is shown its
// NpcInfo walking, ChangeMoveType to running when its run speed is not 0
// (Creature.setWalkOrRun gates on the move speed after the stance change,
// so the run speed), then its NpcInfo running.
func TestSignetPointSpawnsWalkingThenTakesItsRunStance(t *testing.T) {
	t.Parallel()
	walking := signetStanceFrame{opcode: serverpackets.OpcodeNPCInfo}
	toRun := signetStanceFrame{opcode: serverpackets.OpcodeChangeMoveType, running: true}
	running := signetStanceFrame{opcode: serverpackets.OpcodeNPCInfo, running: true}
	for _, tt := range []struct {
		name         string
		run, walk    float64
		wantSequence []signetStanceFrame
	}{
		{name: "shipped point speeds", run: 20, walk: 50, wantSequence: []signetStanceFrame{walking, toRun, running}},
		{name: "run speed 0", run: 0, walk: 50, wantSequence: []signetStanceFrame{walking, running}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			def := modelskill.Definition{
				ID: 454, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
				SkillType: "SIGNET", EffectNpcID: 13018, Radius: 180,
				Effects: []modelskill.EffectTemplate{{Name: "Signet", Count: 1, Time: 10}},
			}
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Mage", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithNPCs(npc.NewTable([]*npc.Template{{
					ID: 13018, Type: "EffectPoint", CollisionRadius: 5, RunSpeed: tt.run, WalkSpeed: tt.walk,
				}})),
				gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{def})),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, 454, 1)
			c.Send(encodeRequestGameStart(0))
			c.Read() // SSQInfo
			c.Read() // CharSelected
			c.Send(encodeEnterWorld())
			drainUntilQuiet(t, c)

			c.Send(encodeRequestMagicSkillUse(454, false, false))
			var point *npc.EffectPoint
			srv.AdvanceUntil(t, "signet effect point", func() bool {
				for _, obj := range srv.State.Objects() {
					if ep, ok := obj.(*npc.EffectPoint); ok {
						point = ep
					}
				}
				return point != nil
			})

			var got []signetStanceFrame
			for _, f := range c.ReadQueued() {
				if len(f) < 5 || (f[0] != serverpackets.OpcodeNPCInfo && f[0] != serverpackets.OpcodeChangeMoveType) {
					continue
				}
				r := wire.NewReader(f[1:])
				if r.ReadInt32() != point.ObjectID() {
					continue
				}
				fr := signetStanceFrame{opcode: f[0]}
				if f[0] == serverpackets.OpcodeChangeMoveType {
					fr.running = r.ReadInt32() == 1
					if swimming := r.ReadInt32(); swimming != 0 {
						t.Fatalf("point ChangeMoveType swimming = %d, want 0", swimming)
					}
				} else {
					for range 7 + 2 + 8 { // template .. heading, 0, C.Spd, P.Spd, speeds
						r.ReadInt32()
					}
					for range 4 { // move and attack speed multipliers, collision radius and height
						r.ReadFloat64()
					}
					for range 3 { // right hand, chest, left hand
						r.ReadInt32()
					}
					r.ReadUint8() // name above char
					fr.running = r.ReadUint8() == 1
				}
				if err := r.Err(); err != nil {
					t.Fatalf("decode point frame %#x: %v", f[0], err)
				}
				got = append(got, fr)
			}
			if len(got) != len(tt.wantSequence) {
				t.Fatalf("point spawn frames = %+v, want %+v", got, tt.wantSequence)
			}
			for i := range tt.wantSequence {
				if got[i] != tt.wantSequence[i] {
					t.Fatalf("point spawn frame %d = %+v, want %+v", i, got[i], tt.wantSequence[i])
				}
			}
		})
	}
}
