package skill

import (
	"reflect"
	"testing"
)

func TestParseExtractableItems(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []ExtractableProduct
	}{
		{"empty", "", nil},
		{
			"single pair",
			"57,10,20.5",
			[]ExtractableProduct{{Items: []ExtractableItem{{ItemID: 57, Quantity: 10}}, Chance: 20.5}},
		},
		{
			"two groups, second with two pairs",
			"57,10,20.5;1234,1,5678,2,79.5",
			[]ExtractableProduct{
				{Items: []ExtractableItem{{ItemID: 57, Quantity: 10}}, Chance: 20.5},
				{Items: []ExtractableItem{{ItemID: 1234, Quantity: 1}, {ItemID: 5678, Quantity: 2}}, Chance: 79.5},
			},
		},
		{
			"malformed group is skipped, well-formed kept",
			"not-a-number,10,20.5;57,10,20.5",
			[]ExtractableProduct{{Items: []ExtractableItem{{ItemID: 57, Quantity: 10}}, Chance: 20.5}},
		},
		{
			"even field count is skipped",
			"57,10;57,10,20.5",
			[]ExtractableProduct{{Items: []ExtractableItem{{ItemID: 57, Quantity: 10}}, Chance: 20.5}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseExtractableItems(tt.raw)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseExtractableItems(%q) = %#v, want %#v", tt.raw, got, tt.want)
			}
		})
	}
}
