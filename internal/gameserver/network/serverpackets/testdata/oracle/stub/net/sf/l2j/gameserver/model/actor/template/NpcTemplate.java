package net.sf.l2j.gameserver.model.actor.template;

// Stand-in for an npc template: the plain values the creature info packets read.
public final class NpcTemplate
{
	public int idTemplate;
	public int rightHand;
	public int leftHand;
	public double collisionRadius;
	public double collisionHeight;
	public boolean serverSideName;
	public boolean serverSideTitle;
	public int aggroRange;
	public String name = "";

	// Read only by the polymorphed player view.
	public int getNpcId()
	{
		return idTemplate;
	}

	public int getIdTemplate()
	{
		return idTemplate;
	}

	public int getRightHand()
	{
		return rightHand;
	}

	public int getLeftHand()
	{
		return leftHand;
	}

	public double getCollisionRadius()
	{
		return collisionRadius;
	}

	public double getCollisionHeight()
	{
		return collisionHeight;
	}

	public boolean isUsingServerSideName()
	{
		return serverSideName;
	}

	public boolean isUsingServerSideTitle()
	{
		return serverSideTitle;
	}

	public int getAggroRange()
	{
		return aggroRange;
	}

	public String getName()
	{
		return name;
	}
}
