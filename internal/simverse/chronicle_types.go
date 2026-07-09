package simverse

// ============================================================
// 编年史系统 — 五级历史记录（个人/家庭/组织/区域/世界）
//
// 设计原则：
//   - 事件通用结构，固定 32 字节主数据
//   - 按时间序存储，支持按时间/实体/类型/重要性多维查询
//   - 重要性决定留存时间（自动降级/删除）
//   - 因果链：每个事件可以有 0-3 个前置原因
// ============================================================

type ChronicleLevel uint8

const (
	ChronLevelPersonal  ChronicleLevel = 0
	ChronLevelFamily    ChronicleLevel = 1
	ChronLevelOrg       ChronicleLevel = 2
	ChronLevelRegion    ChronicleLevel = 3
	ChronLevelWorld     ChronicleLevel = 4
	ChronLevelMax                      = 5
)

var ChronicleLevelNames = [ChronLevelMax]string{
	"Personal",
	"Family",
	"Organization",
	"Region",
	"World",
}

var ChronicleLevelCNNames = [ChronLevelMax]string{
	"个人",
	"家庭",
	"组织",
	"区域",
	"世界",
}

func (l ChronicleLevel) String() string {
	if int(l) < int(ChronLevelMax) {
		return ChronicleLevelNames[l]
	}
	return "Unknown"
}

func (l ChronicleLevel) CN() string {
	if int(l) < int(ChronLevelMax) {
		return ChronicleLevelCNNames[l]
	}
	return "未知"
}

type ChronicleImportance uint8

const (
	ImpTrivial   ChronicleImportance = 0
	ImpMinor     ChronicleImportance = 1
	ImpModerate  ChronicleImportance = 2
	ImpMajor     ChronicleImportance = 3
	ImpGreat     ChronicleImportance = 4
	ImpEpic      ChronicleImportance = 5
	ImpMax                           = 6
)

var ChronicleImpNames = [ImpMax]string{
	"Trivial",
	"Minor",
	"Moderate",
	"Major",
	"Great",
	"Epic",
}

var ChronicleImpCNNames = [ImpMax]string{
	"琐碎",
	"次要",
	"中等",
	"重要",
	"重大",
	"史诗",
}

func (i ChronicleImportance) String() string {
	if int(i) < int(ImpMax) {
		return ChronicleImpNames[i]
	}
	return "Unknown"
}

func (i ChronicleImportance) CN() string {
	if int(i) < int(ImpMax) {
		return ChronicleImpCNNames[i]
	}
	return "未知"
}

type ChronicleEventType uint16

