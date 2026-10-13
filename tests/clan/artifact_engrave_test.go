package clan

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Reference: HolyThing (HolyThing.java) is a civilian NPC no one may attack
// or damage, whose interact does nothing, shown with ServerObjectInfo.
// CastleManager.spawnEntities places each castles.xml artifact at boot.
// Seal of Ruler (246, TAKE_CASTLE on a HOLY target) runs TakeCastle.check
// at PlayerCast.canCast, telling the defenders OPPONENT_STARTED_ENGRAVING,
// and again when it lands (TakeCastle.useSkill, clan leaders only);
// Castle.engrave tells both sides CLAN_S1_ENGRAVED_RULER, and once every
// artifact carries the clan the castle is set to it (Castle.setOwner),
// turning the siege (Siege.midVictory).

const (
	// sealOfRulerID is the shipped Seal of Ruler.
	sealOfRulerID = 246
	// gludioArtifactID is Gludio Castle's holy artifact template.
	gludioArtifactID = 35063
)

// gludioArtifact stands where castles.xml places Gludio's artifact.
var gludioArtifact = location.Location{X: -18113, Y: 107972, Z: -2480}

// artifactTemplate is Gludio's holy artifact as the datapack defines it.
func artifactTemplate() *npc.Template {
	return &npc.Template{
		ID: gludioArtifactID, TemplateID: gludioArtifactID, Type: "HolyThing", Name: "Gludio Holy Artifact",
		Level: 20, HPMax: 342, MPMax: 213, WalkSpeed: 49.5, CollisionRadius: 25, CollisionHeight: 98, Undying: true,
	}
}

// artifactZones are Gludio's battlefield around its artifact, and the
// ground around the artifact where it may be engraved.
func artifactZones(t *testing.T) *zone.Index {
	t.Helper()
	cube := func(r int) zone.Form {
		f, err := zone.NewCuboid(gludioArtifact.X-r, gludioArtifact.X+r, gludioArtifact.Y-r, gludioArtifact.Y+r, -10_000, 10_000)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	zones := zone.NewIndex()
	field := zone.NewSiege(11, cube(3000))
	field.ResidenceID = 1
	zones.Add(field)
	zones.Add(zone.NewPrayer(12, cube(300)))
	return zones
}

// engraveWorld boots the shipped castles with their sieges running. The
// founder leads Knights, registered to attack Gludio Castle, stands 50 from
// its artifact and knows Seal of Ruler; the rival leads Rivals, the lord of
// Gludio, far from it. Neither is in the world yet.
func engraveWorld(t *testing.T) *clanWorld {
	t.Helper()
	datapack.Require(t)
	castles, err := gamexml.LoadCastles(datapack.Path(t, "data", "xml", "castles.xml"))
	if err != nil {
		t.Fatalf("load castles: %v", err)
	}
	defs, _ := shippedSkillData(t)
	seal, ok := defs.Get(sealOfRulerID, 1)
	if !ok {
		t.Fatal("no shipped Seal of Ruler")
	}
	db := sqltest.SharedDB(t)
	at := location.Location{X: gludioArtifact.X + 50, Y: gludioArtifact.Y, Z: gludioArtifact.Z}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Founder", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCastles(castles),
		gameservertest.WithSieges(siege.DefaultConfig()),
		gameservertest.WithZones(artifactZones(t)),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{artifactTemplate()})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{seal}), gamesql.NewCharacterSkillStore(db))),
		seedRival(t),
		seedAllianceClans(t,
			`UPDATE clan_data SET hasCastle = 1 WHERE clan_id = `+itoa(rivalsClanID),
			`INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (1, `+itoa(knightsClanID)+`, 'ATTACKER')`,
			`UPDATE characters SET x = `+itoa(int32(at.X))+`, y = `+itoa(int32(at.Y))+`, z = `+itoa(int32(at.Z))+`, maxMp = 1000, curMp = 300 WHERE char_name = 'Founder'`,
			`INSERT INTO character_skills (char_obj_id, skill_id, skill_level, class_index)
				SELECT obj_Id, `+itoa(sealOfRulerID)+`, 1, 0 FROM characters WHERE char_name = 'Founder'`,
		),
	)
	if !srv.DrivesClock() {
		t.Skip("the three-minute engraving needs the driven clock")
	}
	w := &clanWorld{srv: srv, leader: srv.Client, leaderID: srv.SoleObjectID(t), memberID: rivalLeaderID, at: at}
	w.member = srv.DialClient(t, "player2", 1)
	return w
}

