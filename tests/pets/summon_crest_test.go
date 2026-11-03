package pets

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// npcs.properties ShowSummonCrest (stock False): a summon's NpcInfo carries
// its owner's clan id, clan crest, alliance id and alliance crest, and the
// owner, whose own pet window shows no crest, is sent that NpcInfo too as
// the summon spawns, ahead of its RelationChanged, and as it lands from a
// teleport. Without the switch the four fields stay 0 and the owner gets no
// NpcInfo of its summon.

const (
	summonCrestClanID = 0x10000093
	summonCrestID     = 0x2011
	summonAllyCrestID = 0x2012
)

var summonOwnerCrest = [4]int32{summonCrestClanID, summonCrestID, summonCrestClanID, summonAllyCrestID}

// summonCrestClanSeed makes Owner the leader of a clan bearing a crest and
// leading an alliance bearing one.
func summonCrestClanSeed(t *testing.T) func(db *sql.DB) {
	t.Helper()
	return func(db *sql.DB) {
		ctx := context.Background()
		if _, err := db.ExecContext(ctx, "UPDATE characters SET clanid = ? WHERE char_name = 'Owner'", summonCrestClanID); err != nil {
			t.Fatalf("seed clan member: %v", err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO clan_data (clan_id, clan_name, clan_level, ally_id, ally_name, leader_id, crest_id, ally_crest_id)
			SELECT ?, 'Tamers', 5, ?, 'Pack', obj_Id, ?, ? FROM characters WHERE char_name = 'Owner'`,
			summonCrestClanID, summonCrestClanID, summonCrestID, summonAllyCrestID); err != nil {
			t.Fatalf("seed clan: %v", err)
		}
	}
}

// summonCrestFields decodes an NpcInfo frame's clan id, clan crest,
// alliance id and alliance crest.
func summonCrestFields(t *testing.T, frame []byte) [4]int32 {
	t.Helper()
	r := wire.NewReader(frame[1:])
	for range 18 { // object id .. walk speeds
		r.ReadInt32()
	}
	for range 4 { // speed multipliers, collision radius and height
		r.ReadFloat64()
	}
	for range 3 { // right hand, chest, left hand
		r.ReadInt32()
	}
	for range 5 { // name shown .. summon animation
		r.ReadUint8()
	}
	r.ReadString()
	r.ReadString()
	for range 4 { // summon, PvP flag, karma, abnormal effect
		r.ReadInt32()
	}
	return [4]int32{r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()}
}

// framesAbout returns the index and frame of every frame of opcode whose
// leading object id is objectID.
func framesAbout(frames [][]byte, opcode byte, objectID int32) (at []int, out [][]byte) {
	for i, f := range frames {
		if f[0] == opcode && len(f) >= 5 && wire.NewReader(f[1:]).ReadInt32() == objectID {
			at, out = append(at, i), append(out, f)
		}
	}
	return at, out
}

// summonCrestWorld is the clan-leading owner, a clanless observer beside
// it, and the owner's servitor once called.
type summonCrestWorld struct {
	*servitorOwner
	observer *testsupport.ScriptedClient
}

func bootSummonCrest(t *testing.T, extra ...gameservertest.Option) *summonCrestWorld {
	t.Helper()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{{
		ID: killableServitorSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "SUMMON", NpcID: killableServitorNPCID, SummonTotalLifeTime: 1_200_000,
		SummonTimeLostIdle: 500, SummonTimeLostActive: 1000, StaticHitTime: true, StaticReuse: true,
	}}), gamesql.NewCharacterSkillStore(db))
	cat := &npc.Template{
		ID: killableServitorNPCID, TemplateID: killableServitorNPCID, Type: "Servitor", Name: "Kat the Cat", Level: 20,
		HPMax: 500, MPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}
	// The clan keeps a crest only if its file is on disk.
	dir := t.TempDir()
	for name, size := range map[string]int{
		"Crest_" + strconv.Itoa(summonCrestID) + ".dds":         256,
		"AllyCrest_" + strconv.Itoa(summonAllyCrestID) + ".dds": 192,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), make([]byte, size), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	crests, _, err := datacache.LoadCrests(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv := bootPets(t, append([]gameservertest.Option{
		gameservertest.WithCrests(crests),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{cat})),
		gameservertest.WithSkills(skills),
		gameservertest.WithClanSeed(summonCrestClanSeed(t)),
	}, extra...)...)
	ownerID := srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), ownerID, 0, killableServitorSkill, 1); err != nil {
		t.Fatalf("seed known skill: %v", err)
	}
	// The clan leader's enter-world burst carries its clan's packets too.
	enterLenient(t, srv.Client)
	srv.SeedCharacterFor(t, "player2", "Watcher", 1, 0)
	observer := srv.DialClient(t, "player2", 1)
	enterLenient(t, observer)
	drainUntilQuiet(t, srv.Client)
	return &summonCrestWorld{servitorOwner: &servitorOwner{srv: srv, client: srv.Client, id: ownerID}, observer: observer}
}

// enterLenient selects the first character and enters the world, reading
// past the whole burst.
func enterLenient(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	drainUntilQuiet(t, c)
}

// call casts the servitor summon and returns the servitor with what the
// owner was sent meanwhile.
func (w *summonCrestWorld) call(t *testing.T) (*summon.Actor, [][]byte) {
	t.Helper()
	w.client.Send(encodeRequestMagicSkillUse(killableServitorSkill))
	var servitor *summon.Actor
	w.srv.AdvanceUntil(t, "servitor in world state", func() bool {
		obj, ok := w.srv.State.Summon(w.id)
		if ok {
			servitor, ok = obj.(*summon.Actor)
		}
		return ok
	})
	w.srv.Settle(t)
	return servitor, w.client.ReadQueued()
}

func TestSummonCrestShownWithShowSummonCrest(t *testing.T) {
	t.Parallel()
	w := bootSummonCrest(t, gameservertest.WithShowSummonCrest())
	servitor, frames := w.call(t)
	id := servitor.ObjectID()

	infoAt, infos := framesAbout(frames, serverpackets.OpcodeNPCInfo, id)
	relationAt, _ := framesAbout(frames, serverpackets.OpcodeRelationChanged, id)
	if len(infos) != 1 || len(relationAt) == 0 {
		t.Fatalf("owner got %d NpcInfo and %d RelationChanged of its servitor, want one NpcInfo and its RelationChanged", len(infos), len(relationAt))
	}
	if infoAt[0] > relationAt[0] {
		t.Fatalf("owner's NpcInfo of its servitor came at %d, after its RelationChanged at %d", infoAt[0], relationAt[0])
	}
	if got := summonCrestFields(t, infos[0]); got != summonOwnerCrest {
		t.Fatalf("owner's NpcInfo crest fields = %#x, want %#x", got, summonOwnerCrest)
	}
	_, seen := framesAbout(w.observer.ReadQueued(), serverpackets.OpcodeNPCInfo, id)
	if len(seen) == 0 {
		t.Fatal("observer got no NpcInfo of the servitor")
	}
	if got := summonCrestFields(t, seen[0]); got != summonOwnerCrest {
		t.Fatalf("observer's NpcInfo crest fields = %#x, want %#x", got, summonOwnerCrest)
	}

	// The owner teleports; the servitor lands with it and the owner is shown
	// it again.
	obj, ok := w.srv.State.Player(w.id)
	if !ok {
		t.Fatal("owner missing from the world")
	}
	owner, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("owner = %T, want an online character", obj)
	}
	x, y, z := w.srv.PlayerPosition(t, w.id)
	owner.TeleportTo(x+3000, y, z, 0)
	readUntilOpcode(t, w.client, serverpackets.OpcodeTeleportToLocation, "owner TeleportToLocation")
	drainUntilQuiet(t, w.client)
	w.client.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	w.srv.Settle(t)
	_, landed := framesAbout(w.client.ReadQueued(), serverpackets.OpcodeNPCInfo, id)
	if len(landed) != 1 {
		t.Fatalf("owner got %d NpcInfo of its servitor on landing, want 1", len(landed))
	}
	if got := summonCrestFields(t, landed[0]); got != summonOwnerCrest {
		t.Fatalf("owner's landing NpcInfo crest fields = %#x, want %#x", got, summonOwnerCrest)
	}
}

func TestSummonCrestHiddenWithoutShowSummonCrest(t *testing.T) {
	t.Parallel()
	w := bootSummonCrest(t)
	servitor, frames := w.call(t)
	id := servitor.ObjectID()

	if _, infos := framesAbout(frames, serverpackets.OpcodeNPCInfo, id); len(infos) != 0 {
		t.Fatalf("owner got %d NpcInfo of its servitor, want none", len(infos))
	}
	_, seen := framesAbout(w.observer.ReadQueued(), serverpackets.OpcodeNPCInfo, id)
	if len(seen) == 0 {
		t.Fatal("observer got no NpcInfo of the servitor")
	}
	if got := summonCrestFields(t, seen[0]); got != ([4]int32{}) {
		t.Fatalf("observer's NpcInfo crest fields = %#x, want all 0", got)
	}
}
