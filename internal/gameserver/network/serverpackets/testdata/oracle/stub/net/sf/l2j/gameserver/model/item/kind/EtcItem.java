package net.sf.l2j.gameserver.model.item.kind;

import net.sf.l2j.gameserver.model.holder.IntIntHolder;

// Stand-in for an etc item template: the skills its use casts (null for none).
public final class EtcItem
{
	public IntIntHolder[] skills;

	public IntIntHolder[] getSkills()
	{
		return skills;
	}
}
