package net.sf.l2j;

// Stand-in for the reference configuration class: the packet base class reads
// the debug flag and the manor seed limits the drop rate.
public final class Config
{
	public static boolean PACKET_HANDLER_DEBUG = false;
	public static int RATE_DROP_MANOR = 2;
}
