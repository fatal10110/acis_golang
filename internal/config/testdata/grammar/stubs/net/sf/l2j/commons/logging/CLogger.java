package net.sf.l2j.commons.logging;

// Stand-in for the reference logger: the probe only needs ExProperties to
// compile and run, and its log lines are not part of the golden.
public class CLogger
{
	public CLogger(String name)
	{
	}
	
	public void warn(String message, Object... args)
	{
	}
	
	public void error(String message, Object... args)
	{
	}
}
