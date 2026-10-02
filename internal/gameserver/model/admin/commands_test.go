package admin

import "testing"

// TestCommandsKeepsTableOrder pins AdminData.getAdminCommands: the commands
// come back in the order the table lists them, which //help pages through,
// not in lookup order.
func TestCommandsKeepsTableOrder(t *testing.T) {
	names := []string{"admin_zeta", "admin_alpha", "admin_mid"}
	commands := make([]Command, len(names))
	for i, name := range names {
		commands[i] = Command{Name: name, AccessLevel: 7}
	}
	data, err := NewData(nil, commands)
	if err != nil {
		t.Fatalf("NewData() error: %v", err)
	}
	got := data.Commands()
	if len(got) != len(names) {
		t.Fatalf("Commands() = %v, want %d commands", got, len(names))
	}
	for i, name := range names {
		if got[i].Name != name {
			t.Fatalf("Commands()[%d] = %q, want %q", i, got[i].Name, name)
		}
	}
	commands[0].Name = "admin_changed"
	if data.Commands()[0].Name != "admin_zeta" {
		t.Fatal("Commands() follows the caller's slice, want its own copy")
	}
	if (*Data)(nil).Commands() != nil {
		t.Fatal("nil table Commands() != nil")
	}
}
