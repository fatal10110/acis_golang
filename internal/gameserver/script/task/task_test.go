package task

import (
	"context"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestTasksMatchReferenceFingerprints compares the package with the
// reference's task classes: literals, engine calls and hook shapes.
//
// Exempt: the "task" description every task passes its base class, which a
// Go script has no field for; and the end hooks, which no task reacts to
// and the runner does not build.
func TestTasksMatchReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{"task.CastleTaxRefresh", "task.ClanLeaderTransfer", "task.SevenSignsUpdate"},
		`string "task"`, "hook onEnd none")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// server records the Server calls a task makes, in order.
type server struct {
	sealValidation bool
	calls          []string
}

func (s *server) UpdateCastleTaxes()         { s.calls = append(s.calls, "UpdateCastleTaxes") }
func (s *server) SealValidationPeriod() bool { return s.sealValidation }
func (s *server) SaveFestivalScores(context.Context) {
	s.calls = append(s.calls, "SaveFestivalScores")
}
func (s *server) SaveSevenSigns(context.Context) { s.calls = append(s.calls, "SaveSevenSigns") }
func (s *server) TransferClanLeaders()           { s.calls = append(s.calls, "TransferClanLeaders") }

// TestTaskStarts pins what each task's start does, in order.
func TestTaskStarts(t *testing.T) {
	for _, tc := range []struct {
		name           string
		ctor           func() script.Script
		sealValidation bool
		want           []string
	}{
		{"CastleTaxRefresh", CastleTaxRefresh, false, []string{"UpdateCastleTaxes"}},
		{"ClanLeaderTransfer", ClanLeaderTransfer, false, []string{"TransferClanLeaders"}},
		{"SevenSignsUpdate", SevenSignsUpdate, false, []string{"SaveFestivalScores", "SaveSevenSigns"}},
		{"SevenSignsUpdate in seal validation", SevenSignsUpdate, true, []string{"SaveSevenSigns"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := &server{sealValidation: tc.sealValidation}
			s := tc.ctor()
			s.Hooks.Start(&s, script.Start{Ctx: context.Background(), Server: srv})
			if !slices.Equal(srv.calls, tc.want) {
				t.Fatalf("start calls %v, want %v", srv.calls, tc.want)
			}
		})
	}
}
