package simverse

import "encoding/binary"

type SpeciesType uint8

const (
	SpeciesHuman     SpeciesType = 0
	SpeciesElf       SpeciesType = 1
	SpeciesDwarf     SpeciesType = 2
	SpeciesOrc       SpeciesType = 3
	SpeciesBeastman  SpeciesType = 4
	SpeciesDragonkin SpeciesType = 5
	SpeciesFey       SpeciesType = 6
	SpeciesUndead    SpeciesType = 7
	SpeciesMax                   = 8
)

var SpeciesNames = [SpeciesMax]string{
	"Human", "Elf", "Dwarf", "Orc",
	"Beastman", "Dragonkin", "Fey", "Undead",
}

func (s SpeciesType) String() string {
	if int(s) < int(SpeciesMax) {
		return SpeciesNames[s]
	}
	return "Unknown"
}

type ProfessionType uint16

const (
	ProfNone            ProfessionType = 0
	ProfFarmer          ProfessionType = 1
	ProfMiner           ProfessionType = 2
	ProfWoodcutter      ProfessionType = 3
	ProfBlacksmith      ProfessionType = 4
	ProfTailor          ProfessionType = 5
	ProfMerchant        ProfessionType = 6
	ProfWarrior         ProfessionType = 7
	ProfMage            ProfessionType = 8
	ProfPriest          ProfessionType = 9
	ProfRogue           ProfessionType = 10
	ProfRanger          ProfessionType = 11
	ProfAlchemist       ProfessionType = 12
	ProfScribe          ProfessionType = 13
	ProfNoble           ProfessionType = 14
	ProfSlave           ProfessionType = 15
	ProfBeggar          ProfessionType = 16
	ProfEntertainer     ProfessionType = 17
	ProfCook            ProfessionType = 18
	ProfHealer          ProfessionType = 19
	ProfMax                            = 20
)

var ProfessionNames = [ProfMax]string{
	"None", "Farmer", "Miner", "Woodcutter", "Blacksmith",
	"Tailor", "Merchant", "Warrior", "Mage", "Priest",
	"Rogue", "Ranger", "Alchemist", "Scribe", "Noble",
	"Slave", "Beggar", "Entertainer", "Cook", "Healer",
}

func (p ProfessionType) String() string {
	if int(p) < int(ProfMax) {
		return ProfessionNames[p]
	}
	return "Unknown"
}

type SkillType uint8

const (
	SkillStrength    SkillType = 0
	SkillAgility     SkillType = 1
	SkillIntelligence SkillType = 2
	SkillCharisma    SkillType = 3
	SkillPerception  SkillType = 4
	SkillEndurance   SkillType = 5
	SkillMagic       SkillType = 6
	SkillStealth     SkillType = 7
	SkillCrafting    SkillType = 8
	SkillTrading     SkillType = 9
	SkillMax                    = 10
)

var SkillNames = [SkillMax]string{
	"Strength", "Agility", "Intelligence", "Charisma", "Perception",
	"Endurance", "Magic", "Stealth", "Crafting", "Trading",
}

func (s SkillType) String() string {
	if int(s) < int(SkillMax) {
		return SkillNames[s]
	}
	return "Unknown"
}

type Skills [SkillMax]uint8

func (s *Skills) Add(other Skills, factor uint8) {
	for i := 0; i < int(SkillMax); i++ {
		val := int(s[i]) + int(other[i])*int(factor)/255
		if val > 255 {
			val = 255
		}
		s[i] = uint8(val)
	}
}

type ResourceType uint8

const (
	ResFood        ResourceType = 0
	ResWood        ResourceType = 1
	ResStone       ResourceType = 2
	ResIron        ResourceType = 3
	ResGold        ResourceType = 4
	ResCloth       ResourceType = 5
	ResPotion      ResourceType = 6
	ResManaCrystal ResourceType = 7
	ResHerb        ResourceType = 8
	ResLeather     ResourceType = 9
	ResMax                      = 10
)

var ResourceNames = [ResMax]string{
	"Food", "Wood", "Stone", "Iron", "Gold",
	"Cloth", "Potion", "ManaCrystal", "Herb", "Leather",
}

