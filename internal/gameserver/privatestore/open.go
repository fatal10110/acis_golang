package privatestore

// OpenState is what deciding whether a player may set up a store reads.
type OpenState struct {
	Operate   OperateType
	AlikeDead bool
	// Fighting covers a duel, an attack stance, a PvP flag and a swing in
	// flight.
	Fighting bool
	Casting  bool
	// NoStoreZone covers a no-store zone and the Olympiad.
	NoStoreZone bool
	// Busy covers riding a mount, a pending transaction request and being
	// out of control.
	Busy        bool
	Seated      bool
	StandingNow bool
}

// OpenRefusal is why a player may not set up a store.
type OpenRefusal uint8

const (
	// OpenAllowed lets the store be set up.
	OpenAllowed OpenRefusal = iota
	// OpenRefusedDead refuses a dead store owner and leaves its store as
	// it is.
	OpenRefusedDead
	// OpenRefusedSilently closes the store with no message.
	OpenRefusedSilently
	// OpenRefusedFighting closes the store: no store during combat.
	OpenRefusedFighting
	// OpenRefusedCasting closes the store: no store while casting.
	OpenRefusedCasting
	// OpenRefusedZone closes the store: no store here.
	OpenRefusedZone
)

// CanOpen decides whether a player in st may set up a store. A store
// already open for business only checks that its owner is alive, so its
// owner can always take it down again. Every other refusal closes the
// store; the caller sets the operate state to none and sends the
// refusal's message.
func CanOpen(st OpenState) OpenRefusal {
	if st.Operate.InStoreMode() {
		if st.AlikeDead {
			return OpenRefusedDead
		}
		return OpenAllowed
	}
	switch {
	case st.Fighting:
		return OpenRefusedFighting
	case st.Casting:
		return OpenRefusedCasting
	case st.NoStoreZone:
		return OpenRefusedZone
	case st.AlikeDead || st.Busy:
		return OpenRefusedSilently
	case st.Seated || st.StandingNow:
		// A seated player cannot open one; one standing up from the store
		// it just took down must wait for the stand to end.
		return OpenRefusedSilently
	}
	return OpenAllowed
}
