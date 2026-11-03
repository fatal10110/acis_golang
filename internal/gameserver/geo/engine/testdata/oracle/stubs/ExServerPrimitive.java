package net.sf.l2j.gameserver.network.serverpackets;

import java.awt.Color;

/**
 * Stand-in for the debug packet: the probe always passes null, so only the signatures the cut
 * methods call have to compile.
 */
public final class ExServerPrimitive
{
	public void addSquare(Color color, int x, int y, int z, int size)
	{
	}
	
	public void addPoint(String name, Color color, boolean isNameColored, int x, int y, int z)
	{
	}
}