func (r ResourceType) String() string {
	if int(r) < int(ResMax) {
		return ResourceNames[r]
	}
	return "Unknown"
}

type Resources [ResMax]uint32

func (r *Resources) Add(other Resources, factor int) {
	for i := 0; i < int(ResMax); i++ {
		val := int(r[i]) + int(other[i])*factor
		if val > 0xFFFFFFFF || val < 0 {
			if factor > 0 {
				r[i] = 0xFFFFFFFF
			} else {
				r[i] = 0
			}
		} else {
			r[i] = uint32(val)
		}
	}
}

func (r *Resources) CanAfford(cost Resources) bool {
	for i := 0; i < int(ResMax); i++ {
		if r[i] < cost[i] {
			return false
		}
	}
	return true
}

type RelationshipType uint8

const (
	RelStranger  RelationshipType = 0
	RelAcquaint  RelationshipType = 1
	RelFriend    RelationshipType = 2
	RelLover     RelationshipType = 3
	RelSpouse    RelationshipType = 4
	RelParent    RelationshipType = 5
	RelChild     RelationshipType = 6
	RelSibling   RelationshipType = 7
	RelMaster    RelationshipType = 8
	RelApprentice RelationshipType = 9
	RelEnemy     RelationshipType = 10
	RelRival     RelationshipType = 11
)

type Relationship struct {
	TargetID uint64
	RelType  RelationshipType
	Affinity int16
	LastMeet int32
}

type NPCV2 struct {
	ID           uint64
	Name         string
	Species      SpeciesType
	Profession   ProfessionType
	Level        uint16
	Age          uint16
	Health       uint16
	MaxHealth    uint16
	Energy       uint16
	MaxEnergy    uint16
	Mana         uint16
	MaxMana      uint16
	Mood         int8
	Satisfaction int8
	Skills       Skills
	Inventory    Resources
	Bank         Resources
	OrgID        uint32
	RegionID     uint32
	HomeRegionID uint32
	LastActive   int32
	BornAt       int32
	Experience   uint32
	WealthTier   uint8
	SocialTier   uint8
	FactionRep   [8]int16
}

const NPCV2BaseSize = 128

func (n *NPCV2) MarshalTo(buf []byte) int {
	if len(buf) < NPCV2BaseSize {
		return 0
	}

	binary.LittleEndian.PutUint64(buf[0:8], n.ID)
	buf[8] = byte(n.Species)
	buf[9] = byte(n.Profession & 0xFF)
	buf[10] = byte(n.Profession >> 8)
	binary.LittleEndian.PutUint16(buf[11:13], n.Level)
	binary.LittleEndian.PutUint16(buf[13:15], n.Age)
	binary.LittleEndian.PutUint16(buf[15:17], n.Health)
	binary.LittleEndian.PutUint16(buf[17:19], n.MaxHealth)
	binary.LittleEndian.PutUint16(buf[19:21], n.Energy)
	binary.LittleEndian.PutUint16(buf[21:23], n.MaxEnergy)
	binary.LittleEndian.PutUint16(buf[23:25], n.Mana)
	binary.LittleEndian.PutUint16(buf[25:27], n.MaxMana)
	buf[27] = byte(n.Mood)
	buf[28] = byte(n.Satisfaction)

	for i := 0; i < int(SkillMax); i++ {
		buf[29+i] = n.Skills[i]
	}

	invOff := 29 + int(SkillMax)
	for i := 0; i < int(ResMax); i++ {
		binary.LittleEndian.PutUint32(buf[invOff+i*4:invOff+i*4+4], n.Inventory[i])
	}

	bankOff := invOff + int(ResMax)*4
	for i := 0; i < int(ResMax); i++ {
		binary.LittleEndian.PutUint32(buf[bankOff+i*4:bankOff+i*4+4], n.Bank[i])
	}

	orgOff := bankOff + int(ResMax)*4
	binary.LittleEndian.PutUint32(buf[orgOff:orgOff+4], n.OrgID)
	binary.LittleEndian.PutUint32(buf[orgOff+4:orgOff+8], n.RegionID)
	binary.LittleEndian.PutUint32(buf[orgOff+8:orgOff+12], n.HomeRegionID)
	binary.LittleEndian.PutUint32(buf[orgOff+12:orgOff+16], uint32(n.LastActive))
	binary.LittleEndian.PutUint32(buf[orgOff+16:orgOff+20], uint32(n.BornAt))
	binary.LittleEndian.PutUint32(buf[orgOff+20:orgOff+24], n.Experience)
	buf[orgOff+24] = n.WealthTier
	buf[orgOff+25] = n.SocialTier

	repOff := orgOff + 26
	for i := 0; i < 8; i++ {
		binary.LittleEndian.PutUint16(buf[repOff+i*2:repOff+i*2+2], uint16(n.FactionRep[i]))
	}

	nameStart := repOff + 16
	nameLen := len(n.Name)
	if nameLen > 48 {
		nameLen = 48
	}
	buf[nameStart] = byte(nameLen)
	copy(buf[nameStart+1:nameStart+1+nameLen], n.Name[:nameLen])

	return nameStart + 1 + nameLen
}