// gludioSiege returns Gludio Castle's siege and the object id of its one
// placed artifact.
func gludioSiege(t *testing.T, w *clanWorld) (*siege.Siege, int32) {
	t.Helper()
	s, ok := w.srv.Sieges.Get(1)
	if !ok {
		t.Fatal("no Gludio siege")
	}
	ids := s.Castle().ArtifactIDs()
	if len(ids) != 1 {
		t.Fatalf("Gludio artifacts placed = %v, want one", ids)
	}
	return s, ids[0]
}

// skillMessage decodes a SystemMessage frame naming a skill: its id and the
// skill's id and level.
func skillMessage(t *testing.T, frame []byte) (id, skillID, level int32) {
	t.Helper()
	r := wire.NewReader(frame[1:])
	id = r.ReadInt32()
	if n := r.ReadInt32(); n != 1 {
		t.Fatalf("message %d carries %d parameters, want the skill", id, n)
	}
	if kind := r.ReadInt32(); kind != serverpackets.SystemMessageParamSkillName {
		t.Fatalf("message %d parameter type = %d, want a skill name", id, kind)
	}
	return id, r.ReadInt32(), r.ReadInt32()
}

// TestHolyArtifactShownAndRefusedOutsideSiege has the founder enter the
// world beside Gludio's artifact, placed at boot: it is shown with
// ServerObjectInfo, not attackable and with no HP, never with NpcInfo; a
// click selects it with the neutral color; a second click opens no chat;
// Seal of Ruler cast on it while the siege is not fought is refused with
// S1_CANNOT_BE_USED alone, nothing cast.
func TestHolyArtifactShownAndRefusedOutsideSiege(t *testing.T) {
	t.Parallel()
	w := engraveWorld(t)
	_, artifactID := gludioSiege(t, w)

	frames := startInWorld(t, w.leader)
	var info []byte
	for _, f := range frames {
		if len(f) < 5 || wire.NewReader(f[1:]).ReadInt32() != artifactID {
			continue
		}
		if f[0] == serverpackets.OpcodeNPCInfo {
			t.Fatal("the artifact was shown with NpcInfo")
		}
		if f[0] == serverpackets.OpcodeServerObjectInfo {
			info = f
		}
	}
	if info == nil {
		t.Fatalf("enter-world frames %x carry no ServerObjectInfo for the artifact", opcodes(frames))
	}
	r := wire.NewReader(info[1:])
	r.ReadInt32()
	if tmpl, name, attackable := r.ReadInt32(), r.ReadString(), r.ReadInt32(); tmpl != gludioArtifactID+1_000_000 || name != "Gludio Holy Artifact" || attackable != 0 {
		t.Fatalf("ServerObjectInfo = template %d %q attackable %d, want %d %q 0", tmpl, name, attackable, gludioArtifactID+1_000_000, "Gludio Holy Artifact")
	}
	if x, y, z := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); int(x) != gludioArtifact.X || int(y) != gludioArtifact.Y || int(z) != gludioArtifact.Z {
		t.Fatalf("artifact at (%d,%d,%d), want %v", x, y, z, gludioArtifact)
	}
	r.ReadInt32()
	for range 4 {
		r.ReadFloat64()
	}
	if hp, maxHP := r.ReadInt32(), r.ReadInt32(); hp != 0 || maxHP != 0 {
		t.Fatalf("ServerObjectInfo HP = %d/%d, want 0/0", hp, maxHP)
	}

	w.leader.Send(encodeAction(artifactID, w.at))
	selected, ok := firstOpcode(drainFrames(t, w.leader), serverpackets.OpcodeMyTargetSelected)
	if !ok {
		t.Fatal("clicking the artifact selected nothing")
	}
	if r := wire.NewReader(selected[1:]); r.ReadInt32() != artifactID || r.ReadUint16() != 0 {
		t.Fatalf("MyTargetSelected = %x, want the artifact with color 0", selected)
	}
	w.leader.Send(encodeAction(artifactID, w.at))
	if _, ok := firstOpcode(drainFrames(t, w.leader), serverpackets.OpcodeNpcHtmlMessage); ok {
		t.Fatal("talking to the artifact opened a chat")
	}

	w.leader.Send(encodeRequestMagicSkillUse(sealOfRulerID))
	frames = drainFrames(t, w.leader)
	if !slices.Equal(opcodes(frames), []byte{serverpackets.OpcodeSystemMessage}) {
		t.Fatalf("Seal of Ruler outside the siege answered %x, want one SystemMessage", opcodes(frames))
	}
	if id, skill, level := skillMessage(t, frames[0]); id != serverpackets.SystemMessageS1CannotBeUsed || skill != sealOfRulerID || level != 1 {
		t.Fatalf("refusal = %d naming skill %d level %d, want %d naming %d level 1", id, skill, level, serverpackets.SystemMessageS1CannotBeUsed, sealOfRulerID)
	}
}

