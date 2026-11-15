package petitem

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

func TestAmmunitionIsOnlyArrowsAndShots(t *testing.T) {
	for _, tc := range []struct {
		name string
		tmpl *item.Template
		want bool
	}{
		{"arrow", &item.Template{EtcItem: &item.EtcItemDetail{Type: item.EtcItemArrow}}, true},
		{"shot", &item.Template{EtcItem: &item.EtcItemDetail{Type: item.EtcItemShot}}, true},
		{"herb", &item.Template{EtcItem: &item.EtcItemDetail{Type: item.EtcItemHerb}}, false},
		{"plain etc item", &item.Template{EtcItem: &item.EtcItemDetail{Type: item.EtcItemNone}}, false},
		{"weapon", &item.Template{Weapon: &item.WeaponDetail{}}, false},
		{"nil", nil, false},
	} {
		if got := Ammunition(tc.tmpl); got != tc.want {
			t.Errorf("Ammunition(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
}
