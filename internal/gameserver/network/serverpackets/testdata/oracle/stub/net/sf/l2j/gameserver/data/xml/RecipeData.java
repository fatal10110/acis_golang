package net.sf.l2j.gameserver.data.xml;

import java.util.HashMap;
import java.util.Map;

import net.sf.l2j.gameserver.model.records.Recipe;

// Stand-in for the recipe table.
public final class RecipeData
{
	private static final RecipeData INSTANCE = new RecipeData();

	public final Map<Integer, Recipe> recipes = new HashMap<>();

	public static RecipeData getInstance()
	{
		return INSTANCE;
	}

	public Recipe getRecipeList(int listId)
	{
		return recipes.get(listId);
	}
}
