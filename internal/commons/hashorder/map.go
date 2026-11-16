package hashorder

// MapIndex returns the bucket id falls in, in a plain (single-threaded)
// hash table holding count ids: the table starts at 16 buckets and doubles
// once it holds more than three quarters of its size, and the bucket is
// the low bits of the id folded with its high half. Such a table lists its
// ids by bucket and, within a bucket, in the order they were put in, as a
// doubling keeps that order; a stable sort by MapIndex of ids taken in
// insertion order gives its listing order.
//
// A bucket crowded past eight ids is not modelled (the table then doubles
// early, or from 64 buckets turns the bucket into a tree): at the load the
// table keeps, that takes ids colliding on purpose.
func MapIndex(id int32, count int) int {
	n := setMinTable
	for count > threshold(n) {
		n *= 2
	}
	return int(foldedHash(id)) & (n - 1)
}
