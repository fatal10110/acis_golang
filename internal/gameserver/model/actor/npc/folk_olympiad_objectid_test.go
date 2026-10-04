package npc

import (
	"strconv"
	"testing"
)

// objectIDPages serves every page as a body that carries %objectId%.
type objectIDPages struct{}

func (objectIDPages) Get(path string) (string, bool) {
	return "<html><body>" + path + " npc_%objectId%_Chat 0</body></html>", true
}

// TestOlympiadNobleGatePagesObjectID pins which OlympiadNoble refusal pages
// fill %objectId%: OlympiadManagerNpc.onBypassFeedback sends noble_cant_cw.htm
// without a replace, so the placeholder reaches the client as is, while the
// subclass and third occupation pages are filled with the NPC's object id.
func TestOlympiadNobleGatePagesObjectID(t *testing.T) {
	t.Parallel()
	manager := digitsFolk(t, "OlympiadManagerNpc", 31688)
	id := strconv.Itoa(int(manager.ObjectID()))
	for _, tc := range []struct {
		name   string
		talker Talker
		page   string
		filled bool
	}{
		{"cursed weapon", Talker{CursedWeapon: true, SubclassActive: true}, "noble_cant_cw.htm", false},
		{"subclass", Talker{SubclassActive: true, Noble: true, ThirdClass: true}, "noble_cant_sub.htm", true},
		{"not noble", Talker{ThirdClass: true}, "noble_cant_thirdclass.htm", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := manager.Bypass(objectIDPages{}, ChatRules{}, tc.talker, "OlympiadNoble 4")
			objectID := "%objectId%"
			if tc.filled {
				objectID = id
			}
			want := "<html><body>" + olympiadPages + tc.page + " npc_" + objectID + "_Chat 0</body></html>"
			if got.Outcome != BypassPage || got.HTML != want {
				t.Fatalf("Bypass = %v %q, want %v %q", got.Outcome, got.HTML, BypassPage, want)
			}
		})
	}
}
