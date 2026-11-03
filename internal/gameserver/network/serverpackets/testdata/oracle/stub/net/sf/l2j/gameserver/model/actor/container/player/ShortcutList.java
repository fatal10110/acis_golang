package net.sf.l2j.gameserver.model.actor.container.player;

import net.sf.l2j.gameserver.model.Shortcut;

// Stand-in for the shortcut bar: the shortcuts in the order the packet lists them.
public final class ShortcutList
{
	public Shortcut[] shortcuts = new Shortcut[0];

	public Shortcut[] getShortcuts()
	{
		return shortcuts;
	}
}
