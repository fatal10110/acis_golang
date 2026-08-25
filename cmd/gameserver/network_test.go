package main

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

type rosterPlayerStub struct {
	id      int32
	account string
}

func (s rosterPlayerStub) ObjectID() int32      { return s.id }
func (s rosterPlayerStub) AccountName() string  { return s.account }

func TestOnlineAccountsCollectsWorldRoster(t *testing.T) {
	state := world.New()
	state.AddPlayer(rosterPlayerStub{account: "acc1"})
	state.AddPlayer(rosterPlayerStub{account: ""})

	got := onlineAccounts(state)
	if len(got) != 1 || got[0] != "acc1" {
		t.Fatalf("onlineAccounts = %v, want [acc1]", got)
	}
}

func TestOnlineAccountsEmptyWorldSendsNothing(t *testing.T) {
	if got := onlineAccounts(world.New()); got != nil {
		t.Fatalf("onlineAccounts = %v, want nil", got)
	}
}

func TestPlayerAuthResponseHandlerToleratesMissingLinkAndRejections(t *testing.T) {
	validator := network.NewSessionValidator()
	handler := playerAuthResponseHandler(provideLoginLinkState(), validator, zerolog.Nop())

	handler("acc1", true)  // no login link up yet: must not panic
	handler("acc2", false) // rejection: nothing to report
}