const (
	ChronEventPersonalStart  ChronicleEventType = 0
	ChronEventBirth          ChronicleEventType = 0
	ChronEventChildhood      ChronicleEventType = 1
	ChronEventAdolescence    ChronicleEventType = 2
	ChronEventComingOfAge    ChronicleEventType = 3
	ChronEventFirstJob       ChronicleEventType = 4
	ChronEventPromotion      ChronicleEventType = 5
	ChronEventCareerChange   ChronicleEventType = 6
	ChronEventMarriage       ChronicleEventType = 7
	ChronEventDivorce        ChronicleEventType = 8
	ChronEventChildBirth     ChronicleEventType = 9
	ChronEventRetirement     ChronicleEventType = 10
	ChronEventAging          ChronicleEventType = 11
	ChronEventIllness        ChronicleEventType = 12
	ChronEventDeath          ChronicleEventType = 13
	ChronEventPersonalFight  ChronicleEventType = 14
	ChronEventPersonalEnd    ChronicleEventType = 99

	ChronEventFamilyStart   ChronicleEventType = 100
	ChronEventFamilyFounded ChronicleEventType = 100
	ChronEventFamilyMarriage ChronicleEventType = 101
	ChronEventFamilyHeir    ChronicleEventType = 102
	ChronEventFamilySplit   ChronicleEventType = 103
	ChronEventFamilyRise    ChronicleEventType = 104
	ChronEventFamilyFall    ChronicleEventType = 105
	ChronEventFamilyEnd     ChronicleEventType = 199

	ChronEventOrgStart      ChronicleEventType = 200
	ChronEventOrgFounded    ChronicleEventType = 200
	ChronEventOrgExpand     ChronicleEventType = 201
	ChronEventOrgShrink     ChronicleEventType = 202
	ChronEventOrgSplit      ChronicleEventType = 203
	ChronEventOrgMerge      ChronicleEventType = 204
	ChronEventOrgDissolve   ChronicleEventType = 205
	ChronEventOrgLeaderChange ChronicleEventType = 206
	ChronEventOrgVictory    ChronicleEventType = 207
	ChronEventOrgDefeat     ChronicleEventType = 208
	ChronEventOrgWarDeclare ChronicleEventType = 209
	ChronEventOrgWarEnd     ChronicleEventType = 210
	ChronEventOrgAlliance   ChronicleEventType = 211
	ChronEventOrgBetrayal   ChronicleEventType = 212
	ChronEventOrgEnd        ChronicleEventType = 299

	ChronEventRegionStart   ChronicleEventType = 300
	ChronEventCityFounded   ChronicleEventType = 300
	ChronEventCityProsper   ChronicleEventType = 301
	ChronEventCityDecline   ChronicleEventType = 302
	ChronEventCityDestroyed ChronicleEventType = 303
	ChronEventDisaster      ChronicleEventType = 304
	ChronEventMigration     ChronicleEventType = 305
	ChronEventEconomyBoom   ChronicleEventType = 306
	ChronEventEconomyRecess ChronicleEventType = 307
	ChronEventCultureRise   ChronicleEventType = 308
	ChronEventTechBreakthrough ChronicleEventType = 309
	ChronEventRegionEnd     ChronicleEventType = 399

	ChronEventWorldStart    ChronicleEventType = 400
	ChronEventEraStart      ChronicleEventType = 400
	ChronEventEraEnd        ChronicleEventType = 401
	ChronEventWorldWar      ChronicleEventType = 402
	ChronEventWorldPeace    ChronicleEventType = 403
	ChronEventGreatDisaster ChronicleEventType = 404
	ChronEventGreatMiracle  ChronicleEventType = 405
	ChronEventCivilizationRise ChronicleEventType = 406
	ChronEventCivilizationFall ChronicleEventType = 407
	ChronEventDeityDescend  ChronicleEventType = 408
	ChronEventMagicTide     ChronicleEventType = 409
	ChronEventWorldEnd      ChronicleEventType = 499

	ChronEventMax           ChronicleEventType = 500
)

func (t ChronicleEventType) Level() ChronicleLevel {
	switch {
	case t < ChronEventPersonalEnd:
		return ChronLevelPersonal
	case t < ChronEventFamilyEnd:
		return ChronLevelFamily
	case t < ChronEventOrgEnd:
		return ChronLevelOrg
	case t < ChronEventRegionEnd:
		return ChronLevelRegion
	case t < ChronEventWorldEnd:
		return ChronLevelWorld
	default:
		return ChronLevelWorld
	}
}

