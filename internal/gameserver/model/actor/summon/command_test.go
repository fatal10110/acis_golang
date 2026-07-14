package summon

import "testing"

func TestResolveToggleFollow(t *testing.T) {
	tests := []struct {
		name string
		req  Request
		want Outcome
	}{
		{"no summon", Request{Command: CommandToggleFollow}, OutcomeIgnored},
		{"following too far to recall", Request{Command: CommandToggleFollow, HasSummon: true, FollowActive: true, OwnerWithinFollowRange: false}, OutcomeIgnored},
		{"out of control", Request{Command: CommandToggleFollow, HasSummon: true, OutOfControl: true}, OutcomeRefusedOutOfControl},
		{"applies", Request{Command: CommandToggleFollow, HasSummon: true, FollowActive: true, OwnerWithinFollowRange: true}, OutcomeApplied},
		{"applies while not following", Request{Command: CommandToggleFollow, HasSummon: true, FollowActive: false}, OutcomeApplied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Resolve(tt.req); got != tt.want {
				t.Errorf("Resolve() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveAttack(t *testing.T) {
	base := Request{Command: CommandAttack, HasSummon: true, HasTarget: true}

	tests := []struct {
		name string
		req  Request
		want Outcome
	}{
		{"no target", Request{Command: CommandAttack, HasSummon: true}, OutcomeIgnored},
		{"no summon", Request{Command: CommandAttack, HasTarget: true}, OutcomeIgnored},
		{"target is own summon", withReq(base, func(r *Request) { r.TargetIsSummon = true }), OutcomeIgnored},
		{"target is owner", withReq(base, func(r *Request) { r.TargetIsOwner = true }), OutcomeIgnored},
		{"target already dead", withReq(base, func(r *Request) { r.TargetIsDeadCreature = true }), OutcomeIgnored},
		{"passive summon can't attack", withReq(base, func(r *Request) { r.IsPassiveSummon = true }), OutcomeIgnored},
		{"out of control", withReq(base, func(r *Request) { r.OutOfControl = true }), OutcomeRefusedOutOfControl},
		{"pet outgrew owner", withReq(base, func(r *Request) { r.IsPet = true; r.SummonLevel = 50; r.OwnerLevel = 20 }), OutcomeRefusedLevelGap},
		{"pet within level gap applies", withReq(base, func(r *Request) { r.IsPet = true; r.SummonLevel = 40; r.OwnerLevel = 20 }), OutcomeApplied},
		{"servitor ignores level gap rule", withReq(base, func(r *Request) { r.IsPet = false; r.SummonLevel = 90; r.OwnerLevel = 1 }), OutcomeApplied},
		{"applies", base, OutcomeApplied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Resolve(tt.req); got != tt.want {
				t.Errorf("Resolve() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveStop(t *testing.T) {
	if got := Resolve(Request{Command: CommandStop}); got != OutcomeIgnored {
		t.Errorf("no summon: Resolve() = %v, want OutcomeIgnored", got)
	}
	if got := Resolve(Request{Command: CommandStop, HasSummon: true, OutOfControl: true}); got != OutcomeRefusedOutOfControl {
		t.Errorf("out of control: Resolve() = %v, want OutcomeRefusedOutOfControl", got)
	}
	if got := Resolve(Request{Command: CommandStop, HasSummon: true}); got != OutcomeApplied {
		t.Errorf("Resolve() = %v, want OutcomeApplied", got)
	}
}

func TestResolveReturnPet(t *testing.T) {
	base := Request{Command: CommandReturnPet, IsPet: true}

	tests := []struct {
		name string
		req  Request
		want Outcome
	}{
		{"not a pet", Request{Command: CommandReturnPet, IsPet: false}, OutcomeIgnored},
		{"dead", withReq(base, func(r *Request) { r.SummonIsDead = true }), OutcomeRefusedDead},
		{"out of control", withReq(base, func(r *Request) { r.OutOfControl = true }), OutcomeRefusedOutOfControl},
		{"attacking", withReq(base, func(r *Request) { r.IsAttackingNow = true }), OutcomeRefusedInCombat},
		{"in combat", withReq(base, func(r *Request) { r.InCombat = true }), OutcomeRefusedInCombat},
		{"too hungry", withReq(base, func(r *Request) { r.BelowUnsummonFeedShare = true }), OutcomeRefusedHungry},
		{"applies", base, OutcomeApplied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Resolve(tt.req); got != tt.want {
				t.Errorf("Resolve() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveUnsummonServitor(t *testing.T) {
	base := Request{Command: CommandUnsummonServitor, HasSummon: true}

	tests := []struct {
		name string
		req  Request
		want Outcome
	}{
		{"targets a pet, not a servitor", withReq(base, func(r *Request) { r.IsPet = true }), OutcomeIgnored},
		{"no summon", Request{Command: CommandUnsummonServitor}, OutcomeIgnored},
		{"dead", withReq(base, func(r *Request) { r.SummonIsDead = true }), OutcomeRefusedDead},
		{"out of control", withReq(base, func(r *Request) { r.OutOfControl = true }), OutcomeRefusedOutOfControl},
		{"in combat", withReq(base, func(r *Request) { r.InCombat = true }), OutcomeRefusedInCombat},
		{"applies", base, OutcomeApplied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Resolve(tt.req); got != tt.want {
				t.Errorf("Resolve() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveMoveToTarget(t *testing.T) {
	base := Request{Command: CommandMoveToTarget, HasSummon: true, HasTarget: true}

	tests := []struct {
		name string
		req  Request
		want Outcome
	}{
		{"no target", Request{Command: CommandMoveToTarget, HasSummon: true}, OutcomeIgnored},
		{"target is own summon", withReq(base, func(r *Request) { r.TargetIsSummon = true }), OutcomeIgnored},
		{"out of control", withReq(base, func(r *Request) { r.OutOfControl = true }), OutcomeRefusedOutOfControl},
		{"applies", base, OutcomeApplied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Resolve(tt.req); got != tt.want {
				t.Errorf("Resolve() = %v, want %v", got, tt.want)
			}
		})
	}
}

// withReq returns a copy of base mutated by fn, so table-driven cases can
// vary one field at a time from a shared baseline without aliasing it.
func withReq(base Request, fn func(*Request)) Request {
	r := base
	fn(&r)
	return r
}
