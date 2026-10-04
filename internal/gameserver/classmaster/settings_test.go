package classmaster

import (
	"reflect"
	"testing"
)

// TestParseJobs pins the ConfigClassMaster grammar: ';'-separated groups
// of a tier, its required items and its reward items, each list a
// bracketed set of id(count) split on '[', ']' and ','; empty fields are
// skipped as separators and a later tier replaces an earlier one.
func TestParseJobs(t *testing.T) {
	none := []Item{}
	for _, tc := range []struct {
		name string
		line string
		want map[int]Job
	}{
		{name: "absent", line: "", want: map[int]Job{}},
		{name: "shipped", line: "1;[];[];2;[];[];3;[];[]", want: map[int]Job{
			1: {Required: none, Reward: none}, 2: {Required: none, Reward: none}, 3: {Required: none, Reward: none},
		}},
		{name: "documented prices", line: " 1;[57(100000)];[];2;[57(1000000)];[];3;[57(10000000)],[5575(1000000)];[6622(1)] ", want: map[int]Job{
			1: {Required: []Item{{57, 100000}}, Reward: none},
			2: {Required: []Item{{57, 1000000}}, Reward: none},
			3: {Required: []Item{{57, 10000000}, {5575, 1000000}}, Reward: []Item{{6622, 1}}},
		}},
		{name: "comma list in one bracket", line: "2;[57(1),1864(20)];[1(2)]", want: map[int]Job{
			2: {Required: []Item{{57, 1}, {1864, 20}}, Reward: []Item{{1, 2}}},
		}},
		{name: "tier without lists", line: "1;[];[];2", want: map[int]Job{
			1: {Required: none, Reward: none}, 2: {Required: none, Reward: none},
		}},
		{name: "empty fields skipped", line: "1;;[57(5)];;[];", want: map[int]Job{
			1: {Required: []Item{{57, 5}}, Reward: none},
		}},
		{name: "later tier replaces", line: "1;[57(5)];[];1;[];[]", want: map[int]Job{
			1: {Required: none, Reward: none},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseJobs(tc.line)
			if err != nil {
				t.Fatalf("ParseJobs(%q) error = %v", tc.line, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ParseJobs(%q) = %+v, want %+v", tc.line, got, tc.want)
			}
		})
	}
}

// TestParseJobsRejects pins the lines that fail boot: a tier or an item
// field that does not parse, and an item without its count.
func TestParseJobsRejects(t *testing.T) {
	for _, line := range []string{
		"x;[];[]",
		"1;[57];[]",
		"1;[57(x)];[]",
		"1;2;3",
		"1; [57(1)];[]",
		"99999999999;[];[]",
	} {
		if got, err := ParseJobs(line); err == nil {
			t.Errorf("ParseJobs(%q) = %+v, want an error", line, got)
		}
	}
}
