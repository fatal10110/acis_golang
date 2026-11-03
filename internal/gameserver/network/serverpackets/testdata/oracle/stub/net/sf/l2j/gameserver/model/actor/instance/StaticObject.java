package net.sf.l2j.gameserver.model.actor.instance;

import net.sf.l2j.gameserver.model.WorldObject;

// Stand-in for a static object: its template id (the object id is the base's).
public final class StaticObject extends WorldObject
{
	public int staticObjectId;

	public int getStaticObjectId()
	{
		return staticObjectId;
	}
}
