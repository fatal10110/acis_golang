import java.nio.ByteBuffer;

import net.sf.l2j.gameserver.enums.GeoType;
import net.sf.l2j.gameserver.geoengine.GeoCopy;
import net.sf.l2j.gameserver.geoengine.geodata.BlockComplex;
import net.sf.l2j.gameserver.geoengine.geodata.BlockFlat;
import net.sf.l2j.gameserver.geoengine.geodata.BlockMultilayer;
import net.sf.l2j.gameserver.geoengine.geodata.GeoStructure;
import net.sf.l2j.gameserver.model.World;
import net.sf.l2j.gameserver.model.location.Location;

/**
 * Replays canFly and getValidFlyLocation on a synthetic block grid (see README.md for the
 * layout, which fly_oracle_test.go builds identically) and prints one line per query.
 */
public class FlyProbe
{
	private static final byte ALL = 0x0F;
	
	// World origin of the geodata window, and the far corner of it.
	private static final int X0 = World.WORLD_X_MIN;
	private static final int Y0 = World.WORLD_Y_MIN;
	private static final int XMAX = World.WORLD_X_MAX;
	private static final int YMAX = World.WORLD_Y_MAX;
	
	private static GeoCopy geo;
	
	public static void main(String[] args)
	{
		BlockMultilayer.initialize();
		geo = new GeoCopy();
		buildWorld();
		
		cases();
	}
	
	// ---- synthetic geodata -------------------------------------------------------------
	
	private static short code(int height, int nswe)
	{
		return (short) (height * 2 | nswe);
	}
	
	private interface CellHeight
	{
		int at(int x, int y);
	}
	
	private static BlockComplex complex(CellHeight heights)
	{
		final ByteBuffer bb = ByteBuffer.allocate(GeoStructure.BLOCK_CELLS * 2);
		for (int x = 0; x < GeoStructure.BLOCK_CELLS_X; x++)
			for (int y = 0; y < GeoStructure.BLOCK_CELLS_Y; y++)
				bb.putShort(code(heights.at(x, y), ALL));
		bb.flip();
		return new BlockComplex(bb);
	}
	
	private static BlockFlat flat(int height)
	{
		final ByteBuffer bb = ByteBuffer.allocate(2);
		bb.putShort((short) height);
		bb.flip();
		return new BlockFlat(bb, GeoType.L2J);
	}
	
	private interface CellLayers
	{
		int[] at(int x, int y);
	}
	
	// layers are given lowest first; the file stores them highest first.
	private static BlockMultilayer multilayer(CellLayers layers)
	{
		final ByteBuffer bb = ByteBuffer.allocate(4096);
		for (int x = 0; x < GeoStructure.BLOCK_CELLS_X; x++)
			for (int y = 0; y < GeoStructure.BLOCK_CELLS_Y; y++)
			{
				final int[] cell = layers.at(x, y);
				bb.put((byte) cell.length);
				for (int i = cell.length - 1; i >= 0; i--)
					bb.putShort(code(cell[i], ALL));
			}
		bb.flip();
		return new BlockMultilayer(bb, GeoType.L2J);
	}
	
