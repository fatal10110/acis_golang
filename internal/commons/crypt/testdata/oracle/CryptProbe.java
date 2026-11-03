import java.lang.reflect.Method;
import java.math.BigInteger;
import java.util.HexFormat;

import net.sf.l2j.gameserver.network.gameserverpackets.GameServerBasePacket;
import net.sf.l2j.loginserver.crypt.BlowfishEngine;
import net.sf.l2j.loginserver.crypt.NewCrypt;
import net.sf.l2j.loginserver.crypt.ScrambledKeyPair;

// Prints checksum, Blowfish, game-server link and modulus-scramble vectors by
// calling the unmodified reference classes (compiled from the reference tree
// by run.sh). Inputs are fixed: the Go test's existing vectors plus a
// deterministic xorshift filler. "-" stands for an empty byte string.
public class CryptProbe
{
	private static final HexFormat HEX = HexFormat.of();
	private static int _seed = 0x2545F491;
	
	private static String hex(byte[] b)
	{
		return b.length == 0 ? "-" : HEX.formatHex(b);
	}
	
	private static byte[] unhex(String s)
	{
		return s.equals("-") ? new byte[0] : HEX.parseHex(s);
	}
	
	private static byte[] pattern(int n)
	{
		final byte[] b = new byte[n];
		for (int i = 0; i < n; i++)
			b[i] = (byte) (i * 0x11 + 1);
		return b;
	}
	
	private static byte[] fill(int n)
	{
		final byte[] b = new byte[n];
		for (int i = 0; i < n; i++)
		{
			_seed ^= _seed << 13;
			_seed ^= _seed >>> 17;
			_seed ^= _seed << 5;
			b[i] = (byte) _seed;
		}
		return b;
	}
	
	private static void checksum(byte[] in)
	{
		final boolean verifyIn = NewCrypt.verifyChecksum(in.clone());
		String out = "-";
		boolean verifyOut = false;
		if (in.length >= 8 && in.length % 4 == 0)
		{
			final byte[] buf = in.clone();
			NewCrypt.appendChecksum(buf);
			out = hex(buf);
			verifyOut = NewCrypt.verifyChecksum(buf);
		}
		System.out.println("checksum " + hex(in) + " " + verifyIn + " " + out + " " + verifyOut);
	}
	
	private static void blowfish(byte[] key, byte[] plain) throws Exception
	{
		final BlowfishEngine engine = new BlowfishEngine();
		engine.init(true, key);
		final byte[] out = new byte[plain.length];
		for (int off = 0; off < plain.length; off += 8)
			engine.processBlock(plain, off, out, off);
		System.out.println("blowfish " + hex(key) + " " + hex(plain) + " " + hex(out));
	}
	
	private static final class RawPacket extends GameServerBasePacket
	{
		RawPacket(byte[] payload)
		{
			for (byte b : payload)
				writeC(b);
		}
		
		@Override
		public byte[] getContent()
		{
			return getBytes();
		}
	}
	
	// Mirrors the game server's link send path: getContent(), appendChecksum, crypt.
	private static void link(byte[] key, byte[] payload) throws Exception
	{
		final NewCrypt crypt = (key == null) ? new NewCrypt("_;v.]05-31!|+-%xT!^[$\00") : new NewCrypt(key);
		final byte[] data = new RawPacket(payload).getContent();
		NewCrypt.appendChecksum(data);
		final byte[] padded = data.clone();
		final byte[] encrypted = crypt.crypt(data);
		System.out.println("link " + (key == null ? "-" : hex(key)) + " " + hex(payload) + " " + hex(padded) + " " + hex(encrypted));
	}
	
	private static void scramble(Method m, byte[] modulus) throws Exception
	{
		final BigInteger n = new BigInteger(1, modulus);
		final byte[] scrambled = (byte[]) m.invoke(null, n);
		System.out.println("scramble " + hex(n.toByteArray()) + " " + hex(scrambled));
	}
	
	public static void main(String[] args) throws Exception
	{
		System.out.println("# checksum <in> <verify(in)> <appendChecksum(in)|-> <verify(out)>");
		for (int n : new int[] { 0, 1, 4, 5, 8, 12, 16, 20, 21, 24, 28, 32, 40, 64 })
			checksum(pattern(n));
		for (int n : new int[] { 8, 12, 16, 24, 32, 40, 64, 128 })
			for (int k = 0; k < 4; k++)
				checksum(fill(n));
		
		System.out.println("# blowfish <key> <plaintext> <ciphertext, ECB per 8-byte block>");
		blowfish(unhex("0000000000000000"), unhex("0000000000000000"));
		blowfish(unhex("ab"), unhex("0011223344556677"));
		blowfish(unhex("0123456789abcdef"), unhex("fedcba9876543210"));
		blowfish(unhex("4162436445664768"), unhex("0000000000000000"));
		blowfish(unhex("6b60cb5b82ce90b1c26732cb164e0eff"), unhex("00112233445566778899aabbccddeeff0011223344556677"));
		for (int n : new int[] { 1, 4, 7, 8, 16, 21, 22, 32, 40, 55, 56 })
			for (int k = 0; k < 3; k++)
				blowfish(fill(n), fill(8 * (1 + k)));
		
		System.out.println("# link <key|- for the bootstrap key> <payload> <padded+checksum> <encrypted>");
		link(null, unhex("002a010200000102"));
		link(unhex("030a11181f262d343b424950575e656c"), unhex("68656c6c6f2c2067616d65736572766572206c696e6b"));
		link(null, new byte[0]);
		for (int n = 0; n <= 17; n++)
			link(null, fill(n));
		for (int keyLen : new int[] { 16, 40 })
			for (int n : new int[] { 0, 3, 4, 11, 12, 20, 33 })
				link(fill(keyLen), fill(n));
		
		System.out.println("# scramble <modulus as BigInteger.toByteArray()> <scrambled>");
		final Method m = ScrambledKeyPair.class.getDeclaredMethod("scrambleModulus", BigInteger.class);
		m.setAccessible(true);
		scramble(m, unhex("59170fbb183477a35a94c9bf390b770209d32a595d4a987d41f4b3e592471abe05194bb55dcab3910016175aa147e1f3e98ee81edf7fae40bfe793d9fced52b202e34d15e5c4caaf166fc2edcea4fb1b65e240a4e944b31d0705e6c8b71cf8bdfc97c696693058b22edd1479f1953297e1c82141a6a46d99364cf461b317c99f"));
		scramble(m, unhex("ee614cc5a74b029f8f814e38d93995b694aa2150847891d75da8ddddc46434afeb50781ae04364f9b3a3f3707eab79470ba3c885b66c499800da641a22a420ee9989ec985901e416c9c38fbf7ae22a00a4174c64b5144dc69831c2d5e9f8c7dcb1c3a276da03c5c5347dd8728b234273149eb6e9bb0ff5f4111a021c9a0168a9"));
		for (int k = 0; k < 6; k++)
		{
			final byte[] mod = fill(128);
			mod[0] = (byte) ((k % 2 == 0) ? (mod[0] | 0x80) : ((mod[0] & 0x7f) | 0x40));
			scramble(m, mod);
		}
	}
}
