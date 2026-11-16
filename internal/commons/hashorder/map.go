package hashorder

import (
	"cmp"
	"slices"
	"unicode/utf16"
)

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
	return int(foldedHash(id)) & (mapTable(count) - 1)
}

// mapTable is the bucket count of a plain hash table holding count keys.
func mapTable(count int) int {
	n := setMinTable
	for count > threshold(n) {
		n *= 2
	}
	return n
}

// TextHash is the hash a text key is filed under in a hash table: each
// UTF-16 code unit of s in turn added to 31 times the hash so far, wrapping
// at 32 bits. MapIndex(TextHash(s), count) is the bucket of text key s.
func TextHash(s string) int32 {
	var h int32
	for _, u := range utf16.Encode([]rune(s)) {
		h = 31*h + int32(u)
	}
	return h
}

// SortMap sorts keys, distinct and in the order they were put in, into the
// order a plain hash table holding just them lists them, as MapIndex
// describes; hash is a key's hash. A key put in again keeps its place, so
// keys lists each key where it was first put.
func SortMap[K any](keys []K, hash func(K) int32) {
	mask := mapTable(len(keys)) - 1
	slices.SortStableFunc(keys, func(a, b K) int {
		return cmp.Compare(int(foldedHash(hash(a)))&mask, int(foldedHash(hash(b)))&mask)
	})
}
