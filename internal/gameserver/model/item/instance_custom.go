package item

// SetCustomType1 changes the first custom-type value and schedules
// persistence. It reports whether anything changed.
func (inst *Instance) SetCustomType1(value int) bool {
	mu := inst.lock()
	mu.Lock()
	if inst.CustomType1 == value {
		mu.Unlock()
		return false
	}
	inst.CustomType1 = value
	mu.Unlock()

	inst.persisted()
	return true
}
