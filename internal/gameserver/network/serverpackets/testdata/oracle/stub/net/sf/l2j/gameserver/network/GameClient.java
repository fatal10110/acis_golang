package net.sf.l2j.gameserver.network;

import java.nio.ByteBuffer;

import net.sf.l2j.commons.mmocore.MMOClient;
import net.sf.l2j.commons.mmocore.MMOConnection;

// Stand-in client: packets never read it unless writeImpl throws.
public final class GameClient extends MMOClient<MMOConnection<GameClient>>
{
	public GameClient()
	{
		super(null);
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
