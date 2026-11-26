package attackable

// ReduceAllHate subtracts amount from every entry's hate, uncapped in
// either direction. Entries are kept even once negative.
func (h *HateTable) ReduceAllHate(amount float64) { h.t.reduceAll(amount) }
