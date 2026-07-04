package simverse

import (
	"encoding/binary"
	"math"
)

const NPCSize = 96

type NPC struct {
	ID         uint64
	Name       string
	Age        uint16
	Health     uint16
	Energy     uint16
	Strength   uint8
	Intellect  uint8
	Charisma   uint8
	Agility    uint8
	Luck       uint8
	Status     uint16
	OrgID      uint32
	RegionID   uint32
	LastActive int64
	BornAt     int32
}

func (n *NPC) MarshalTo(buf []byte) int {
	if len(buf) < NPCSize {
		return 0
	}
	binary.LittleEndian.PutUint64(buf[0:8], n.ID)
	binary.LittleEndian.PutUint16(buf[8:10], n.Age)
	binary.LittleEndian.PutUint16(buf[10:12], n.Health)
	binary.LittleEndian.PutUint16(buf[12:14], n.Energy)

	buf[14] = n.Strength
	buf[15] = n.Intellect
	buf[16] = n.Charisma
	buf[17] = n.Agility
	buf[18] = n.Luck

	binary.LittleEndian.PutUint16(buf[19:21], n.Status)
	binary.LittleEndian.PutUint32(buf[21:25], n.OrgID)
	binary.LittleEndian.PutUint32(buf[25:29], n.RegionID)
	binary.LittleEndian.PutUint64(buf[29:37], math.Float64bits(float64(n.LastActive)))
	binary.LittleEndian.PutUint32(buf[37:41], uint32(n.BornAt))

	nameLen := len(n.Name)
	if nameLen > 50 {
		nameLen = 50
	}
	buf[41] = byte(nameLen)
	copy(buf[42:42+nameLen], n.Name[:nameLen])

	return 42 + nameLen
}

func (n *NPC) Unmarshal(buf []byte) bool {
	if len(buf) < 42 {
		return false
	}
	n.ID = binary.LittleEndian.Uint64(buf[0:8])
	n.Age = binary.LittleEndian.Uint16(buf[8:10])
	n.Health = binary.LittleEndian.Uint16(buf[10:12])
	n.Energy = binary.LittleEndian.Uint16(buf[12:14])

	n.Strength = buf[14]
	n.Intellect = buf[15]
	n.Charisma = buf[16]
	n.Agility = buf[17]
	n.Luck = buf[18]

	n.Status = binary.LittleEndian.Uint16(buf[19:21])
	n.OrgID = binary.LittleEndian.Uint32(buf[21:25])
	n.RegionID = binary.LittleEndian.Uint32(buf[25:29])
	n.LastActive = int64(math.Float64frombits(binary.LittleEndian.Uint64(buf[29:37])))
	n.BornAt = int32(binary.LittleEndian.Uint32(buf[37:41]))

	nameLen := int(buf[41])
	if 42+nameLen > len(buf) || nameLen > 50 {
		return false
	}
	n.Name = string(buf[42 : 42+nameLen])
	return true
}
