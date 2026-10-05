package q900

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// Test files are not read: these literals and calls must not count.
var _ = []any{"not in the reference", 4242, script.TakeItems}
