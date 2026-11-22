package xml

import "unique"

// intern returns the one shared copy of s. A loaded template keeps the
// strings it reads, and some of them repeat across thousands of templates
// (NPC type and clan names, AI parameter names and values, stat names): an
// interned one is held once, not once per template. The copy is independent
// of s, so it never keeps a decode buffer alive either.
func intern(s string) string {
	return unique.Make(s).Value()
}

// internAll interns every element of ss in place and returns ss.
func internAll(ss []string) []string {
	for i, s := range ss {
		ss[i] = intern(s)
	}
	return ss
}