	// Blocks are placed by grid block coordinates; every other block is null geodata.
	private static void buildWorld()
	{
		// (0,0): a floor at 0 on cell columns 0-3 and a 200-high floor on columns 4-7.
		geo.setBlock(0, 0, complex((x, y) -> x >= 4 ? 200 : 0));
		// (1,0): flat floor at 50.
		geo.setBlock(1, 0, flat(50));
		// (2,0): floor 0 with a trench 200 deep on cell columns 3-4.
		geo.setBlock(2, 0, complex((x, y) -> x == 3 || x == 4 ? -200 : 0));
		// (0,2): a ramp, 8 units higher per cell in x and in y.
		geo.setBlock(0, 2, complex((x, y) -> 8 * (x + y)));
		// (1,2): floor 0 with a ceiling at 152 on cells whose x+y is even; column 7 has a
		// middle layer at 80 and a ceiling at 400 instead.
		geo.setBlock(1, 2, multilayer((x, y) -> x == 7 ? new int[] { 0, 80, 400 } : (x + y) % 2 == 0 ? new int[] { 0, 152 } : new int[] { 0 }));
		// the three other corners of the grid: flat floor at 30.
		geo.setBlock(GeoStructure.GEO_BLOCKS_X - 1, 0, flat(30));
		geo.setBlock(0, GeoStructure.GEO_BLOCKS_Y - 1, flat(30));
		geo.setBlock(GeoStructure.GEO_BLOCKS_X - 1, GeoStructure.GEO_BLOCKS_Y - 1, flat(30));
	}
	
	// ---- queries -------------------------------------------------------------------------
	
	private static void fly(String name, int ox, int oy, int oz, double oheight, int tx, int ty, int tz)
	{
		final boolean can = geo.canFly(ox, oy, oz, oheight, tx, ty, tz, null);
		final Location stop = geo.getValidFlyLocation(ox, oy, oz, oheight, tx, ty, tz, null);
		System.out.println("fly\t" + name + "\t" + ox + " " + oy + " " + oz + "\t" + oheight + "\t" + tx + " " + ty + " " + tz + "\t" + can + "\t" + stop.x + " " + stop.y + " " + stop.z);
	}
	
