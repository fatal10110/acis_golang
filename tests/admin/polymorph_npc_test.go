package admin

import (
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// npcLook is what an NpcInfo shows of an NPC's look.
type npcLook struct {
	template        int32
	radius, height  float64
	rightHand, left int32
	name, title     string
}

// npcInfoLook decodes the look an NpcInfo frame shows.
func npcInfoLook(f []byte) npcLook {
	r := wire.NewReader(f[1:])
	r.ReadInt32() // object id
	v := npcLook{template: r.ReadInt32() - 1_000_000}
	for range 16 { // attackable .. walk speeds
		r.ReadInt32()
	}
	r.ReadFloat64() // move multiplier
	r.ReadFloat64() // attack speed multiplier
	v.radius, v.height = r.ReadFloat64(), r.ReadFloat64()
	v.rightHand = r.ReadInt32()
	r.ReadInt32() // chest
	v.left = r.ReadInt32()
	for range 5 { // name shown .. summon animation
		r.ReadUint8()
	}
	v.name, v.title = r.ReadString(), r.ReadString()
	return v
}

// reshown requires frames to show objectID forgotten, then seen again, and
// returns the look the NpcInfo shows.
func reshown(t *testing.T, who string, frames [][]byte, objectID int32) npcLook {
	t.Helper()
	deleted := false
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeDeleteObject && frameObjectID(f) == objectID {
			deleted = true
		}
	}
	if !deleted {
		t.Fatalf("%s: frames %x do not forget %d", who, testsupport.FrameOpcodes(frames), objectID)
	}
	return npcInfoLook(morphView(t, who, frames, objectID))
}

// morphTarget is an NPC //polymorph is tried on.
type morphTarget interface {
	ObjectID() int32
	Polymorph() *npc.Template
	ServerObjectInfoSnapshot() serverpackets.NPCInfoSnapshot
}

