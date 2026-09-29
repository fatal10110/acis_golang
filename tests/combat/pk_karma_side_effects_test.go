package combat

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/rs/zerolog"

	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// apprenticeKnifeID is the shipped Apprentice Adventurer's Knife (rhand):
// its <cond msgId="1685"><player pkCount="0"/></cond> lets only a
// PK-free player wear it.
const apprenticeKnifeID int32 = 7818

// shippedItemTemplate loads one item template from the shared datapack,
// skipping the calling test when the datapack is not checked out next to
// the module.
func shippedItemTemplate(t *testing.T, id int32) *item.Template {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "aCis_datapack", "data", "xml", "items")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("aCis_datapack not checked out near the module root")
	}
	table, err := xmldata.LoadItemTemplates(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadItemTemplates: %v", err)
	}
	tmpl, ok := table.Get(id)
	if !ok {
		t.Fatalf("shipped item %d missing", id)
	}
	return tmpl
}

// TestPKKillUnequipsPKFreeWeaponAndEndsTheFlag has a PvP-flagged player,
// wearing the shipped PK-free knife, kill an innocent player. The kill makes
// it a PKer: after the karma announcement the knife comes off through the
// equip toggle (S1_DISARMED, the refresh, and ActionFailed for the aborted
// attack), then the PvP flag task stops and the flag resets, which the
// victim sees as a RelationChanged without the flag.
func TestPKKillUnequipsPKFreeWeaponAndEndsTheFlag(t *testing.T) {
	t.Parallel()
	knife := shippedItemTemplate(t, apprenticeKnifeID)
	if len(knife.UseConditions) != 1 || knife.UseConditions[0].MessageID != 1685 {
		t.Fatalf("shipped knife conditions = %+v, want the one pkCount clause with message 1685", knife.UseConditions)
	}
	// The fixture character's weight limit is tiny; a weightless copy keeps
	// the knife from adding a weight-penalty update to EnterWorld.
	light := *knife
	light.Weight = 0
	templates := append(gameservertest.ItemTemplates().All(), &light)
	flags := task.NewPvPFlags(task.DefaultPvPFlagOptions(), nil)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Killer", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(combatPersistence(t, killSkillDefs())),
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithPvPFlags(flags),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 42, 1)
	knifeObjID := srv.GiveItem(t, objID, apprenticeKnifeID, 1)
	victim := srv.SeedCharacterFor(t, "victim", "Victim", 1, 0)
	vc := srv.DialClient(t, "victim", 1)
	startInWorld(t, vc)
	startInWorld(t, c)
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)

	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("killer missing from world state")
	}
	inv := obj.(interface {
		Inventory() *itemcontainer.Inventory
	}).Inventory()
	c.Send(encodeUseItem(knifeObjID, false))
	srv.AdvanceUntil(t, "knife equipped", func() bool { return inv.ItemByObjectID(knifeObjID).Equipped() })
	killer := obj.(task.PvPFlagActor)
	done := make(chan struct{})
	if !srv.PlayerQueue(t, objID).Post(func() { defer close(done); flags.AddNormal(killer) }) {
		t.Fatal("killer queue closed")
	}
	<-done
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)

	selectPlayerTarget(t, c, victim.ID)
	castKillSkill(t, srv, c, objID, victim.ID, true)
	assertKarmaChangeFrames(t, c, objID, 240)
	srv.Settle(t)

	frames := readQuiet(c)
	disarmed := -1
	for i, f := range frames {
		if f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wireReader(f[1:])
		if r.ReadInt32() != serverpackets.SystemMessageS1Disarmed {
			continue
		}
		if params, typ, id := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); params != 1 || typ != serverpackets.SystemMessageParamItemName || id != apprenticeKnifeID {
			t.Fatalf("S1_DISARMED params = %d type %d item %d, want the knife", params, typ, id)
		}
		disarmed = i
		break
	}
	if disarmed < 0 {
		t.Fatalf("no S1_DISARMED for the knife among %x", opcodesOf(frames))
	}
	refresh := indexOf(frames, disarmed, serverpackets.OpcodeUserInfo, -1)
	aborted := indexOf(frames, disarmed, serverpackets.OpcodeActionFailed, -1)
	flagReset := indexOf(frames, aborted+1, serverpackets.OpcodeUserInfo, -1)
	if refresh < 0 || aborted < refresh || flagReset < 0 {
		t.Fatalf("frames after the karma change = %x, want S1_DISARMED, UserInfo, ActionFailed, then the flag reset's UserInfo", opcodesOf(frames))
	}

	if inv.ItemByObjectID(knifeObjID).Equipped() {
		t.Fatal("knife still equipped after the PK kill")
	}
	if state := obj.(interface{ PvPFlagState() task.PvPFlagState }).PvPFlagState(); state != task.PvPFlagNone {
		t.Fatalf("killer PvP flag = %v after the PK kill, want none", state)
	}
	if n := flags.Len(); n != 0 {
		t.Fatalf("PvP flag task tracks %d players after the PK kill, want none", n)
	}

	var last []byte
	for _, f := range readQuiet(vc) {
		if f[0] == serverpackets.OpcodeRelationChanged && wireReader(f[1:]).ReadInt32() == objID {
			last = f
		}
	}
	if last == nil {
		t.Fatal("victim never received the killer's RelationChanged")
	}
	r := wireReader(last[1:])
	r.ReadInt32()
	relation, _, karma := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	if pvpFlag := r.ReadInt32(); relation&serverpackets.RelationPvPFlag != 0 || pvpFlag != 0 || karma != 240 {
		t.Fatalf("victim's last RelationChanged for the killer = relation %#x karma %d flag %d, want karma 240 and no flag", relation, karma, pvpFlag)
	}
}

// opcodesOf lists each frame's opcode, for failure messages.
func opcodesOf(frames [][]byte) []byte {
	out := make([]byte, len(frames))
	for i, f := range frames {
		out[i] = f[0]
	}
	return out
}
