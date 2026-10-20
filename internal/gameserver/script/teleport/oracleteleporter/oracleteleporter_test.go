package oracleteleporter

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptpages"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// class: literals, engine calls and hook shapes. The player memo's write is
// the Player handle's SetMemo; the census row of MemoSet.set is shared
// with the quest state's variables. The 10 and 32 are the base and bit
// size of the memo's parse, which rejects a value past int32 as the
// reference's parseInt does. The 401 is the quest-limit system message,
// a named constant in the reference. A memo naming an NPC outside the
// table aborts the reference's return on a null location, the port's on a
// named panic.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{"script.teleport.OracleTeleporter"},
		"call MemoSet.set", "number 10", "number 32", "number 401",
		`string "oracleteleporter: no return point for NPC "`)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestPagesAreInTheDatapack requires every page the teleporter names to be a
// page of its datapack directory.
func TestPagesAreInTheDatapack(t *testing.T) {
	idx, err := scriptpages.Load()
	if err != nil {
		t.Fatal(err)
	}
	problems, err := scriptpages.CheckPackage(idx, ".", "teleport/OracleTeleporter")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
