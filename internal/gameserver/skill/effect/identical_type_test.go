package effect

import "testing"

// TestEffectIdenticalComparesTheCoarseEffectType: two effects of one skill
// with the same stack type and order are the same effect when their kinds
// share an effect type, even when the kinds differ (Buff, BigHead,
// ImobileBuff, ImobilePetBuff, Grow and Recovery are all buffs; Relax and
// ChameleonRest both relax), and different ones when the types differ.
func TestEffectIdenticalComparesTheCoarseEffectType(t *testing.T) {
	for _, tc := range []struct {
		a, b Type
		want bool
	}{
		{TypeBuff, TypeGrow, true},
		{TypeGrow, TypeRecovery, true},
		{TypeBigHead, TypeImmobilizeEffector, true},
		{TypeImmobilizePetBuff, TypeBuff, true},
		{TypeRecovery, TypeBigHead, true},
		{TypeRelax, TypeChameleonRest, true},
		{TypeBuff, TypeBuff, true},
		{TypeBuff, TypeDebuff, false},
		{TypeGrow, TypeRelax, false},
		{TypeBuff, TypeBlockBuff, false},
	} {
		a := newEffect("a", 1204, "speed", 1, false)
		a.Type = tc.a
		b := newEffect("b", 1204, "speed", 1, false)
		b.Type = tc.b
		if got := a.identical(b); got != tc.want {
			t.Errorf("%s identical to %s = %v, want %v", tc.a, tc.b, got, tc.want)
		}
		if got := b.identical(a); got != tc.want {
			t.Errorf("%s identical to %s = %v, want %v", tc.b, tc.a, got, tc.want)
		}
	}
}

// TestListReplacesAnIdenticalBuffOfAnotherKind: a recast whose effect is
// another buff kind of the same skill, stack type and order replaces the
// one already running.
func TestListReplacesAnIdenticalBuffOfAnotherKind(t *testing.T) {
	var events []string
	list := newTestList(eventOwner{events: &events})
	old := namedEffect("old", 1204, "speed", 1, false, &events)
	old.Type = TypeGrow
	fresh := namedEffect("fresh", 1204, "speed", 1, false, &events)

	list.Add(old)
	list.Add(fresh)

	requireNames(t, list.All(), []string{"fresh"})
}
