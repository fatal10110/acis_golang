import java.io.DataInputStream;
import java.io.IOException;
import java.io.InputStream;
import java.lang.reflect.Method;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * Minimal class-file reader: for each method, the parent calls (non-constructor
 * invokespecial to another class) in bytecode order, with how their arguments are
 * loaded and where they sit in the method.
 */
final class ClassBytes
{
	private ClassBytes()
	{
	}

	/** One parent call. */
	record SuperCall(String owner, String name, String desc, boolean sameArgs, boolean first, boolean tail)
	{
		/**
		 * Rendered as {@code super=<name>[@<owner>](args=same|changed,at=first|tail|mid|only)}.
		 * The owner is written only when it is not the direct superclass.
		 */
		String render(Class<?> c, Method m, String callerDesc)
		{
			final String direct = c.getSuperclass().getName().replace('.', '/');
			final String own = owner.equals(direct) ? "" : "@" + ScriptProbe.rel(owner.replace('/', '.'));
			final String nm = name.equals(m.getName()) && desc.equals(callerDesc) ? "" : ":" + name;
			final String at = first && tail ? "only" : first ? "first" : tail ? "tail" : "mid";
			return "super" + nm + own + "(args=" + (sameArgs ? "same" : "changed") + ",at=" + at + ")";
		}
	}

	record MethodCode(List<SuperCall> superCalls)
	{
	}

	static Map<String, MethodCode> read(Class<?> c) throws IOException
	{
		final String self = c.getName().replace('.', '/');
		try (InputStream in = c.getClassLoader().getResourceAsStream(self + ".class"))
		{
			final DataInputStream d = new DataInputStream(in);
			d.readInt();
			d.readUnsignedShort();
			d.readUnsignedShort();
			final int cpCount = d.readUnsignedShort();
			final Object[] cp = new Object[cpCount];
			final int[][] refs = new int[cpCount][];
			for (int i = 1; i < cpCount; i++)
			{
				final int tag = d.readUnsignedByte();
				switch (tag)
				{
					case 1 -> cp[i] = d.readUTF();
					case 3, 4 -> d.readInt();
					case 5, 6 ->
					{
						d.readLong();
						i++;
					}
					case 7, 8, 16, 19, 20 -> refs[i] = new int[]
					{
						tag,
						d.readUnsignedShort()
					};
					case 9, 10, 11, 12, 17, 18 -> refs[i] = new int[]
					{
						tag,
						d.readUnsignedShort(),
						d.readUnsignedShort()
					};
					case 15 ->
					{
						d.readUnsignedByte();
						d.readUnsignedShort();
					}
					default -> throw new IOException("constant tag " + tag + " in " + self);
				}
			}
			d.readUnsignedShort();
			d.readUnsignedShort();
			d.readUnsignedShort();
			final int ifaces = d.readUnsignedShort();
			for (int i = 0; i < ifaces; i++)
				d.readUnsignedShort();
			final int fields = d.readUnsignedShort();
			for (int i = 0; i < fields; i++)
			{
				d.readUnsignedShort();
				d.readUnsignedShort();
				d.readUnsignedShort();
				skipAttributes(d);
			}
			final Map<String, MethodCode> out = new HashMap<>();
			final int methods = d.readUnsignedShort();
			for (int i = 0; i < methods; i++)
			{
				final int access = d.readUnsignedShort();
				final String name = (String) cp[d.readUnsignedShort()];
				final String desc = (String) cp[d.readUnsignedShort()];
				final int attrs = d.readUnsignedShort();
				for (int a = 0; a < attrs; a++)
				{
					final String an = (String) cp[d.readUnsignedShort()];
					final int len = d.readInt();
					final byte[] body = d.readNBytes(len);
					if (an.equals("Code"))
						out.put(name + desc, new MethodCode(scan(body, cp, refs, self, desc, (access & 0x0008) != 0)));
				}
			}
			return out;
		}
	}

	private static void skipAttributes(DataInputStream d) throws IOException
	{
		final int attrs = d.readUnsignedShort();
		for (int a = 0; a < attrs; a++)
		{
			d.readUnsignedShort();
			d.skipNBytes(d.readInt());
		}
	}

	/** One decoded instruction: offset, opcode, and the local index or pool index it uses. */
	private record Insn(int pc, int op, int arg)
	{
	}

