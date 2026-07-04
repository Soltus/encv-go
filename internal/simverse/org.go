package simverse

import "encoding/binary"

type OrgType uint8

const (
	OrgFamily     OrgType = 0
	OrgClan       OrgType = 1
	OrgVillage    OrgType = 2
	OrgTown       OrgType = 3
	OrgKingdom    OrgType = 4
	OrgGuild      OrgType = 5
	OrgReligion   OrgType = 6
	OrgMerchant   OrgType = 7
	OrgMercenary  OrgType = 8
	OrgThieves    OrgType = 9
	OrgMageGuild  OrgType = 10
	OrgAdventurer OrgType = 11
	OrgMax                  = 12
)

type OrganizationV2 struct {
	ID         uint32
	Name       string
	OrgType    OrgType
	Level      uint16
	ParentID   uint32
	RegionID   uint32
	FoundedAt  int32
	MemberCount uint32
	Wealth     Resources
	Influence  uint32
	Stability  uint16
	Reputation int16
	LeaderID   uint64
	Tags       uint32
}

const OrgBaseSize = 96

func (o *OrganizationV2) MarshalTo(buf []byte) int {
	if len(buf) < OrgBaseSize {
		return 0
	}
	binary.LittleEndian.PutUint32(buf[0:4], o.ID)
	buf[4] = byte(o.OrgType)
	binary.LittleEndian.PutUint16(buf[5:7], o.Level)
	binary.LittleEndian.PutUint32(buf[7:11], o.ParentID)
	binary.LittleEndian.PutUint32(buf[11:15], o.RegionID)
	binary.LittleEndian.PutUint32(buf[15:19], uint32(o.FoundedAt))
	binary.LittleEndian.PutUint32(buf[19:23], o.MemberCount)

	for i := 0; i < int(ResMax); i++ {
		binary.LittleEndian.PutUint32(buf[23+i*4:23+i*4+4], o.Wealth[i])
	}

	off := 23 + int(ResMax)*4
	binary.LittleEndian.PutUint32(buf[off:off+4], o.Influence)
	off += 4
	binary.LittleEndian.PutUint16(buf[off:off+2], o.Stability)
	off += 2
	binary.LittleEndian.PutUint16(buf[off:off+2], uint16(o.Reputation))
	off += 2
	binary.LittleEndian.PutUint64(buf[off:off+8], o.LeaderID)
	off += 8
	binary.LittleEndian.PutUint32(buf[off:off+4], o.Tags)
	off += 4

	nameLen := len(o.Name)
	if nameLen > 40 {
		nameLen = 40
	}
	buf[off] = byte(nameLen)
	copy(buf[off+1:off+1+nameLen], o.Name[:nameLen])

	return off + 1 + nameLen
}

type Region struct {
	ID         uint32
	Name       string
	ParentID   uint32
	Climate    uint8
	Terrain    uint8
	Population uint32
	Resources  Resources
	NPCCount   uint32
	OrgCount   uint16
}

const RegionBaseSize = 64

type EventType uint16

const (
	EventWork          EventType = 0
	EventRest          EventType = 1
	EventEat           EventType = 2
	EventSocialize     EventType = 3
	EventTrade         EventType = 4
	EventFight         EventType = 5
	EventCraft         EventType = 6
	EventLearn         EventType = 7
	EventTravel        EventType = 8
	EventRestSleep     EventType = 9
	EventTypeMax                 = 10
)

type SimEvent struct {
	ID          uint64
	Type        EventType
	ActorID     uint64
	TargetID    uint64
	ScheduledAt int32
	Duration    uint16
	Priority    uint8
	RegionID    uint32
	Result      int16
	Data        [16]byte
}

const SimEventSize = 48

func (e *SimEvent) MarshalTo(buf []byte) int {
	if len(buf) < SimEventSize {
		return 0
	}
	binary.LittleEndian.PutUint64(buf[0:8], e.ID)
	binary.LittleEndian.PutUint16(buf[8:10], uint16(e.Type))
	binary.LittleEndian.PutUint64(buf[10:18], e.ActorID)
	binary.LittleEndian.PutUint64(buf[18:26], e.TargetID)
	binary.LittleEndian.PutUint32(buf[26:30], uint32(e.ScheduledAt))
	binary.LittleEndian.PutUint16(buf[30:32], e.Duration)
	buf[32] = e.Priority
	binary.LittleEndian.PutUint32(buf[33:37], e.RegionID)
	binary.LittleEndian.PutUint16(buf[37:39], uint16(e.Result))
	copy(buf[39:47], e.Data[:])
	return SimEventSize
}

func (e *SimEvent) Unmarshal(buf []byte) bool {
	if len(buf) < SimEventSize {
		return false
	}
	e.ID = binary.LittleEndian.Uint64(buf[0:8])
	e.Type = EventType(binary.LittleEndian.Uint16(buf[8:10]))
	e.ActorID = binary.LittleEndian.Uint64(buf[10:18])
	e.TargetID = binary.LittleEndian.Uint64(buf[18:26])
	e.ScheduledAt = int32(binary.LittleEndian.Uint32(buf[26:30]))
	e.Duration = binary.LittleEndian.Uint16(buf[30:32])
	e.Priority = buf[32]
	e.RegionID = binary.LittleEndian.Uint32(buf[33:37])
	e.Result = int16(binary.LittleEndian.Uint16(buf[37:39]))
	copy(e.Data[:], buf[39:47])
	return true
}
