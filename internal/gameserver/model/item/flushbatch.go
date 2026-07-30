package item

// FlushBatch groups the resolved persistence effects of one item
// persistence flush, spanning item, augmentation, and pet rows. A
// FlushBatch is meant to be applied atomically: either every group in it
// lands, or none of it does.
//
// Saves holds point-in-time state, not live instances, so a store may
// read its fields directly without taking Instance's snapshot lock.
//
// An object id must not appear in both a table's save group and its
// delete group (Saves and Deletes; AugmentationSaves and
// AugmentationDeletes) — a store is free to apply saves before deletes
// for a table, so a duplicate would have its save silently dropped.
type FlushBatch struct {
	Saves               []InstanceState
	Deletes             []int32
	AugmentationSaves   []FlushAugmentationSave
	AugmentationDeletes []int32
	PetDeletes          []int32
}

// FlushAugmentationSave is one item's augmentation to upsert.
type FlushAugmentationSave struct {
	ObjectID     int32
	Augmentation Augmentation
}
