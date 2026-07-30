package item

// FlushBatch groups the resolved persistence effects of one item
// persistence flush, spanning item, augmentation, and pet rows. A
// FlushBatch is meant to be applied atomically: either every group in it
// lands, or none of it does.
type FlushBatch struct {
	Saves               []*Instance
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
