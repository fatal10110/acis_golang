package xml

import (
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/multisell"
)

type multiSellFile struct {
	Attrs []xml.Attr        `xml:",any,attr"`
	NPCs  []multiSellNPCSet `xml:"npcs"`
	Items []multiSellItem   `xml:"item"`
}

type multiSellNPCSet struct {
	IDs []int32 `xml:"npc"`
}

type multiSellItem struct {
	Ingredients []attrsElement `xml:"ingredient"`
	Products    []attrsElement `xml:"production"`
}

// LoadMultiSellLists parses every ".xml" file directly under dir and returns
// the loaded lists keyed by the bare filename's legacy hash. If items is
// non-nil, ingredient/product templates are resolved against it.
func LoadMultiSellLists(dir string, items *item.Table) (*multisell.Table, error) {
	docs, err := loadXMLDocuments[multiSellFile](dir, "multisell")
	if err != nil {
		return nil, err
	}

	lists := make([]*multisell.List, 0, len(docs))
	for _, doc := range docs {
		list, err := buildMultiSellList(doc.Path, doc.Data, items)
		if err != nil {
			return nil, err
		}
		lists = append(lists, list)
	}
	return multisell.NewTable(lists)
}

func buildMultiSellList(path string, file multiSellFile, items *item.Table) (*multisell.List, error) {
	id := commons.LegacyStringHash(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	set := commons.StatSetFromXMLAttrs(file.Attrs)

	list := &multisell.List{
		ID:                  id,
		ApplyTaxes:          set.GetBoolDefault("applyTaxes", false),
		MaintainEnchantment: set.GetBoolDefault("maintainEnchantment", false),
		Entries:             make([]multisell.Entry, 0, len(file.Items)),
	}

	for _, npcSet := range file.NPCs {
		list.NPCIDs = append(list.NPCIDs, npcSet.IDs...)
	}

	for itemIndex, el := range file.Items {
		ingredients := make([]multisell.Ingredient, 0, len(el.Ingredients))
		for _, ingredientEl := range el.Ingredients {
			in, err := multisell.NewIngredient(commons.StatSetFromXMLAttrs(ingredientEl.Attrs), items)
			if err != nil {
				return nil, fmt.Errorf("data/xml: %s: item %d ingredient: %w", path, itemIndex+1, err)
			}
			ingredients = append(ingredients, in)
		}

		products := make([]multisell.Ingredient, 0, len(el.Products))
		for _, productEl := range el.Products {
			in, err := multisell.NewIngredient(commons.StatSetFromXMLAttrs(productEl.Attrs), items)
			if err != nil {
				return nil, fmt.Errorf("data/xml: %s: item %d production: %w", path, itemIndex+1, err)
			}
			products = append(products, in)
		}

		list.Entries = append(list.Entries, multisell.NewEntry(ingredients, products))
	}

	return list, nil
}
