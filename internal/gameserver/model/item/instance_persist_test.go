package item

import "testing"

func newPersistTestInstance() *Instance {
	return &Instance{
		ObjectID:     0x20000001,
		TemplateID:   57,
		OwnerID:      1,
		Count:        10,
		EnchantLevel: 3,
		Location:     LocationInventory,
		LocationData: 0,
		ManaLeft:     100,
	}
}

// TestInstancePersistNotifier pins which mutations schedule a database
// write. A mutation that changes persisted state must report it; one that
// leaves the stored row identical must not, so an unchanged item doesn't
// keep getting rewritten.
func TestInstancePersistNotifier(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*Instance)
		mutate func(*Instance)
		want   int
	}{
		{name: "add count", mutate: func(inst *Instance) { inst.AddCount(5) }, want: 1},
		{name: "add no count", mutate: func(inst *Instance) { inst.AddCount(0) }, want: 0},
		{name: "reduce count", mutate: func(inst *Instance) { inst.ReduceCount(4) }, want: 1},
		{name: "reduce more than held", mutate: func(inst *Instance) { inst.ReduceCount(11) }, want: 0},
		{name: "reduce nothing", mutate: func(inst *Instance) { inst.ReduceCount(0) }, want: 0},
		{name: "destroy", mutate: func(inst *Instance) { inst.DestroyState() }, want: 1},
		{
			name:   "set owner location",
			mutate: func(inst *Instance) { inst.SetOwnerLocation(2, LocationWarehouse, 4) },
			want:   1,
		},
		{
			name:   "set same owner location",
			mutate: func(inst *Instance) { inst.SetOwnerLocation(1, LocationInventory, 0) },
			want:   0,
		},
		{name: "set location", mutate: func(inst *Instance) { inst.SetLocation(LocationPaperdoll, 7) }, want: 1},
		{name: "set same location", mutate: func(inst *Instance) { inst.SetLocation(LocationInventory, 0) }, want: 0},
		{name: "set enchant level", mutate: func(inst *Instance) { inst.SetEnchantLevel(4) }, want: 1},
		{name: "set same enchant level", mutate: func(inst *Instance) { inst.SetEnchantLevel(3) }, want: 0},
		{name: "decrease mana", mutate: func(inst *Instance) { inst.DecreaseMana(60) }, want: 1},
		{name: "decrease no mana", mutate: func(inst *Instance) { inst.DecreaseMana(0) }, want: 0},
		{
			name:   "decrease mana on a non-shadow item",
			setup:  func(inst *Instance) { inst.ManaLeft = -1 },
			mutate: func(inst *Instance) { inst.DecreaseMana(60) },
			want:   0,
		},
		{
			name:   "charging a shot is transient",
			mutate: func(inst *Instance) { inst.SetChargedShot(ShotSoul, true) },
			want:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst := newPersistTestInstance()
			if tt.setup != nil {
				tt.setup(inst)
			}

			notified := 0
			inst.SetPersistNotifier(func(got *Instance) {
				if got != inst {
					t.Errorf("notifier got instance %p, want %p", got, inst)
				}
				notified++
			})

			tt.mutate(inst)

			if notified != tt.want {
				t.Errorf("notifications = %d, want %d", notified, tt.want)
			}
		})
	}
}

// TestInstancePersistNotifierCleared proves the hook is releasable: an
// instance whose owner detached must stop scheduling writes.
func TestInstancePersistNotifierCleared(t *testing.T) {
	inst := newPersistTestInstance()

	notified := 0
	inst.SetPersistNotifier(func(*Instance) { notified++ })
	inst.AddCount(1)
	if notified != 1 {
		t.Fatalf("notifications before clearing = %d, want 1", notified)
	}

	inst.SetPersistNotifier(nil)
	inst.AddCount(1)
	if notified != 1 {
		t.Errorf("notifications after clearing = %d, want 1", notified)
	}
}

// TestInstancePersistNotifierUnset covers the default: an instance nothing
// persists mutates silently, so domain code needs no persistence layer.
func TestInstancePersistNotifierUnset(t *testing.T) {
	inst := newPersistTestInstance()
	inst.AddCount(1)
	inst.DestroyState()
}

// TestInstanceSnapshotDropsPersistNotifier proves a detached copy doesn't
// inherit the live instance's hook: persisting a snapshot must never
// schedule further writes through the original's owner.
func TestInstanceSnapshotDropsPersistNotifier(t *testing.T) {
	inst := newPersistTestInstance()
	notified := 0
	inst.SetPersistNotifier(func(*Instance) { notified++ })

	clone := inst.Clone()
	clone.AddCount(5)

	if notified != 0 {
		t.Errorf("notifications from clone = %d, want 0", notified)
	}
}
