package script

import "testing"

// TestIsPageFile pins which hook answers can name a page file of the
// script: plain names and subdirectories of the script's own directory,
// never a climbing segment or a character outside a file name.
func TestIsPageFile(t *testing.T) {
	for _, name := range []string{"30473-09.htm", "35598/watering_manager001.htm", "darkelf/guide_delf_frankia_f10_022.htm", "a.html"} {
		if !isPageFile(name) {
			t.Errorf("isPageFile(%q) = false, want true", name)
		}
	}
	for _, name := range []string{
		"../Q001/secret.htm", "a/../../b.htm", "./a.htm", "/a.htm", "a//b.htm", `..\a.htm`,
		"a b.htm", `x.htm"><a action="bypass -h npc_1_Chat 1">`, "<b>.htm", "é.htm",
	} {
		if isPageFile(name) {
			t.Errorf("isPageFile(%q) = true, want false", name)
		}
	}
}
