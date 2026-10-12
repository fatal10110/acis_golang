package clan

import (
	"slices"
	"testing"
	"time"

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
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Reference: TakeCastle.check refuses, in this order: no castle's
// battlefield under the caster (S1_CANNOT_BE_USED), a target that is not
// one of that castle's artifacts (INVALID_TARGET), the siege not fought
// (S1_CANNOT_BE_USED), 200 or farther from the artifact
// (DIST_TOO_FAR_CASTING_STOPPED), outside the CAST_ON_ARTIFACT zone or not
// on the attacking side (S1_CANNOT_BE_USED). TakeCastle.useSkill engraves
// only for a clan leader, and only once the check passes again.

const (
	// dionArtifactID is Dion Castle's holy artifact template.
	dionArtifactID = 35105

	strayID          int32 = 0x7f200301
	engraverSquireID int32 = 0x7f200302
	pageID           int32 = 0x7f200303
)

// The refusal world's ground, around Gludio's artifact A: the founder
// stands at A+(50,0), the rival at A-(50,0), the stray at A+(0,80), off
// Gludio's battlefield, the squire at A-(0,60), on the battlefield but off
// the engraving ground, and the page at A+(-30,30), on both. Dion's
// artifact copy stands at A+(40,60), within reach of the founder and the
// stray.
var (
	founderSpot = location.Location{X: gludioArtifact.X + 50, Y: gludioArtifact.Y, Z: gludioArtifact.Z}
	rivalSpot   = location.Location{X: gludioArtifact.X - 50, Y: gludioArtifact.Y, Z: gludioArtifact.Z}
	straySpot   = location.Location{X: gludioArtifact.X, Y: gludioArtifact.Y + 80, Z: gludioArtifact.Z}
	squireSpot  = location.Location{X: gludioArtifact.X, Y: gludioArtifact.Y - 60, Z: gludioArtifact.Z}
	pageSpot    = location.Location{X: gludioArtifact.X - 30, Y: gludioArtifact.Y + 30, Z: gludioArtifact.Z}
	dionSpot    = location.Location{X: gludioArtifact.X + 40, Y: gludioArtifact.Y + 60, Z: gludioArtifact.Z}
)

// refusalZones are Gludio's battlefield, ending 70 north of its artifact,
// and the engraving ground, starting 40 south of it.
func refusalZones(t *testing.T) *zone.Index {
	t.Helper()
	cube := func(x0, x1, y0, y1 int) zone.Form {
		f, err := zone.NewCuboid(gludioArtifact.X+x0, gludioArtifact.X+x1, gludioArtifact.Y+y0, gludioArtifact.Y+y1, -10_000, 10_000)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	zones := zone.NewIndex()
	field := zone.NewSiege(11, cube(-3000, 3000, -3000, 70))
	field.ResidenceID = 1
	zones.Add(field)
	zones.Add(zone.NewPrayer(12, cube(-300, 300, -40, 300)))
	return zones
}

// refusalWorld is a booted server over the shipped castles: Knights,
// registered to attack Gludio Castle, led by the founder with the stray,
// the squire and the page as members; Rivals, the lord of Gludio, led by
// the rival. Everyone knows Seal of Ruler and is in the world at its spot,
// and a copy of Dion's artifact stands beside Gludio's.
type refusalWorld struct {
	*clanWorld
	stray, squire, page *testsupport.ScriptedClient
	dion                *npc.Folk
}

func bootRefusalWorld(t *testing.T) *refusalWorld {
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
	dion := &npc.Template{
		ID: dionArtifactID, TemplateID: dionArtifactID, Type: "HolyThing", Name: "Dion Holy Artifact",
		Level: 20, HPMax: 342, MPMax: 213, WalkSpeed: 49.5, CollisionRadius: 25, CollisionHeight: 98, Undying: true,
	}
	place := func(where string, at location.Location) string {
		return `UPDATE characters SET x = ` + itoa(int32(at.X)) + `, y = ` + itoa(int32(at.Y)) + `, z = ` + itoa(int32(at.Z)) + `, curMp = 300 WHERE ` + where
	}
	member := func(id int32) string {
		return `UPDATE characters SET clanid = ` + itoa(knightsClanID) + `, power_grade = 6 WHERE obj_Id = ` + itoa(id)
	}
	db := sqltest.SharedDB(t)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Founder", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCastles(castles),
		gameservertest.WithSieges(siege.DefaultConfig()),
		gameservertest.WithZones(refusalZones(t)),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{artifactTemplate(), dion})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{seal}), gamesql.NewCharacterSkillStore(db))),
		seedRival(t,
			castMember{strayID, "player3", "Stray"},
			castMember{engraverSquireID, "player4", "Squire"},
			castMember{pageID, "player5", "Page"},
		),
		seedAllianceClans(t,
			`UPDATE clan_data SET hasCastle = 1 WHERE clan_id = `+itoa(rivalsClanID),
			`INSERT INTO siege_clans (castle_id, clan_id, type) VALUES (1, `+itoa(knightsClanID)+`, 'ATTACKER')`,
			member(strayID), member(engraverSquireID), member(pageID),
			place(`char_name = 'Founder'`, founderSpot),
			place(`obj_Id = `+itoa(rivalLeaderID), rivalSpot),
			place(`obj_Id = `+itoa(strayID), straySpot),
			place(`obj_Id = `+itoa(engraverSquireID), squireSpot),
			place(`obj_Id = `+itoa(pageID), pageSpot),
			`INSERT INTO character_skills (char_obj_id, skill_id, skill_level, class_index)
				SELECT obj_Id, `+itoa(sealOfRulerID)+`, 1, 0 FROM characters`,
		),
	)
	if !srv.DrivesClock() {
		t.Skip("the three-minute engraving needs the driven clock")
	}
	w := &refusalWorld{clanWorld: &clanWorld{srv: srv, leader: srv.Client, leaderID: srv.SoleObjectID(t), memberID: rivalLeaderID, at: founderSpot}}
	w.member = srv.DialClient(t, "player2", 1)
	w.stray = srv.DialClient(t, "player3", 1)
	w.squire = srv.DialClient(t, "player4", 1)
	w.page = srv.DialClient(t, "player5", 1)
	w.dion = srv.SpawnFolkNPCAt(t, dion, dionSpot)
	for _, c := range w.all() {
		startInWorld(t, c)
	}
	w.drainAll(t)
	return w
}

