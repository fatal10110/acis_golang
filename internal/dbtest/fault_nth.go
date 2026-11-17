package dbtest

import (
	"strings"
	"sync"
)

// FailNth makes the n-th statement executed or query run from now on f's
// handles whose SQL contains match fail with ErrFault, once; every other
// statement, before and after it, runs. A suite uses it to fail one write in
// the middle of a chain, such as the second row of a multi-row insert, or to
// prove a read is not repeated. A later call replaces a pending one.
func (f *Fault) FailNth(match string, n int) {
	f.nth.mu.Lock()
	defer f.nth.mu.Unlock()
	f.nth.match, f.nth.left = match, n
}

// nthFault is the one pending FailNth of a Fault.
type nthFault struct {
	mu    sync.Mutex
	match string
	left  int
}

// targets reports whether query is one a pending FailNth counts.
func (n *nthFault) targets(query string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.match != "" && strings.Contains(query, n.match)
}

// hit counts query against the pending FailNth and reports whether it is
// the statement to fail, which ends the FailNth.
func (n *nthFault) hit(query string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.match == "" || !strings.Contains(query, n.match) {
		return false
	}
	n.left--
	if n.left > 0 {
		return false
	}
	n.match = ""
	return true
}