func (t ChronicleEventType) String() string {
	switch t {
	case ChronEventBirth:
		return "Birth"
	case ChronEventChildhood:
		return "Childhood"
	case ChronEventAdolescence:
		return "Adolescence"
	case ChronEventComingOfAge:
		return "ComingOfAge"
	case ChronEventFirstJob:
		return "FirstJob"
	case ChronEventPromotion:
		return "Promotion"
	case ChronEventCareerChange:
		return "CareerChange"
	case ChronEventMarriage:
		return "Marriage"
	case ChronEventDivorce:
		return "Divorce"
	case ChronEventChildBirth:
		return "ChildBirth"
	case ChronEventRetirement:
		return "Retirement"
	case ChronEventAging:
		return "Aging"
	case ChronEventIllness:
		return "Illness"
	case ChronEventDeath:
		return "Death"
	case ChronEventPersonalFight:
		return "Fight"
	case ChronEventFamilyFounded:
		return "FamilyFounded"
	case ChronEventFamilyMarriage:
		return "FamilyMarriage"
	case ChronEventFamilyHeir:
		return "FamilyHeir"
	case ChronEventFamilySplit:
		return "FamilySplit"
	case ChronEventFamilyRise:
		return "FamilyRise"
	case ChronEventFamilyFall:
		return "FamilyFall"
	case ChronEventOrgFounded:
		return "OrgFounded"
	case ChronEventOrgExpand:
		return "OrgExpand"
	case ChronEventOrgShrink:
		return "OrgShrink"
	case ChronEventOrgSplit:
		return "OrgSplit"
	case ChronEventOrgMerge:
		return "OrgMerge"
	case ChronEventOrgDissolve:
		return "OrgDissolve"
	case ChronEventOrgLeaderChange:
		return "OrgLeaderChange"
	case ChronEventOrgVictory:
		return "OrgVictory"
	case ChronEventOrgDefeat:
		return "OrgDefeat"
	case ChronEventOrgWarDeclare:
		return "WarDeclare"
	case ChronEventOrgWarEnd:
		return "WarEnd"
	case ChronEventOrgAlliance:
		return "Alliance"
	case ChronEventOrgBetrayal:
		return "Betrayal"
	case ChronEventCityFounded:
		return "CityFounded"
	case ChronEventCityProsper:
		return "CityProsper"
	case ChronEventCityDecline:
		return "CityDecline"
	case ChronEventCityDestroyed:
		return "CityDestroyed"
	case ChronEventDisaster:
		return "Disaster"
	case ChronEventMigration:
		return "Migration"
	case ChronEventEconomyBoom:
		return "EconomyBoom"
	case ChronEventEconomyRecess:
		return "EconomyRecess"
	case ChronEventCultureRise:
		return "CultureRise"
	case ChronEventTechBreakthrough:
		return "TechBreakthrough"
	case ChronEventEraStart:
		return "EraStart"
	case ChronEventEraEnd:
		return "EraEnd"
	case ChronEventWorldWar:
		return "WorldWar"
	case ChronEventWorldPeace:
		return "WorldPeace"
	case ChronEventGreatDisaster:
		return "GreatDisaster"
	case ChronEventGreatMiracle:
		return "GreatMiracle"
	case ChronEventCivilizationRise:
		return "CivilizationRise"
	case ChronEventCivilizationFall:
		return "CivilizationFall"
	case ChronEventDeityDescend:
		return "DeityDescend"
	case ChronEventMagicTide:
		return "MagicTide"
	default:
		return "Unknown"
	}
}

