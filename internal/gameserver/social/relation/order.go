package relation

import "slices"

// The hash table a friend or block list is gathered into before it is sent:
// 16 buckets to start, doubled once it holds more than three quarters of
// its size, and doubled too, while under 64 buckets, when a ninth id lands
// in one bucket.
const (
	listInitialBuckets = 16
	listTreeifyBuckets = 64
	listBucketCrowd    = 8
)

// clientOrder puts ids in the order the client is sent a friend or block
// list: grouped by bucket of the hash table the list is gathered into,
// lowest bucket first. An id's bucket is its value folded onto its own high
// half (id ^ id>>>16), masked to the table size.
//
// Within one bucket the ids keep the order they were gathered in, which is
// the iteration order of the relation store; that order is not modelled
// here and the ids are taken in ascending order instead (#3156). Nor is the
// order of a bucket holding nine or more ids once the table has 64 buckets.
func clientOrder(ids []int32) []int32 {
	if len(ids) < 2 {
		return ids
	}
	slices.Sort(ids)
	buckets := listBuckets(ids)
	slices.SortStableFunc(ids, func(a, b int32) int {
		return listBucket(a, buckets) - listBucket(b, buckets)
	})
	return ids
}

// listBuckets is the table size once every id in ids (in insertion order)
// has been added.
func listBuckets(ids []int32) int {
	buckets := listInitialBuckets
	for size := 1; size <= len(ids); size++ {
		if buckets < listTreeifyBuckets && crowded(ids[:size], buckets) {
			buckets *= 2
		}
		if size > buckets*3/4 {
			buckets *= 2
		}
	}
	return buckets
}

// crowded reports whether the last id of added landed in a bucket that
// already held listBucketCrowd ids.
func crowded(added []int32, buckets int) bool {
	last := listBucket(added[len(added)-1], buckets)
	n := 0
	for _, id := range added {
		if listBucket(id, buckets) == last {
			n++
		}
	}
	return n > listBucketCrowd
}

func listBucket(id int32, buckets int) int {
	h := uint32(id)
	return int((h ^ h>>16) & uint32(buckets-1))
}
