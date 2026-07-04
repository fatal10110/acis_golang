// Package idfactory allocates the unique object ids assigned to characters,
// items, clans, and other persisted objects, reusing ids released back to it
// and reconstructing already-used ids from the database at startup.
package idfactory