	private static void cases()
	{
		// the hand-traced cases of fly_test.go, on block (0,0) and null geodata
		fly("level line over the low floor", X0 + 8, Y0 + 8, 100, 32, X0 + 56, Y0 + 8, 100);
		fly("level line into the high floor", X0 + 8, Y0 + 8, 100, 32, X0 + 104, Y0 + 8, 100);
		fly("level line over the high floor", X0 + 8, Y0 + 8, 250, 32, X0 + 120, Y0 + 8, 250);
		fly("descent into the floor", X0 + 8, Y0 + 8, 40, 32, X0 + 56, Y0 + 8, -40);
		fly("null geodata", X0 + 8, Y0 + 8 + 128, 100, 32, X0 + 56, Y0 + 8 + 128, 100);
		fly("out of the grid", X0 + 8, Y0 + 8, 100, 32, X0 - 92, Y0 + 8, 100);
		fly("same cell", X0 + 8, Y0 + 8, 100, 32, X0 + 12, Y0 + 3, -500);
		
		// diagonals in every direction over block (0,0) and the ramp (0,2)
		fly("diagonal se over the steps", X0 + 8, Y0 + 8, 300, 32, X0 + 120, Y0 + 120, 300);
		fly("diagonal nw over the steps", X0 + 120, Y0 + 120, 300, 32, X0 + 8, Y0 + 8, 300);
		fly("diagonal ne over the steps", X0 + 8, Y0 + 120, 300, 32, X0 + 120, Y0 + 8, 300);
		fly("diagonal sw over the steps", X0 + 120, Y0 + 8, 300, 32, X0 + 8, Y0 + 120, 300);
		fly("diagonal se into the steps", X0 + 8, Y0 + 8, 100, 32, X0 + 120, Y0 + 120, 100);
		fly("diagonal sw into the steps", X0 + 8, Y0 + 8, 100, 32, X0 - 40, Y0 + 120, 100);
		fly("diagonal up the ramp", X0 + 8, Y0 + 264, 100, 32, X0 + 120, Y0 + 376, 100);
		fly("diagonal down the ramp", X0 + 120, Y0 + 376, 130, 32, X0 + 8, Y0 + 264, 130);
		fly("diagonal across the ramp", X0 + 120, Y0 + 264, 90, 32, X0 + 8, Y0 + 376, 90);
		
		// shallow and steep slopes, horizontal and vertical lines
		fly("shallow slope over steps and flat", X0 + 8, Y0 + 8, 250, 32, X0 + 250, Y0 + 40, 250);
		fly("shallow descent over steps and flat", X0 + 8, Y0 + 8, 250, 32, X0 + 250, Y0 + 100, 40);
		fly("steep ascent over flat", X0 + 136, Y0 + 8, 60, 32, X0 + 236, Y0 + 8, 2000);
		fly("steep descent into flat", X0 + 136, Y0 + 8, 300, 32, X0 + 232, Y0 + 8, -100);
		fly("level line under flat floor", X0 + 136, Y0 + 8, 40, 32, X0 + 232, Y0 + 8, 40);
		fly("level line on flat floor", X0 + 136, Y0 + 8, 50, 32, X0 + 232, Y0 + 8, 50);
		fly("steep slope across cells", X0 + 136, Y0 + 8, 120, 32, X0 + 152, Y0 + 120, 120);
		fly("vertical south over flat", X0 + 136, Y0 + 8, 100, 32, X0 + 136, Y0 + 120, 100);
		fly("vertical north over flat", X0 + 136, Y0 + 120, 100, 32, X0 + 136, Y0 + 8, 100);
		fly("vertical south into null geodata", X0 + 136, Y0 + 8, 100, 32, X0 + 136, Y0 + 200, 100);
		fly("horizontal west over flat", X0 + 248, Y0 + 8, 100, 32, X0 + 136, Y0 + 8, 100);
		fly("horizontal east over the trench", X0 + 264, Y0 + 8, 100, 32, X0 + 376, Y0 + 8, 100);
		fly("into the trench and out", X0 + 300, Y0 + 8, -100, 32, X0 + 340, Y0 + 8, -100);
		fly("down into the trench", X0 + 264, Y0 + 8, 50, 32, X0 + 312, Y0 + 8, -150);
		
		// cell borders
		fly("origin on the last unit of a cell", X0 + 15, Y0 + 8, 250, 32, X0 + 100, Y0 + 8, 250);
		fly("origin on the first unit of a cell going west", X0 + 16, Y0 + 8, 250, 32, X0 + 2, Y0 + 8, 250);
		fly("origin on the last row of a cell going south", X0 + 8, Y0 + 15, 250, 32, X0 + 8, Y0 + 40, 250);
		fly("target on a cell border", X0 + 8, Y0 + 8, 250, 32, X0 + 64, Y0 + 8, 250);
		
		// multilayer block (1,2): ceilings at 152 on even cells, three layers on column 7
		fly("ceiling clears the corridor", X0 + 136, Y0 + 264, 100, 32, X0 + 248, Y0 + 264, 100);
		fly("ceiling blocks the corridor", X0 + 136, Y0 + 264, 130, 32, X0 + 248, Y0 + 264, 130);
		fly("ceiling exactly at the corridor top", X0 + 136, Y0 + 264, 120, 32, X0 + 248, Y0 + 264, 120);
		fly("floor above the line in a multilayer block", X0 + 136, Y0 + 264, -10, 32, X0 + 248, Y0 + 264, -10);
		fly("middle layer under the line", X0 + 200, Y0 + 264, 300, 32, X0 + 248, Y0 + 264, 300);
		fly("middle layer above the line", X0 + 200, Y0 + 264, 60, 32, X0 + 248, Y0 + 264, 60);
		fly("top layer in the corridor", X0 + 200, Y0 + 264, 380, 32, X0 + 248, Y0 + 264, 380);
		fly("vertical south through ceilings", X0 + 136, Y0 + 264, 130, 32, X0 + 136, Y0 + 376, 130);
		fly("vertical south under ceilings", X0 + 136, Y0 + 264, 100, 32, X0 + 136, Y0 + 376, 100);
		fly("diagonal through ceilings", X0 + 136, Y0 + 264, 200, 32, X0 + 248, Y0 + 376, 20);
		fly("diagonal under ceilings", X0 + 136, Y0 + 264, 100, 32, X0 + 240, Y0 + 376, 100);
		fly("from the ramp into the multilayer block", X0 + 8, Y0 + 264, 140, 32, X0 + 248, Y0 + 280, 140);
		
		// the corridor height is truncated to an int
		fly("fractional corridor height", X0 + 136, Y0 + 264, 120, 32.9, X0 + 248, Y0 + 264, 120);
		fly("corridor height one more", X0 + 136, Y0 + 264, 120, 33, X0 + 248, Y0 + 264, 120);
		fly("corridor height one more fractional", X0 + 136, Y0 + 264, 120, 33.9, X0 + 248, Y0 + 264, 120);
		fly("zero corridor height", X0 + 136, Y0 + 264, 150, 0, X0 + 248, Y0 + 264, 150);
		fly("negative fractional corridor height", X0 + 136, Y0 + 264, 150, -3.9, X0 + 248, Y0 + 264, 150);
		fly("huge corridor height", X0 + 136, Y0 + 264, 120, 1e10, X0 + 248, Y0 + 264, 120);
		fly("not a number corridor height", X0 + 136, Y0 + 264, 120, Double.NaN, X0 + 248, Y0 + 264, 120);
		
		// the squared deltas are summed in 32-bit int
		fly("long line just under the wrap", X0 + 8, Y0 + 1032, 0, 0, X0 + 46000, Y0 + 1032, 40);
		fly("long line just over the wrap", X0 + 8, Y0 + 1032, 0, 0, X0 + 46349, Y0 + 1032, 40);
		fly("long line wrapping to a negative sum", X0 + 8, Y0 + 1032, 0, 0, X0 + 50008, Y0 + 1032, 40);
		fly("long line wrapping to a positive sum", X0 + 8, Y0 + 1032, 0, 0, X0 + 100008, Y0 + 1032, 40);
		fly("long vertical line wrapping", X0 + 1032, Y0 + 8, 0, 0, X0 + 1032, Y0 + 50008, 40);
		fly("long diagonal wrapping", X0 + 1032, Y0 + 1032, 0, 0, X0 + 51032, Y0 + 51032, -40);
		
		// the walk stops at the border of the geodata grid
		fly("north out of the grid", X0 + 8, Y0 + 8, 100, 32, X0 + 8, Y0 - 100, 100);
		fly("north west out of the grid", X0 + 8, Y0 + 8, 100, 32, X0 - 100, Y0 - 100, 100);
		fly("west out of the grid over the flat corner", X0 + 8, YMAX - 100, 100, 32, X0 - 100, YMAX - 100, 100);
		fly("east out of the grid", XMAX - 100, Y0 + 8, 100, 32, XMAX + 200, Y0 + 8, 100);
		fly("east to the edge", XMAX - 100, Y0 + 8, 100, 32, XMAX, Y0 + 8, 100);
		fly("south out of the grid", X0 + 8, YMAX - 100, 100, 32, X0 + 8, YMAX + 300, 100);
		fly("south east out of the grid", XMAX - 100, YMAX - 100, 100, 32, XMAX + 100, YMAX + 100, 100);
		fly("north east out of the grid", XMAX - 100, Y0 + 100, 100, 32, XMAX + 100, Y0 - 100, 100);
		fly("south west out of the grid", X0 + 100, YMAX - 100, 100, 32, X0 - 100, YMAX + 100, 100);
		fly("shallow line out of the grid", XMAX - 100, Y0 + 8, 100, 32, XMAX + 5000, Y0 + 60, 100);
		fly("out of the world through null geodata", X0 + 8, Y0 + 2056, 100, 32, X0 + 8, Y0 - 100, 100);
		fly("origin on the grid border", X0, Y0, 100, 32, X0 + 40, Y0 + 40, 100);
		fly("target on the world edge", X0 + 8, Y0 + 8, 100, 32, X0, Y0, 100);
	}
}