func (t ChronicleEventType) CN() string {
	switch t {
	case ChronEventBirth:
		return "出生"
	case ChronEventChildhood:
		return "童年"
	case ChronEventAdolescence:
		return "青春期"
	case ChronEventComingOfAge:
		return "成年"
	case ChronEventFirstJob:
		return "第一份工作"
	case ChronEventPromotion:
		return "晋升"
	case ChronEventCareerChange:
		return "转职"
	case ChronEventMarriage:
		return "结婚"
	case ChronEventDivorce:
		return "离婚"
	case ChronEventChildBirth:
		return "生育"
	case ChronEventRetirement:
		return "退休"
	case ChronEventAging:
		return "衰老"
	case ChronEventIllness:
		return "疾病"
	case ChronEventDeath:
		return "死亡"
	case ChronEventFamilyFounded:
		return "家族创立"
	case ChronEventFamilyMarriage:
		return "家族联姻"
	case ChronEventFamilyHeir:
		return "继承人诞生"
	case ChronEventFamilySplit:
		return "分家"
	case ChronEventFamilyRise:
		return "家族崛起"
	case ChronEventFamilyFall:
		return "家族衰落"
	case ChronEventOrgFounded:
		return "组织创立"
	case ChronEventOrgExpand:
		return "组织扩张"
	case ChronEventOrgShrink:
		return "组织收缩"
	case ChronEventOrgSplit:
		return "组织分裂"
	case ChronEventOrgMerge:
		return "组织合并"
	case ChronEventOrgDissolve:
		return "组织解散"
	case ChronEventOrgLeaderChange:
		return "领袖更迭"
	case ChronEventOrgVictory:
		return "重大胜利"
	case ChronEventOrgDefeat:
		return "重大失败"
	case ChronEventOrgWarDeclare:
		return "宣战"
	case ChronEventOrgWarEnd:
		return "战争结束"
	case ChronEventOrgAlliance:
		return "结盟"
	case ChronEventOrgBetrayal:
		return "背盟"
	case ChronEventCityFounded:
		return "城市建立"
	case ChronEventCityProsper:
		return "城市繁荣"
	case ChronEventCityDecline:
		return "城市衰落"
	case ChronEventCityDestroyed:
		return "城市毁灭"
	case ChronEventDisaster:
		return "自然灾害"
	case ChronEventMigration:
		return "人口迁徙"
	case ChronEventEconomyBoom:
		return "经济繁荣"
	case ChronEventEconomyRecess:
		return "经济衰退"
	case ChronEventCultureRise:
		return "文化兴起"
	case ChronEventTechBreakthrough:
		return "技术突破"
	case ChronEventEraStart:
		return "时代开始"
	case ChronEventEraEnd:
		return "时代结束"
	case ChronEventWorldWar:
		return "世界大战"
	case ChronEventWorldPeace:
		return "世界和平"
	case ChronEventGreatDisaster:
		return "大灾难"
	case ChronEventGreatMiracle:
		return "大奇迹"
	case ChronEventCivilizationRise:
		return "文明兴起"
	case ChronEventCivilizationFall:
		return "文明衰落"
	case ChronEventDeityDescend:
		return "神之降临"
	case ChronEventMagicTide:
		return "魔法潮汐"
	default:
		return "未知"
	}
}

func DefaultImportanceForType(t ChronicleEventType) ChronicleImportance {
	switch t {
	case ChronEventBirth, ChronEventDeath, ChronEventMarriage, ChronEventChildBirth:
		return ImpModerate
	case ChronEventComingOfAge, ChronEventFirstJob, ChronEventPromotion, ChronEventCareerChange:
		return ImpMinor
	case ChronEventRetirement, ChronEventDivorce:
		return ImpModerate
	case ChronEventIllness, ChronEventAging, ChronEventChildhood, ChronEventAdolescence:
		return ImpTrivial
	case ChronEventFamilyFounded, ChronEventFamilyRise, ChronEventFamilyFall:
		return ImpMajor
	case ChronEventOrgFounded, ChronEventOrgDissolve, ChronEventOrgSplit, ChronEventOrgMerge:
		return ImpMajor
	case ChronEventOrgWarDeclare, ChronEventOrgWarEnd, ChronEventOrgVictory, ChronEventOrgDefeat:
		return ImpGreat
	case ChronEventCityFounded, ChronEventCityDestroyed:
		return ImpGreat
	case ChronEventDisaster:
		return ImpMajor
	case ChronEventEraStart, ChronEventEraEnd:
		return ImpEpic
	case ChronEventWorldWar, ChronEventWorldPeace:
		return ImpGreat
	case ChronEventGreatDisaster, ChronEventGreatMiracle:
		return ImpEpic
	case ChronEventCivilizationRise, ChronEventCivilizationFall:
		return ImpGreat
	case ChronEventDeityDescend, ChronEventMagicTide:
		return ImpEpic
	default:
		return ImpMinor
	}
}

type ChronicleEvent struct {
	ID           uint64
	Tick         uint32
	Level        ChronicleLevel
	Type         ChronicleEventType
	Importance   ChronicleImportance
	EntityID     uint64
	TargetID     uint64
	DataTag      uint16
	Cause1ID     uint64
	Cause2ID     uint64
	Cause3ID     uint64
}

const ChronicleEventSize = 32 + 24

func ChronicleEventSizeEstimate() float64 {
	return float64(ChronicleEventSize)
}
