import java.util.Locale;

/**
 * Drives the reference vitals code (VitalsCopy, cut from the reference sources
 * by run.sh) and prints one golden line per observation; formats in README.md.
 */
public final class VitalsProbe
{
	public static void main(String[] args)
	{
		Locale.setDefault(Locale.ROOT);

		// Door damage stage by remaining HP, in half-point steps, as runs.
		for (int max : new int[]
		{
			600,
			100000
		})
		{
			final VitalsCopy.Door door = new VitalsCopy.Door();
			door._status.maxHp = max;
			final StringBuilder runs = new StringBuilder();
			int last = -1;
			final int step = max / 1200 > 0 ? max / 1200 : 1;
			for (double hp = 0; hp <= max; hp += 0.5 * step)
			{
				door._status._hp = hp;
				final int stage = door.getDamage();
				if (stage != last)
					runs.append(' ').append(hp).append(':').append(stage);
				last = stage;
			}
			System.out.println("damage " + max + runs);
		}

		// Regeneration period by kind of creature.
		System.out.println("period door " + VitalsCopy.Formulas.getRegeneratePeriod(new VitalsCopy.Door()));
		System.out.println("period other " + VitalsCopy.Formulas.getRegeneratePeriod(new VitalsCopy.Creature(false)));

		// Regeneration ticks of a damaged door of 600 HP from its HP at the
		// first hit until the task stops: hp:running after each tick, under the
		// default HP regeneration multiplier and a raised one.
		for (double mult : new double[]
		{
			1,
			4
		})
		{
			VitalsCopy.Config.HP_REGEN_MULTIPLIER = mult;
			for (double start : new double[]
			{
				597,
				595,
				590
			})
			{
				final VitalsCopy.Door door = new VitalsCopy.Door();
				door._status.maxHp = 600;
				door._status._mp = 0;
				door._status.setHp(start, false);
				final StringBuilder ticks = new StringBuilder();
				for (int i = 0; i < 1000 && door._status.regTask; i++)
				{
					door._status.doRegeneration();
					ticks.append(' ').append(door._status._hp).append(':').append(door._status.regTask ? 1 : 0);
				}
				System.out.println("regen " + mult + " " + start + ticks);
			}
		}
		VitalsCopy.Config.HP_REGEN_MULTIPLIER = 1;

		// HP of a revived creature of that maximum.
		for (int max : new int[]
		{
			410,
			600,
			632,
			100000
		})
		{
			final VitalsCopy.Door door = new VitalsCopy.Door();
			door._status.maxHp = max;
			door.doRevive();
			System.out.println("revive " + max + " " + door._status._hp);
		}

		// A pet's maximum HP from its base maximum and constitution.
		for (int con : new int[]
		{
			0,
			21,
			43,
			99
		})
		{
			for (int base : new int[]
			{
				100,
				400,
				500,
				1000
			})
			{
				final VitalsCopy.Creature pet = new VitalsCopy.Creature(true);
				pet.petData.base = base;
				pet._status.con = con;
				System.out.println("petmaxhp " + base + " " + con + " " + pet._status.getMaxHp());
			}
		}

		// Experience a resurrection of that power gives back to a pet.
		final int[][] cases =
		{
			{
				510,
				481
			},
			{
				700,
				671
			},
			{
				1000,
				900
			},
			{
				0,
				0
			}
		};
		for (int[] c : cases)
		{
			for (double power : new double[]
			{
				0,
				10,
				50,
				89,
				90,
				100
			})
			{
				final VitalsCopy.PetExp pet = new VitalsCopy.PetExp();
				pet._expBeforeDeath = c[0];
				pet.exp = c[1];
				pet.restoreExp(power);
				System.out.println("petexp " + c[0] + " " + c[1] + " " + power + " " + (pet.exp - c[1]));
			}
		}

		// HP and MP of a dead player or summon right after a revive: a plain one
		// restores the share of the maximum, a phoenix-blessed one the maximum.
		for (boolean phoenix : new boolean[]
		{
			false,
			true
		})
		{
			for (int[] max : new int[][]
			{
				{
					410,
					200
				},
				{
					632,
					97
				},
				{
					1500,
					3000
				}
			})
			{
				final VitalsCopy.Playable who = new VitalsCopy.Playable();
				who._status.maxHp = max[0];
				who._status.maxMp = max[1];
				who.dead = true;
				who.phoenixBlessed = phoenix;
				who.doRevive();
				System.out.println("playablerevive " + (phoenix ? 1 : 0) + " " + max[0] + " " + max[1] + " " + who._status._hp + " " + who._status._mp + " " + (who.phoenixBlessed ? 1 : 0));
			}
		}

		// HP of the doors of a castle after its doors respawn, weak or not, from
		// a standing or a dead door.
		for (int max : new int[]
		{
			410,
			600,
			601
		})
		{
			for (boolean weak : new boolean[]
			{
				false,
				true
			})
			{
				for (boolean dead : new boolean[]
				{
					false,
					true
				})
				{
					final VitalsCopy.Castle castle = new VitalsCopy.Castle();
					final VitalsCopy.Door door = new VitalsCopy.Door();
					door._status.maxHp = max;
					door._status._hp = dead ? 0 : max / 3;
					door.dead = dead;
					castle._doors.add(door);
					castle.spawnDoors(weak);
					System.out.println("spawndoors " + max + " " + (weak ? 1 : 0) + " " + (dead ? 1 : 0) + " " + door._status._hp + " " + (door.dead ? 1 : 0));
				}
			}
		}

		// A pet that dies: level, the experience the levels before and after
		// start at, its experience; the loss, the experience kept for a
		// resurrection, and the experience and level afterwards.
		final long[][] deaths =
		{
			{
				10,
				200,
				500,
				1000,
				700
			},
			{
				10,
				200,
				500,
				1000,
				510
			},
			{
				10,
				200,
				500,
				1000,
				10
			},
			{
				10,
				200,
				500,
				1000,
				500
			},
			{
				10,
				200,
				500,
				1000,
				529
			},
			{
				10,
				200,
				500,
				1000,
				530
			},
			{
				10,
				200,
				500,
				750,
				600
			},
			{
				10,
				200,
				500,
				757,
				600
			},
			{
				44,
				700000,
				800000,
				1000000,
				900000
			},
			{
				10,
				900,
				1000,
				2500,
				1500
			},
			{
				1,
				0,
				0,
				136,
				100
			},
			{
				1,
				0,
				0,
				136,
				5
			},
			{
				99,
				4900000,
				5000000,
				5200000,
				5100000
			},
			{
				79,
				400000000,
				480562077,
				555934039,
				490562077
			}
		};
		for (long[] d : deaths)
		{
			final VitalsCopy.PetExp pet = new VitalsCopy.PetExp();
			final int level = (int) d[0];
			pet.level = level;
			pet.expForLevel[level - 1] = d[1];
			pet.expForLevel[level] = d[2];
			pet.expForLevel[level + 1] = d[3];
			pet.exp = d[4];
			pet.die();
			System.out.println("petdeath " + level + " " + d[1] + " " + d[2] + " " + d[3] + " " + d[4] + " " + (d[4] - pet.exp) + " " + pet._expBeforeDeath + " " + pet.exp + " " + pet.level);
		}
	}
}
