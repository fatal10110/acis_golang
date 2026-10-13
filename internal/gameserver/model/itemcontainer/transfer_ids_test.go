package itemcontainer

import (
	"errors"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// idSource hands out ids in order and counts how many were taken; asked
// past its list it fails.
type idSource struct {
	ids   []int32
	taken int
}

func (s *idSource) next() (int32, error) {
	if s.taken >= len(s.ids) {
		s.taken++
		return 0, errors.New("no id left")
	}
	id := s.ids[s.taken]
	s.taken++
	return id, nil
}

// TestFreightTransferSplitTagsActiveTown: units split into a freight that
// holds no visible stack start one under the one id taken, tagged with the
// active town; a hidden other-town stack is not merged into.
func TestFreightTransferSplitTagsActiveTown(t *testing.T) {
	src := NewContainer(0x10000001, 0, freightTestTemplates())
	dst := NewFreight(0x10000002, freightTestTemplates())

	dst.ActiveLocation = 1
	townOne := dst.AddNew(freightTestStackableID, 10, 0x20000002)
	dst.ActiveLocation = 2
	inst := src.AddNew(freightTestStackableID, 5, 0x20000001)

	ids := &idSource{ids: []int32{0x30000001}}
	result, _, freed, err := src.Transfer(inst.ObjectID, 3, dst, ids.next)
	if err != nil || result == nil || result == townOne {
		t.Fatalf("Transfer() = %+v, %v, want a new town-2 stack", result, err)
	}
	if result.ObjectID != 0x30000001 || result.Count != 3 || result.LocationData != 2 || ids.taken != 1 || freed {
		t.Fatalf("split = id %#x count %d town %d, ids taken %d, freed %v; want %#x, 3, 2, 1, false",
			result.ObjectID, result.Count, result.LocationData, ids.taken, freed, 0x30000001)
	}

	// The rest joins that stack and takes no id.
	result, freedID, freed, err := src.Transfer(inst.ObjectID, 2, dst, ids.next)
	if err != nil || result == nil || result.ObjectID != 0x30000001 || result.Count != 5 {
		t.Fatalf("merge Transfer() = %+v, %v, want the town-2 stack at 5", result, err)
	}
	if ids.taken != 1 || !freed || freedID != inst.ObjectID {
		t.Fatalf("merge took %d ids, freed (%v, %#x); want 1 total and the source freed", ids.taken, freed, freedID)
	}
}

// TestTransferWithoutIDKeepsSplitOut: a split that needs a new target
// instance but is given no id source reports the failure instead of
// creating an instance under id 0.
func TestTransferWithoutIDKeepsSplitOut(t *testing.T) {
	src := newTestContainer()
	dst := NewPlayerInventory(0x10000002, testTemplates())
	inst := src.AddNew(adenaTemplateID, 100, 0x20000001)

	result, _, _, err := src.Transfer(inst.ObjectID, 40, dst, nil)
	if result != nil || !errors.Is(err, errNoObjectID) {
		t.Fatalf("Transfer() = %+v, %v; want nil and errNoObjectID", result, err)
	}
	if dst.ItemByObjectID(0) != nil || dst.Size() != 0 {
		t.Fatalf("destination holds %d items, want none", dst.Size())
	}
}

// TestTransferWholeOntoLateStackFreesSource: a whole stack moving into a
// target that gains a stack of it after the check is absorbed there, and its
// own id is reported freed so its row goes.
func TestTransferWholeOntoLateStackFreesSource(t *testing.T) {
	src := newTestContainer()
	dst := &lateStackTarget{Container: NewContainer(0x10000002, item.LocationWarehouse, testTemplates())}
	held := dst.AddNew(adenaTemplateID, 5, 0x20000002)
	inst := src.AddNew(adenaTemplateID, 100, 0x20000001)

	ids := &idSource{}
	result, freedID, freed, err := src.Transfer(inst.ObjectID, 100, dst, ids.next)
	if err != nil || result != held || held.CountValue() != 105 {
		t.Fatalf("Transfer() = %+v, %v; want the held stack at 105", result, err)
	}
	if !freed || freedID != inst.ObjectID || ids.taken != 0 {
		t.Fatalf("freed (%v, %#x), ids taken %d; want the source id freed and none taken", freed, freedID, ids.taken)
	}
}

// lateStackTarget hides its stacks from the transfer's check, as if they
// arrived just after it.
type lateStackTarget struct {
	*Container
}

func (*lateStackTarget) ItemByTemplateID(int32) *item.Instance { return nil }
