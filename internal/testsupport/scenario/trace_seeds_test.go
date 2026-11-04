package scenario

import (
	"reflect"
	"testing"
)

const tracedSeedSample = `trace x.golden
character Leader 40
member Mate 40 standing 2 hold 7212 1
npc footman 21324 Monster 40 0 0
script script.feature.KetraOrcSupport

bypass Quest X x3
appear
kill footman
  standing Mate 0
  standing player 0
`

func TestParseTakesAMemberAnAppearStepAndABypassCountInATracedScenario(t *testing.T) {
	sc, err := Parse("traced.scenario", []byte(tracedSeedSample))
	if err != nil {
		t.Fatal(err)
	}
	if sc.Steps[0].Repeat != 3 || sc.Steps[1].Verb != "appear" {
		t.Fatalf("steps = %+v", sc.Steps)
	}
	// The probe reads the standing of the character and of a member, as the trace writes it.
	want := []string{"standing Mate 0", "standing player 0"}
	if got := sc.seedChecks(sc.Steps[2]); !reflect.DeepEqual(got, want) {
		t.Fatalf("seedChecks = %q, want %q", got, want)
	}
}

func TestTraceClientsWritesAnAppearStepAsTheClientsAnswer(t *testing.T) {
	sc, err := Parse("traced.scenario", []byte(tracedSeedSample))
	if err != nil {
		t.Fatal(err)
	}
	got, err := traceClients(sc.Steps[1], nil, sc)
	if err != nil || !reflect.DeepEqual(got, []string{"C Appearing"}) {
		t.Fatalf("traceClients = %q, %v", got, err)
	}
	// An xN bypass is one send in the trace: the others are not recorded.
	got, err = traceClients(sc.Steps[0], nil, sc)
	if err != nil || !reflect.DeepEqual(got, []string{"C RequestBypassToServer Quest X"}) {
		t.Fatalf("traceClients = %q, %v", got, err)
	}
}