	private static List<SuperCall> scan(byte[] attr, Object[] cp, int[][] refs, String self, String callerDesc, boolean isStatic)
	{
		// Code attribute: max_stack u2, max_locals u2, code_length u4, code.
		final int codeLen = ((attr[4] & 0xff) << 24) | ((attr[5] & 0xff) << 16) | ((attr[6] & 0xff) << 8) | (attr[7] & 0xff);
		final byte[] code = new byte[codeLen];
		System.arraycopy(attr, 8, code, 0, codeLen);

		final List<Insn> insns = new ArrayList<>();
		for (int pc = 0; pc < code.length;)
		{
			final int op = code[pc] & 0xff;
			int arg = -1;
			int len;
			if (op == 0xc4)
			{
				// wide
				final int wop = code[pc + 1] & 0xff;
				arg = u2(code, pc + 2);
				insns.add(new Insn(pc, wop, arg));
				pc += (wop == 0x84) ? 6 : 4;
				continue;
			}
			if (op == 0xaa || op == 0xab)
			{
				int p = (pc + 4) & ~3;
				if (op == 0xaa)
				{
					final int low = s4(code, p + 4);
					final int high = s4(code, p + 8);
					p += 12 + (high - low + 1) * 4;
				}
				else
					p += 8 + s4(code, p + 4) * 8;
				insns.add(new Insn(pc, op, -1));
				pc = p;
				continue;
			}
			len = operandLength(op);
			if (op >= 0x1a && op <= 0x2d)
				arg = (op - 0x1a) % 4;
			else if (op >= 0x15 && op <= 0x19)
				arg = code[pc + 1] & 0xff;
			else if (op == 0xb7)
				arg = u2(code, pc + 1);
			insns.add(new Insn(pc, op, arg));
			pc += 1 + len;
		}

		final List<SuperCall> calls = new ArrayList<>();
		for (int i = 0; i < insns.size(); i++)
		{
			final Insn in = insns.get(i);
			if (in.op() != 0xb7)
				continue;
			final int[] ref = refs[in.arg()];
			final String owner = (String) cp[refs[ref[1]][1]];
			final int[] nat = refs[ref[2]];
			final String name = (String) cp[nat[1]];
			final String desc = (String) cp[nat[2]];
			if (name.equals("<init>") || owner.equals(self))
				continue;

			// Arguments: the receiver then one load per parameter, straight from the caller's slots.
			final List<Character> params = paramKinds(desc);
			final int start = i - params.size() - 1;
			boolean same = !isStatic && start >= 0 && desc.equals(callerDesc);
			if (same)
			{
				same = isLoad(insns.get(start), 'L', 0);
				int slot = 1;
				for (int p = 0; same && p < params.size(); p++)
				{
					same = isLoad(insns.get(start + 1 + p), params.get(p), slot);
					slot += (params.get(p) == 'J' || params.get(p) == 'D') ? 2 : 1;
				}
			}
			final boolean first = start >= 0 && insns.get(start).pc() == 0 && isLoad(insns.get(start), 'L', 0);
			boolean tail = i + 1 < insns.size() && insns.get(i + 1).op() >= 0xac && insns.get(i + 1).op() <= 0xb1;
			// A void parent call whose result is discarded before the return still ends the method.
			if (!tail && i + 2 < insns.size() && (insns.get(i + 1).op() == 0x57 || insns.get(i + 1).op() == 0x58))
				tail = insns.get(i + 2).op() >= 0xac && insns.get(i + 2).op() <= 0xb1;
			calls.add(new SuperCall(owner, name, desc, same, first, tail));
		}
		return calls;
	}

	private static boolean isLoad(Insn in, char kind, int slot)
	{
		final int base;
		final int shortBase;
		switch (kind)
		{
			case 'I', 'Z', 'B', 'C', 'S' ->
			{
				base = 0x15;
				shortBase = 0x1a;
			}
			case 'J' ->
			{
				base = 0x16;
				shortBase = 0x1e;
			}
			case 'F' ->
			{
				base = 0x17;
				shortBase = 0x22;
			}
			case 'D' ->
			{
				base = 0x18;
				shortBase = 0x26;
			}
			default ->
			{
				base = 0x19;
				shortBase = 0x2a;
			}
		}
		if (in.op() == base)
			return in.arg() == slot;
		return in.op() >= shortBase && in.op() < shortBase + 4 && in.arg() == slot;
	}

	/** One char per parameter: the descriptor's base type, 'L' for references and arrays. */
	static List<Character> paramKinds(String desc)
	{
		final List<Character> out = new ArrayList<>();
		for (int i = 1; desc.charAt(i) != ')';)
		{
			char ch = desc.charAt(i);
			if (ch == '[')
			{
				while (desc.charAt(i) == '[')
					i++;
				if (desc.charAt(i) == 'L')
					i = desc.indexOf(';', i);
				i++;
				out.add('L');
				continue;
			}
			if (ch == 'L')
			{
				i = desc.indexOf(';', i) + 1;
				out.add('L');
				continue;
			}
			out.add(ch);
			i++;
		}
		return out;
	}

	static String descriptor(Method m)
	{
		final StringBuilder sb = new StringBuilder("(");
		for (Class<?> p : m.getParameterTypes())
			sb.append(p.descriptorString());
		return sb.append(')').append(m.getReturnType().descriptorString()).toString();
	}

	private static int operandLength(int op)
	{
		if (op == 0x10 || op == 0x12 || (op >= 0x15 && op <= 0x19) || (op >= 0x36 && op <= 0x3a) || op == 0xa9 || op == 0xbc)
			return 1;
		if (op == 0x11 || op == 0x13 || op == 0x14 || op == 0x84 || (op >= 0x99 && op <= 0xa8) || (op >= 0xb2 && op <= 0xb8) || op == 0xbb || op == 0xbd || op == 0xc0 || op == 0xc1 || op == 0xc6 || op == 0xc7)
			return 2;
		if (op == 0xc5)
			return 3;
		if (op == 0xb9 || op == 0xba || op == 0xc8 || op == 0xc9)
			return 4;
		return 0;
	}

	private static int u2(byte[] b, int p)
	{
		return ((b[p] & 0xff) << 8) | (b[p + 1] & 0xff);
	}

	private static int s4(byte[] b, int p)
	{
		return ((b[p] & 0xff) << 24) | ((b[p + 1] & 0xff) << 16) | ((b[p + 2] & 0xff) << 8) | (b[p + 3] & 0xff);
	}
}