// TestSealOfRulerTakesCastle has the founder, leading Knights, the attacker
// of Gludio Castle under siege, engrave its one artifact: casting Seal of
// Ruler tells the lord's clan OPPONENT_STARTED_ENGRAVING; the three-minute
// cast landing tells both sides CLAN_S1_ENGRAVED_RULER naming Knights, and
// Knights owns the castle at once, stored, the siege turning: Knights owns
// its defending side and Rivals attacks.
func TestSealOfRulerTakesCastle(t *testing.T) {
	t.Parallel()
	w := engraveWorld(t)
	gludio, artifactID := gludioSiege(t, w)
	gludio.Start()
	if !gludio.InProgress() {
		t.Fatal("siege not in progress")
	}
	startInWorld(t, w.leader)
	startInWorld(t, w.member)
	drainFrames(t, w.leader)
	drainFrames(t, w.member)

	w.leader.Send(encodeAction(artifactID, w.at))
	drainFrames(t, w.leader)
	w.leader.Send(encodeRequestMagicSkillUse(sealOfRulerID))
	if _, ok := firstOpcode(drainFrames(t, w.leader), serverpackets.OpcodeMagicSkillUse); !ok {
		t.Fatal("Seal of Ruler did not start")
	}
	if ids := messages(t, drainFrames(t, w.member)); !slices.Equal(ids, []int{siege.MsgOpponentStartedEngraving}) {
		t.Fatalf("lord's clan told %v, want OPPONENT_STARTED_ENGRAVING", ids)
	}
	if gludio.Castle().OwnerID() != rivalsClanID {
		t.Fatal("the castle changed hands before the seal landed")
	}

	w.srv.Advance(t, 181*time.Second)
	engraved := func(frames [][]byte, who string) {
		t.Helper()
		for _, f := range frames {
			if f[0] != serverpackets.OpcodeSystemMessage {
				continue
			}
			if id, params := sysMsg(t, f); id == siege.MsgClanEngravedRuler {
				if !slices.Equal(params, []string{"Knights"}) {
					t.Fatalf("%s told CLAN_S1_ENGRAVED_RULER %v, want Knights", who, params)
				}
				return
			}
		}
		t.Fatalf("%s got %x with messages %v, want CLAN_S1_ENGRAVED_RULER", who, opcodes(frames), messages(t, frames))
	}
	engraved(drainFrames(t, w.leader), "the founder")
	engraved(drainFrames(t, w.member), "the lord's clan")

	if got := gludio.Castle().OwnerID(); got != knightsClanID {
		t.Fatalf("Gludio owner = %#x, want Knights", got)
	}
	if gludio.Side(knightsClanID) != siege.SideOwner || gludio.Side(rivalsClanID) != siege.SideAttacker {
		t.Fatalf("sides after the engraving: Knights %s, Rivals %s; want OWNER, ATTACKER", gludio.Side(knightsClanID), gludio.Side(rivalsClanID))
	}
	w.srv.FlushPersistence(t)
	if n := queryInt(t, w, "SELECT hasCastle FROM clan_data WHERE clan_id = ?", knightsClanID); n != 1 {
		t.Fatalf("stored Knights castle = %d, want 1", n)
	}
	if n := queryInt(t, w, "SELECT hasCastle FROM clan_data WHERE clan_id = ?", rivalsClanID); n != 0 {
		t.Fatalf("stored Rivals castle = %d, want 0", n)
	}
}
