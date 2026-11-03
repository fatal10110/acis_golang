package net.sf.l2j.gameserver.model.craft;

import java.util.ArrayList;
import java.util.Collection;
import java.util.List;

import net.sf.l2j.gameserver.model.records.Recipe;

// Stand-in for a recipe book: the dwarven and the common recipes.
public final class RecipeBook
{
	public final List<Recipe> dwarven = new ArrayList<>();
	public final List<Recipe> common = new ArrayList<>();

	public Collection<Recipe> get(boolean isDwarven)
	{
		return isDwarven ? dwarven : common;
	}

	public boolean hasRecipe(int id)
	{
		for (Recipe r : dwarven)
			if (r.id() == id)
				return true;
		for (Recipe r : common)
			if (r.id() == id)
				return true;
		return false;
	}
}