func (w *refusalWorld) all() []*testsupport.ScriptedClient {
	return []*testsupport.ScriptedClient{w.leader, w.member, w.stray, w.squire, w.page}
}

func (w *refusalWorld) drainAll(t *testing.T) {
	t.Helper()
	for _, c := range w.all() {
		drainFrames(t, c)
	}
}

// seal has c, standing at at, select target and cast Seal of Ruler on it,
// and returns what c is answered.
func (w *refusalWorld) seal(t *testing.T, c *testsupport.ScriptedClient, at location.Location, target int32) [][]byte {
	t.Helper()
	c.Send(encodeAction(target, at))
	if _, ok := firstOpcode(drainFrames(t, c), serverpackets.OpcodeMyTargetSelected); !ok {
		t.Fatalf("selecting %d selected nothing", target)
	}
	c.Send(encodeRequestMagicSkillUse(sealOfRulerID))
	return drainFrames(t, c)
}

// refused fails unless frames are one SystemMessage, want: the skill named
// for S1_CANNOT_BE_USED, no parameter otherwise.
func refused(t *testing.T, who string, frames [][]byte, want int32) {
	t.Helper()
	if !slices.Equal(opcodes(frames), []byte{serverpackets.OpcodeSystemMessage}) {
		t.Fatalf("%s answered %x, want one SystemMessage", who, opcodes(frames))
	}
	if want == serverpackets.SystemMessageS1CannotBeUsed {
		if id, skill, level := skillMessage(t, frames[0]); id != want || skill != sealOfRulerID || level != 1 {
			t.Fatalf("%s refused with %d naming skill %d level %d, want %d naming %d level 1", who, id, skill, level, want, sealOfRulerID)
		}
		return
	}
	if id, params := sysMsg(t, frames[0]); id != int(want) || len(params) != 0 {
		t.Fatalf("%s refused with %d %v, want %d alone", who, id, params, want)
	}
}

// TestSealOfRulerCastStartRefusalOrder casts Seal of Ruler where two of the
// castle check's refusals hold at once, the earlier one answering alone:
// off the battlefield, Dion's artifact is not refused as INVALID_TARGET;
// on it, Dion's artifact is INVALID_TARGET though the siege is not fought;
// with the siege fought, a Knights member off the engraving ground and the
// lord on it are refused S1_CANNOT_BE_USED. No refusal starts a cast or
// tells the lord's clan anything, and the castle stays Rivals'.
func TestSealOfRulerCastStartRefusalOrder(t *testing.T) {
	t.Parallel()
	w := bootRefusalWorld(t)
	gludio, artifactID := gludioSiege(t, w.clanWorld)
	dionID := w.dion.ObjectID()

	refused(t, "the stray off the battlefield on Dion's artifact", w.seal(t, w.stray, straySpot, dionID), serverpackets.SystemMessageS1CannotBeUsed)
	refused(t, "the founder on Dion's artifact outside the siege", w.seal(t, w.leader, founderSpot, dionID), serverpackets.SystemMessageInvalidTarget)

	gludio.Start()
	if !gludio.InProgress() {
		t.Fatal("siege not in progress")
	}
	w.drainAll(t)
	refused(t, "the squire off the engraving ground", w.seal(t, w.squire, squireSpot, artifactID), serverpackets.SystemMessageS1CannotBeUsed)
	refused(t, "the lord", w.seal(t, w.member, rivalSpot, artifactID), serverpackets.SystemMessageS1CannotBeUsed)
	if ids := messages(t, drainFrames(t, w.member)); len(ids) != 0 {
		t.Fatalf("lord's clan told %v by refused casts, want nothing", ids)
	}
	if got := gludio.Castle().OwnerID(); got != rivalsClanID {
		t.Fatalf("Gludio owner = %#x, want Rivals", got)
	}
}

