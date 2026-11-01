package net.sf.l2j.gameserver.network;

import java.nio.ByteBuffer;

import net.sf.l2j.commons.mmocore.MMOClient;
import net.sf.l2j.commons.mmocore.MMOConnection;
import net.sf.l2j.gameserver.model.CharSelectSlot;

// Stand-in client: records the character slots a character list hands it.
public final class GameClient extends MMOClient<MMOConnection<GameClient>>
{
	public CharSelectSlot[] slots;

	public GameClient()
	{
		super(null);
	}

	public void setCharSelectSlot(CharSelectSlot[] list)
	{
		slots = list;
	}

	public static void deleteCharByObjId(int objectId)
	{
	}

	@Override
	public boolean decrypt(ByteBuffer buf, int size)
	{
		return true;
	}

	@Override
	public boolean encrypt(ByteBuffer buf, int size)
	{
		return true;
	}

	@Override
	protected void onDisconnection()
	{
	}

	@Override
	protected void onForcedDisconnection()
	{
	}
}