// TestAdminPolymorphNPCTarget pins //polymorph on a selected monster and on
// a selected civilian (AdminPolymorph, Creature.polymorph and unpolymorph,
// AbstractNpcInfo.NpcInfo): everyone around forgets the NPC (decayMe) and
// sees it again (spawnMe) as the NPC the id names, its template id, held
// items and body, under its own name and title; a player coming in range
// later sees the form too, while the stationary view (ServerObjectInfo)
// never takes it. A second //polymorph shows the NPC as itself again
// whatever follows. The GM gets no message of its own, and forgetting the
// NPC drops its selection. An id none or
// unreadable gets the usage, one no NPC has the refusal, and either leaves
// the NPC as it is.
func TestAdminPolymorphNPCTarget(t *testing.T) {
	t.Parallel()
	own := func(id int, kind string) *npc.Template {
		return &npc.Template{
			ID: id, TemplateID: id, Type: kind, Name: "Own " + kind, Title: "Own title", Level: 1, HPMax: 1000,
			AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
			RightHand: 7, LeftHand: 8, UsingServerSideName: true, UsingServerSideTitle: true,
		}
	}
	for _, tc := range []struct {
		kind  string
		spawn func(*testing.T, *gameservertest.Server, *npc.Template, location.Location) morphTarget
	}{
		{"Monster", func(t *testing.T, srv *gameservertest.Server, tmpl *npc.Template, at location.Location) morphTarget {
			return srv.SpawnHostileNPCTemplateAt(t, tmpl, at)
		}},
		{"Folk", func(t *testing.T, srv *gameservertest.Server, tmpl *npc.Template, at location.Location) morphTarget {
			return srv.SpawnFolkNPCAt(t, tmpl, at)
		}},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			t.Parallel()
			srv, gmID := bootAdmin(t, adminLevel, polymorphNPCs())
			gm := srv.Client
			enterWorld(t, gm)
			tmpl := own(100, tc.kind)
			target := tc.spawn(t, srv, tmpl, location.Location{X: spawnX + 40, Y: spawnY, Z: spawnZ})
			id := target.ObjectID()
			watcher, _ := addPlayer(t, srv, "player2", "Watcher", userLevel)
			exchange(t, gm, encodeAction(id))
			settle(t, watcher)
			itself := npcLook{
				template: int32(tmpl.TemplateID), radius: tmpl.CollisionRadius, height: tmpl.CollisionHeight,
				rightHand: int32(tmpl.RightHand), left: int32(tmpl.LeftHand), name: tmpl.Name, title: tmpl.Title,
			}
			form := npcLook{
				template: polymorphNPCID + 1, radius: polymorphForm.CollisionRadius, height: polymorphForm.CollisionHeight,
				rightHand: polymorphForm.RightHand, left: polymorphForm.LeftHand, name: tmpl.Name, title: tmpl.Title,
			}

			frames := exchange(t, gm, encodeBuildCmd("polymorph "+strconv.Itoa(polymorphNPCID)))
			if got := reshown(t, "GM", frames, id); got != form {
				t.Fatalf("GM sees the polymorphed NPC as %+v, want %+v", got, form)
			}
			for _, f := range frames {
				if f[0] == serverpackets.OpcodeSystemMessage {
					t.Fatalf("GM frames %x answer the polymorph", testsupport.FrameOpcodes(frames))
				}
			}
			if got := reshown(t, "Watcher", settle(t, watcher), id); got != form {
				t.Fatalf("Watcher sees the polymorphed NPC as %+v, want %+v", got, form)
			}
			if got := target.Polymorph(); got == nil || got.ID != polymorphNPCID {
				t.Fatalf("NPC form = %+v, want NPC %d", got, polymorphNPCID)
			}
			if s := target.ServerObjectInfoSnapshot(); s.TemplateID != tmpl.TemplateID || s.CollisionRadius != tmpl.CollisionRadius || s.CollisionHeight != tmpl.CollisionHeight {
				t.Fatalf("stationary view %d radius %v height %v, want the NPC's own %d %v %v",
					s.TemplateID, s.CollisionRadius, s.CollisionHeight, tmpl.TemplateID, tmpl.CollisionRadius, tmpl.CollisionHeight)
			}
			requirePolymorph(t, srv, gmID, 0)

			// A player coming in range later sees the form.
			srv.SeedCharacterFor(t, "player3", "Newcomer", 1, 0)
			newcomer := srv.DialClient(t, "player3", 1)
			if got := npcInfoLook(morphView(t, "Newcomer", append(enterWorldFrames(t, newcomer), settle(t, newcomer)...), id)); got != form {
				t.Fatalf("Newcomer sees the polymorphed NPC as %+v, want %+v", got, form)
			}
			// Forgetting the NPC dropped the GM's selection (removeKnownObject).
			exchange(t, gm, encodeAction(id))
			settle(t, watcher)

			// The toggle ignores what follows, even an id no NPC has.
			frames = exchange(t, gm, encodeBuildCmd("polymorph 99999"))
			if got := reshown(t, "GM", frames, id); got != itself {
				t.Fatalf("GM sees the unpolymorphed NPC as %+v, want %+v", got, itself)
			}
			if got := reshown(t, "Watcher", settle(t, watcher), id); got != itself {
				t.Fatalf("Watcher sees the unpolymorphed NPC as %+v, want %+v", got, itself)
			}
			if got := target.Polymorph(); got != nil {
				t.Fatalf("NPC form after the toggle = %+v, want none", got)
			}

			exchange(t, gm, encodeAction(id))
			frames = exchange(t, gm, encodeBuildCmd("polymorph 99999"))
			if len(frames) != 1 {
				t.Fatalf("unknown id on an NPC frames = %x, want one message", testsupport.FrameOpcodes(frames))
			}
			assertStatic(t, frames[0], serverpackets.SystemMessageApplicantInformationIncorrect)
			assertTexts(t, exchange(t, gm, encodeBuildCmd("polymorph")), adminPolymorphUsageText)
			assertTexts(t, exchange(t, gm, encodeBuildCmd("polymorph x")), adminPolymorphUsageText)
			for _, f := range settle(t, watcher) {
				if (f[0] == serverpackets.OpcodeNPCInfo || f[0] == serverpackets.OpcodeDeleteObject) && frameObjectID(f) == id {
					t.Fatalf("Watcher frame %x shows the NPC changed by a refused polymorph", f)
				}
			}
			if target.Polymorph() != nil {
				t.Fatal("a refused polymorph gave the NPC a form")
			}
		})
	}
}
