package clan

import "testing"

// TestPledgeClass checks the clan rank of every clan level, sub-unit and
// sub-unit leadership against the stored rank table.
func TestPledgeClass(t *testing.T) {
	type row struct {
		level      int
		leader     bool
		pledgeType int
		leads      int
		want       int
	}
	rows := []row{
		{0, true, SubunitMain, 0, 1},
		{0, false, SubunitMain, 0, 1},
		{3, false, SubunitAcademy, 0, 1},
		{4, true, SubunitMain, 0, 3},
		{4, false, SubunitMain, 0, 0},
		{5, true, SubunitMain, 0, 4},
		{5, false, SubunitMain, 0, 2},
		{5, false, SubunitAcademy, 0, 2},
	}
	// Per level 6-8: academy, royal guard, knight, leader, royal captain,
	// knight captain, plain main-clan member.
	for level, want := range map[int][7]int{
		6: {1, 2, 0, 5, 4, 3, 3},
		7: {1, 3, 2, 7, 6, 5, 4},
		8: {1, 4, 3, 8, 7, 6, 5},
	} {
		rows = append(rows,
			row{level, false, SubunitAcademy, 0, want[0]},
			row{level, false, SubunitRoyal1, 0, want[1]},
			row{level, false, SubunitRoyal2, 0, want[1]},
			row{level, false, SubunitKnight1, 0, want[2]},
			row{level, false, SubunitKnight4, 0, want[2]},
			row{level, true, SubunitMain, 0, want[3]},
			row{level, false, SubunitMain, SubunitRoyal2, want[4]},
			row{level, false, SubunitMain, SubunitKnight3, want[5]},
			row{level, false, SubunitMain, 0, want[6]},
			row{level, false, SubunitMain, SubunitAcademy, want[6]},
			row{level, false, 300, 0, 0},
		)
	}
	for _, r := range rows {
		if got := PledgeClass(r.level, r.leader, r.pledgeType, r.leads); got != r.want {
			t.Errorf("PledgeClass(level %d, leader %v, type %d, leads %d) = %d, want %d", r.level, r.leader, r.pledgeType, r.leads, got, r.want)
		}
	}
}
