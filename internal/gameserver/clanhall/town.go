package clanhall

import "strings"

// InTown returns the clan halls whose town is town, matched
// case-insensitively, by id.
func (hs *Halls) InTown(town string) []HallView {
	if hs == nil {
		return nil
	}
	hs.mu.Lock()
	defer hs.mu.Unlock()
	var out []HallView
	for _, h := range hs.order {
		if strings.EqualFold(h.data.Town, town) {
			out = append(out, h.viewLocked())
		}
	}
	return out
}
