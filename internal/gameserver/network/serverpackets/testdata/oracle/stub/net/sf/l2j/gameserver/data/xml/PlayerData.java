package net.sf.l2j.gameserver.data.xml;

import java.util.HashMap;
import java.util.Map;

import net.sf.l2j.gameserver.enums.actors.ClassId;
import net.sf.l2j.gameserver.model.actor.template.PlayerTemplate;

// Stand-in for the player template table: the templates a probe registers,
// looked up by class or class id.
public final class PlayerData
{
	private static final PlayerData INSTANCE = new PlayerData();

	public final Map<Integer, PlayerTemplate> templates = new HashMap<>();

	public static PlayerData getInstance()
	{
		return INSTANCE;
	}

	public PlayerTemplate getTemplate(ClassId classId)
	{
		return templates.get(classId.getId());
	}

	public PlayerTemplate getTemplate(int classId)
	{
		return templates.get(classId);
	}
}
