package player

import (
	"fmt"
	"reflect"
	"testing"
)

func TestCharacterIncreaseChargesClampsToMax(t *testing.T) {
	c := &Character{ID: 1}

	if ok := c.IncreaseCharges(2, 5); !ok || c.Charges() != 2 {
		t.Fatalf("after +2: Charges() = %d, ok = %v, want 2, true", c.Charges(), ok)
	}
	if ok := c.IncreaseCharges(2, 5); !ok || c.Charges() != 4 {
		t.Fatalf("after +2: Charges() = %d, ok = %v, want 4, true", c.Charges(), ok)
	}
	if ok := c.IncreaseCharges(3, 5); !ok || c.Charges() != 5 {
		t.Fatalf("after +3 clamped: Charges() = %d, ok = %v, want 5, true", c.Charges(), ok)
	}
	if ok := c.IncreaseCharges(1, 5); ok || c.Charges() != 5 {
		t.Fatalf("at max: Charges() = %d, ok = %v, want 5, false", c.Charges(), ok)
	}
}

func TestCharacterIncreaseChargesNotifiesStatusOnlyAfterSuccessfulAdd(t *testing.T) {
	c := &Character{ID: 1}
	var updates int
	c.SetChargesUpdater(func() { updates++ })

	if !c.IncreaseCharges(5, 5) {
		t.Fatal("IncreaseCharges() = false, want true")
	}
	if updates != 1 {
		t.Fatalf("updates after clamped add = %d, want 1", updates)
	}
	if c.IncreaseCharges(1, 5) {
		t.Fatal("IncreaseCharges() = true at max, want false")
	}
	if updates != 1 {
		t.Fatalf("updates after at-max no-op = %d, want 1", updates)
	}
}

func TestCharacterIncreaseChargesNotifiesForceMessageBeforeStatus(t *testing.T) {
	c := &Character{ID: 1}
	var events []string
	c.SetChargeMessageSender(func(charges int, maxed bool) {
		events = append(events, fmt.Sprintf("message:%d:%t", charges, maxed))
	})
	c.SetChargesUpdater(func() { events = append(events, "status") })

	c.IncreaseCharges(2, 5)
	c.IncreaseCharges(3, 5)
	c.IncreaseCharges(1, 5)

	want := []string{"message:2:false", "status", "message:5:true", "status", "message:5:true"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("charge notifications = %v, want %v", events, want)
	}
}

func TestCharacterDecreaseChargesReportsInsufficientCharges(t *testing.T) {
	c := &Character{ID: 1}
	c.IncreaseCharges(2, 5)

	if ok := c.DecreaseCharges(3); ok || c.Charges() != 2 {
		t.Fatalf("DecreaseCharges(3) over available = ok %v, Charges() %d, want false, 2", ok, c.Charges())
	}
	if ok := c.DecreaseCharges(2); !ok || c.Charges() != 0 {
		t.Fatalf("DecreaseCharges(2) = ok %v, Charges() %d, want true, 0", ok, c.Charges())
	}
}

func TestCharacterDecreaseChargesNotifiesStatusOnlyAfterSuccessfulRemoval(t *testing.T) {
	c := &Character{ID: 1}
	c.IncreaseCharges(2, 5)
	var updates int
	c.SetChargesUpdater(func() { updates++ })

	if !c.DecreaseCharges(1) {
		t.Fatal("DecreaseCharges() = false, want true")
	}
	if updates != 1 {
		t.Fatalf("updates after successful removal = %d, want 1", updates)
	}
	if c.DecreaseCharges(2) {
		t.Fatal("DecreaseCharges() = true with insufficient charges, want false")
	}
	if updates != 1 {
		t.Fatalf("updates after failed removal = %d, want 1", updates)
	}
}

func TestCharacterClearChargesResetsToZero(t *testing.T) {
	c := &Character{ID: 1}
	c.IncreaseCharges(4, 5)

	c.ClearCharges()

	if got := c.Charges(); got != 0 {
		t.Fatalf("Charges() after ClearCharges = %d, want 0", got)
	}
}

func TestCharacterClearChargesNotifiesStatusOnlyWhenChargesChange(t *testing.T) {
	c := &Character{ID: 1}
	c.IncreaseCharges(4, 5)
	var updates int
	c.SetChargesUpdater(func() { updates++ })

	c.ClearCharges()
	if updates != 1 {
		t.Fatalf("updates after clearing charges = %d, want 1", updates)
	}
	c.ClearCharges()
	if updates != 1 {
		t.Fatalf("updates after clearing empty charges = %d, want 1", updates)
	}
}

func TestCharacterDieClearsCharges(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(1)
	c.IncreaseCharges(3, 5)

	c.Die(nil)

	if got := c.Charges(); got != 0 {
		t.Fatalf("Charges() after Die = %d, want 0", got)
	}
}
