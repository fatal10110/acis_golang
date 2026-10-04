package xml

import (
	"testing"

	"github.com/rs/zerolog"
)

// TestItemRejectedAtLoadLikeReference checks that an item is skipped, and the
// items around it load, when the reference rejects it while loading
// (DocumentItem.parseDocument catches per item, DocumentItem.java:66-77):
//   - an <effect> naming no implementation (#3265): an item discards its
//     effects, but EffectTemplate's constructor resolves the name first
//     (EffectTemplate.java:68-84);
//   - a condition value DocumentBase.parse*Condition cannot decode (#3088).
func TestItemRejectedAtLoadLikeReference(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"unknown effect name":                 `<for><effect name="Bogus" val="0"/></for>`,
		"effect name differing in case":       `<for><effect name="buff" val="0"/></for>`,
		"malformed use condition integer":     `<cond msgId="113"><player level="oops"/></cond>`,
		"malformed for-block condition list":  `<for><cond><target npcId="1,x"/></cond><add stat="pAtk" val="1"/></for>`,
		"malformed func condition pair":       `<for><add stat="pAtk" val="1"><player active_skill_id_lvl="5"/></add></for>`,
		"malformed effect condition integer":  `<for><effect name="Buff" val="0"><cond><player hp="x"/></cond></effect></for>`,
		"unknown race in use condition":       `<cond><player race="human"/></cond>`,
		"unknown skill condition stat in for": `<for><add stat="pAtk" val="1"><skill stat="pAtk2"/></add></for>`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeItemFile(t, dir, "fixture.xml", `
				<item id="1" type="EtcItem" name="a"/>
				<item id="2" type="Weapon" name="b"><set name="bodypart" val="rhand"/>`+body+`</item>
				<item id="3" type="EtcItem" name="c"/>`)

			table, err := LoadItemTemplates(dir, zerolog.Nop())
			if err != nil {
				t.Fatalf("LoadItemTemplates: %v", err)
			}
			if _, ok := table.Get(2); ok {
				t.Fatal("item 2 loaded, want it skipped")
			}
			for _, id := range []int32{1, 3} {
				if _, ok := table.Get(id); !ok {
					t.Fatalf("item %d not loaded", id)
				}
			}
		})
	}
}
