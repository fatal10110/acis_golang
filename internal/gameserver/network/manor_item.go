package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	itemhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/item"
	skillhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// ManorConfig is the manor the seed and harvester items and the harvest
// read: server.properties AllowManor and RateDropManor, the seed rows and
// the manor areas.
type ManorConfig struct {
	Allowed  bool
	CropRate int
	Seeds    *manor.Table
	Areas    *manor.AreaIndex
}

func (c ManorConfig) rules() itemhandler.ManorRules {
	return itemhandler.ManorRules{Allowed: c.Allowed, Seeds: c.Seeds, Areas: c.Areas}
}

// useManorItem answers UseItem on a seed or a harvester. A seed is sown on
// the targeted monster with the seed itself carrying the cast; a harvester
// casts the harvest skill on the targeted corpse. Each refusal is answered
// by its system message alone, or by nothing where the specified handler is
// silent: a use-item request leaves no client action pending. It reports
// whether inst is a manor item.
func (l *GameClientLink) useManorItem(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance, tmpl *item.Template) bool {
	decision := itemhandler.ResolveManorItem(tmpl, live.Character, l.manor.rules(), l.skills)
	if !decision.Handled {
		return false
	}
	switch decision.Refusal {
	case itemhandler.ManorAllowed:
		var carrier *item.Instance
		if decision.Carrier {
			carrier = inst
		}
		l.castItemSkills(live, inv, carrier, []modelskill.Definition{decision.Skill}, false, false)
	case itemhandler.ManorUnavailableForSeeding:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetUnavailableForSeeding))
	case itemhandler.ManorSeedNotHere:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSeedMayNotBeSownHere))
	case itemhandler.ManorInvalidTarget:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
	case itemhandler.ManorAlreadySown:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSeedHasBeenSown))
	case itemhandler.ManorSilent:
		// A disabled manor, an item no seed row names and a missing skill
		// are refused with no packet, as specified.
	}
	return true
}

// seedCarrier is the seed a SOW cast was started with: the seed row its
// item names, looked up when the sow lands.
type seedCarrier struct {
	seeds  *manor.Table
	itemID int32
}

func (c seedCarrier) Seed() (manor.Seed, bool) { return c.seeds.Seed(c.itemID) }

// castCarrierPayload is what a carried cast hands its skill handler: the
// seed of a seed item, nothing for any other carrier.
func (l *GameClientLink) castCarrierPayload(carrier *item.Instance) any {
	if carrier == nil || l.itemTemplates == nil {
		return nil
	}
	tmpl, ok := l.itemTemplates.Get(carrier.TemplateID)
	if !ok || tmpl.EtcItem == nil || tmpl.EtcItem.Handler != itemhandler.SeedsHandler {
		return nil
	}
	return seedCarrier{seeds: l.manor.Seeds, itemID: carrier.TemplateID}
}

// manorMessageFrame is the system message a sow or harvest outcome sends
// its caster.
func manorMessageFrame(m skillhandler.ManorMessage) wire.Frame {
	id := serverpackets.SystemMessageHarvestHasFailed
	switch m {
	case skillhandler.SeedAlreadySown:
		id = serverpackets.SystemMessageSeedHasBeenSown
	case skillhandler.SeedNotSown:
		id = serverpackets.SystemMessageSeedNotSown
	case skillhandler.SeedSown:
		id = serverpackets.SystemMessageSeedSuccessfullySown
	case skillhandler.HarvestTargetNotSown:
		id = serverpackets.SystemMessageHarvestFailedSeedNotSown
	case skillhandler.HarvestNotAuthorized:
		id = serverpackets.SystemMessageNotAuthorizedToHarvest
	}
	return serverpackets.FrameSystemMessage(id)
}

// sendCropHarvested tells every member of harvester's party but harvester
// what it harvested: the crop's count when more than one.
func (l *GameClientLink) sendCropHarvested(harvester *livePlayer, m skillhandler.CropHarvested) {
	members, ok := l.partyMembers(harvester)
	if !ok {
		return
	}
	broadcastFrame(func() wire.Frame {
		name := serverpackets.TextParam(harvester.Name)
		crop := serverpackets.ItemNameParam(m.CropID)
		if m.Count > 1 {
			return serverpackets.FrameSystemMessageParams(serverpackets.SystemMessageS1HarvestedS3S2, name, crop, serverpackets.NumberParam(int32(m.Count)))
		}
		return serverpackets.FrameSystemMessageParams(serverpackets.SystemMessageS1HarvestedS2, name, crop)
	}, func(send func(frameReceiver)) {
		for _, member := range members {
			if member != harvester {
				send(member)
			}
		}
	})
}
