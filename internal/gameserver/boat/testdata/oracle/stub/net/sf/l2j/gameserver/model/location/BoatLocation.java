package net.sf.l2j.gameserver.model.location;

// Stand-in for the dock's tie-up point: the dock geometry only reads its
// position.
public class BoatLocation extends Location
{
	public BoatLocation(int x, int y, int z, int moveSpeed, int rotationSpeed)
	{
		super(x, y, z);
	}
}
