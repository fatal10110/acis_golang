package admin

// Defines reports whether the command table lists command, an admin_ word
// spelled exactly as the table spells it.
func (d *Data) Defines(command string) bool {
	if d == nil {
		return false
	}
	cmd, ok := d.Command(command)
	return ok && cmd.Name == command
}

// HasAccess reports whether a character playing under access may run
// command. The command must be in the command table, its required level in
// the access-level table, and access either that level or one reached by
// following access's child levels down the table. A command the table does
// not list is refused.
func (d *Data) HasAccess(command string, access AccessLevel) bool {
	if d == nil {
		return false
	}
	cmd, ok := d.Command(command)
	if !ok {
		return false
	}
	required, ok := d.accessLevels[cmd.AccessLevel]
	if !ok {
		return false
	}
	return required.Level == access.Level || d.hasChildAccess(access, required.Level)
}

// hasChildAccess walks access's child levels (a positive childLevel naming a
// defined level) and reports whether one of them is level. The walk is
// bounded by the table size, so a cycle of child levels ends refused.
func (d *Data) hasChildAccess(access AccessLevel, level int) bool {
	for range d.accessLevels {
		if access.ChildLevel <= 0 {
			return false
		}
		child, ok := d.accessLevels[access.ChildLevel]
		if !ok {
			return false
		}
		if child.Level == level {
			return true
		}
		access = child
	}
	return false
}
