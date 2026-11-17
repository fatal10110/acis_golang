package net.sf.l2j.gameserver.data.xml;

import java.util.HashMap;
import java.util.Map;

import net.sf.l2j.gameserver.model.actor.template.NpcTemplate;

// Stand-in for the npc template table: the templates a probe registers, looked up by
// npc id (the mount a player rides is read from it).
public final class NpcData
{
	private static final NpcData INSTANCE = new NpcData();

	public final Map<Integer, NpcTemplate> templates = new HashMap<>();

	public static NpcData getInstance()
	{
		return INSTANCE;
	}

	public NpcTemplate getTemplate(int id)
	{
		return templates.get(id);
	}
}
