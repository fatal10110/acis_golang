package pets

import (
	"encoding/binary"
	"strconv"
	"testing"
	"time"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// TestObserveRefusedWithSummonOut pins the summon gate of a viewpoint: a
// castle viewpoint answers NO_OBSERVE_WITH_PET, any other turns the owner
// away without a word; each is then released, and the owner stays with
// its servitor.
func TestObserveRefusedWithSummonOut(t *testing.T) {
	t.Parallel()
	groups, err := gamexml.LoadObserverGroups(datapack.Path(t, "data", "xml", "observerGroups.xml"))
	if err != nil {
		t.Fatalf("load observer groups: %v", err)
	}
	o := bootServitorOwner(t, gameservertest.WithObserverGroups(groups), gameservertest.WithReuseDelays(3*time.Second, 0))
	o.summonServitor(t)
	x, y, z := o.srv.PlayerPosition(t, o.id)
	tower := o.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", 31031), location.Location{X: x + 30, Y: y, Z: z})
	tower.SetObserverGroups([]int{612, 618})
	drainUntilQuiet(t, o.client)
	o.client.Send(encodeAction(tower.ObjectID(), int32(x), int32(y), int32(z), false))
	drainUntilQuiet(t, o.client)
	prefix := "npc_" + strconv.Itoa(int(tower.ObjectID())) + "_"

	for _, tc := range []struct {
		group, seat string
		want        []byte
	}{
		{"612", "620", []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed}},
		{"618", "632", []byte{serverpackets.OpcodeActionFailed}},
	} {
		// Talk to the tower again, for its group links.
		o.client.Send(encodeAction(tower.ObjectID(), int32(x), int32(y), int32(z), false))
		drainUntilQuiet(t, o.client)
		o.client.Send(encodeBypass(prefix + "observe_group " + tc.group))
		drainUntilQuiet(t, o.client)
		o.client.Send(encodeBypass(prefix + "observe " + tc.seat))
		frames := drainFrames(t, o.client)
		got := make([]byte, 0, len(frames))
		for _, f := range frames {
			got = append(got, f[0])
		}
		if string(got) != string(tc.want) {
			t.Fatalf("observe %s with a servitor out = %x, want %x", tc.seat, got, tc.want)
		}
		if tc.seat == "620" {
			if id := binary.LittleEndian.Uint32(frames[0][1:5]); id != serverpackets.SystemMessageNoObserveWithPet {
				t.Fatalf("castle viewpoint message = %d, want NO_OBSERVE_WITH_PET", id)
			}
		}
	}
	if _, ok := o.srv.State.Summon(o.id); !ok {
		t.Fatal("servitor gone after the refusals")
	}
}
