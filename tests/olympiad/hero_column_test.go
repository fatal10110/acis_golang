package olympiad

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestHeroColumnMakesNoHero pins where hero status comes from at login: the
// running era's heroes rows alone (Player.restore asks the hero registry). A
// stored characters.hero flag without an active heroes row shows no hero
// byte and gives no hero skill; with the row, the hero is restored as usual.
func TestHeroColumnMakesNoHero(t *testing.T) {
	t.Parallel()
	const column = `UPDATE characters SET hero = 1 WHERE char_name = 'Login'`
	for _, tc := range []struct {
		name  string
		seeds []string
		hero  bool
	}{
		{"column only", []string{column}, false},
		{"column and active heroes row", []string{
			column,
			`INSERT INTO heroes (char_id, class_id, count, played, active) SELECT obj_Id, 88, 1, 1, 1 FROM characters WHERE char_name = 'Login'`,
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, burst := bootNamed(t, "Login", tc.seeds, skillOptions(t)...)
			want := byte(0)
			if tc.hero {
				want = 1
			}
			if got := userInfoHero(firstFrame(t, burst, serverpackets.OpcodeUserInfo)); got != want {
				t.Errorf("UserInfo hero byte = %d, want %d", got, want)
			}
			skills := skillListLevels(t, firstFrame(t, burst, serverpackets.OpcodeSkillList))
			for _, id := range heroSkills {
				if _, ok := skills[id]; ok != tc.hero {
					t.Errorf("SkillList hero skill %d known = %v, want %v", id, ok, tc.hero)
				}
			}
		})
	}
}
