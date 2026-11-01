package net.sf.l2j.gameserver.model;

// Stand-in for an item augmentation: only the packed bonus id.
public final class Augmentation
{
	private final int id;

	public Augmentation(int id)
	{
		this.id = id;
	}

	public int getId()
	{
		return id;
	}
}
