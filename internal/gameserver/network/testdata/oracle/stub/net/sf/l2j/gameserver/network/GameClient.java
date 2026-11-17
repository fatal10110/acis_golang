package net.sf.l2j.gameserver.network;

import net.sf.l2j.commons.mmocore.MMOConnection;
import net.sf.l2j.commons.mmocore.ReceivablePacket;

/** Probe stub: the client's lifecycle state and nothing else of the reference class. */
public class GameClient
{
	public enum GameClientState
	{
		CONNECTED,
		AUTHED,
		ENTERING,
		IN_GAME
	}
	
	private GameClientState _state = GameClientState.CONNECTED;
	
	public GameClient(MMOConnection<GameClient> con)
	{
	}
	
	public boolean dropPacket()
	{
		return false;
	}
	
	public GameClientState getState()
	{
		return _state;
	}
	
	public void setState(GameClientState state)
	{
		_state = state;
	}
	
	/** Unknown packets counted so far. */
	public int unknown;
	
	public void onUnknownPacket()
	{
		unknown++;
	}
	
	public void execute(ReceivablePacket<GameClient> rp)
	{
	}
}
