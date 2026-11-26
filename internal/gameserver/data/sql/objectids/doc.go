// Package objectids owns the game schema the object id allocator depends on:
// the boot-time repair of orphaned rows and the queries that read back every
// stored object id. A table that starts persisting allocator ids adds its
// query here.
package objectids