func (n *NPCV2) Unmarshal(buf []byte) bool {
	if len(buf) < 120 {
		return false
	}

	n.ID = binary.LittleEndian.Uint64(buf[0:8])
	n.Species = SpeciesType(buf[8])
	n.Profession = ProfessionType(uint16(buf[9]) | uint16(buf[10])<<8)
	n.Level = binary.LittleEndian.Uint16(buf[11:13])
	n.Age = binary.LittleEndian.Uint16(buf[13:15])
	n.Health = binary.LittleEndian.Uint16(buf[15:17])
	n.MaxHealth = binary.LittleEndian.Uint16(buf[17:19])
	n.Energy = binary.LittleEndian.Uint16(buf[19:21])
	n.MaxEnergy = binary.LittleEndian.Uint16(buf[21:23])
	n.Mana = binary.LittleEndian.Uint16(buf[23:25])
	n.MaxMana = binary.LittleEndian.Uint16(buf[25:27])
	n.Mood = int8(buf[27])
	n.Satisfaction = int8(buf[28])

	for i := 0; i < int(SkillMax); i++ {
		n.Skills[i] = buf[29+i]
	}

	invOff := 29 + int(SkillMax)
	for i := 0; i < int(ResMax); i++ {
		n.Inventory[i] = binary.LittleEndian.Uint32(buf[invOff+i*4 : invOff+i*4+4])
	}

	bankOff := invOff + int(ResMax)*4
	for i := 0; i < int(ResMax); i++ {
		n.Bank[i] = binary.LittleEndian.Uint32(buf[bankOff+i*4 : bankOff+i*4+4])
	}

	orgOff := bankOff + int(ResMax)*4
	n.OrgID = binary.LittleEndian.Uint32(buf[orgOff : orgOff+4])
	n.RegionID = binary.LittleEndian.Uint32(buf[orgOff+4 : orgOff+8])
	n.HomeRegionID = binary.LittleEndian.Uint32(buf[orgOff+8 : orgOff+12])
	n.LastActive = int32(binary.LittleEndian.Uint32(buf[orgOff+12 : orgOff+16]))
	n.BornAt = int32(binary.LittleEndian.Uint32(buf[orgOff+16 : orgOff+20]))
	n.Experience = binary.LittleEndian.Uint32(buf[orgOff+20 : orgOff+24])
	n.WealthTier = buf[orgOff+24]
	n.SocialTier = buf[orgOff+25]

	repOff := orgOff + 26
	for i := 0; i < 8; i++ {
		n.FactionRep[i] = int16(binary.LittleEndian.Uint16(buf[repOff+i*2 : repOff+i*2+2]))
	}

	nameStart := repOff + 16
	if nameStart >= len(buf) {
		return false
	}
	nameLen := int(buf[nameStart])
	if nameStart+1+nameLen > len(buf) || nameLen > 48 {
		return false
	}
	n.Name = string(buf[nameStart+1 : nameStart+1+nameLen])
	return true
}
