package xml

import (
	"encoding/xml"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/multisell"
)

var multisellFilenameXML = regexp.MustCompile(".xml")

type multiSellFile struct {
	Attrs []xml.Attr        `xml:",any,attr"`
	NPCs  []multiSellNPCSet `xml:"npcs"`
	Items []multiSellItem   `xml:"item"`
}

type multiSellNPCSet struct {
	IDs []npcIDText `xml:"npc"`
}

// npcIDText is an <npc> element whose text is a plain base-10 int32 npc id.
// Empty, padded and non-numeric text fails the load, where the decoder's own
// int conversion would read empty text as 0 and trim the padding.
type npcIDText int32

func (n *npcIDText) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var text string
	if err := d.DecodeElement(&text, &start); err != nil {
		return err
	}
	v, err := strconv.ParseInt(text, 10, 32)
	if err != nil {
		return fmt.Errorf("npc: %w", err)
	}
	*n = npcIDText(v)
	return nil
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
	id := commons.LegacyStringHash(multisellFilenameXML.ReplaceAllString(filepath.Base(path), ""))
	set := commons.StatSetFromXMLAttrs(file.Attrs)

	list := &multisell.List{
		ID:                  id,
		ApplyTaxes:          set.GetBoolDefault("applyTaxes", false),
		MaintainEnchantment: set.GetBoolDefault("maintainEnchantment", false),
		Entries:             make([]multisell.Entry, 0, len(file.Items)),
	}

	for _, npcSet := range file.NPCs {
		for _, id := range npcSet.IDs {
			list.NPCIDs = append(list.NPCIDs, int32(id))
		}
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