// TestSealOfRulerLandingTakesNothing has two Seals of Ruler land on
// Gludio's artifact without taking the castle: the page's, whose cast
// started (the lord's clan told the opponents started engraving) but who
// leads no clan, lands telling no one anything; the founder's, whose siege
// ends before it lands. Gludio stays Rivals', in the world and stored.
func TestSealOfRulerLandingTakesNothing(t *testing.T) {
	t.Parallel()
	w := bootRefusalWorld(t)
	gludio, artifactID := gludioSiege(t, w.clanWorld)
	gludio.Start()
	w.drainAll(t)

	if _, ok := firstOpcode(w.seal(t, w.page, pageSpot, artifactID), serverpackets.OpcodeMagicSkillUse); !ok {
		t.Fatal("the page's Seal of Ruler did not start")
	}
	if ids := messages(t, drainFrames(t, w.member)); !slices.Equal(ids, []int{siege.MsgOpponentStartedEngraving}) {
		t.Fatalf("lord's clan told %v, want OPPONENT_STARTED_ENGRAVING", ids)
	}
	w.drainAll(t)
	w.srv.Advance(t, 181*time.Second)
	if ids := messages(t, drainFrames(t, w.page)); len(ids) != 0 {
		t.Fatalf("the page's landing told it %v, want nothing", ids)
	}
	for i, c := range w.all() {
		if ids := messages(t, drainFrames(t, c)); slices.Contains(ids, siege.MsgClanEngravedRuler) {
			t.Fatalf("client %d told %v by the page's landing, want no CLAN_S1_ENGRAVED_RULER", i, ids)
		}
	}
	if got := gludio.Castle().OwnerID(); got != rivalsClanID {
		t.Fatalf("Gludio owner after the page's landing = %#x, want Rivals", got)
	}

	if _, ok := firstOpcode(w.seal(t, w.leader, founderSpot, artifactID), serverpackets.OpcodeMagicSkillUse); !ok {
		t.Fatal("the founder's Seal of Ruler did not start")
	}
	w.srv.Advance(t, 60*time.Second)
	gludio.End()
	w.drainAll(t)
	w.srv.Advance(t, 121*time.Second)
	// The landing's castle check finds the siege over: the founder is told
	// the reason alone, and nothing is engraved.
	var refusals [][]byte
	for _, f := range drainFrames(t, w.leader) {
		if f[0] == serverpackets.OpcodeSystemMessage {
			refusals = append(refusals, f)
		}
	}
	if len(refusals) != 1 {
		t.Fatalf("the founder's landing after the end told %v, want S1_CANNOT_BE_USED alone", messages(t, refusals))
	}
	if id, skill, level := skillMessage(t, refusals[0]); id != serverpackets.SystemMessageS1CannotBeUsed || skill != sealOfRulerID || level != 1 {
		t.Fatalf("landing refusal = %d naming skill %d level %d, want %d naming %d level 1", id, skill, level, serverpackets.SystemMessageS1CannotBeUsed, sealOfRulerID)
	}
	for i, c := range w.all()[1:] {
		if ids := messages(t, drainFrames(t, c)); slices.Contains(ids, siege.MsgClanEngravedRuler) {
			t.Fatalf("client %d told %v after the siege ended, want no CLAN_S1_ENGRAVED_RULER", i+1, ids)
		}
	}
	if got := gludio.Castle().OwnerID(); got != rivalsClanID {
		t.Fatalf("Gludio owner after the siege ended mid-cast = %#x, want Rivals", got)
	}
	w.srv.FlushPersistence(t)
	if n := queryInt(t, w.clanWorld, "SELECT hasCastle FROM clan_data WHERE clan_id = ?", knightsClanID); n != 0 {
		t.Fatalf("stored Knights castle = %d, want 0", n)
	}
	if n := queryInt(t, w.clanWorld, "SELECT hasCastle FROM clan_data WHERE clan_id = ?", rivalsClanID); n != 1 {
		t.Fatalf("stored Rivals castle = %d, want 1", n)
	}
}
