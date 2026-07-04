package simverse

type Event struct {
	ID          uint64
	Type        uint8
	TargetID    uint64
	ScheduledAt int64
	Priority    uint8
	Data        [32]byte
}

type EventScheduler struct {
	events []Event
	size   int
}

func NewEventScheduler() *EventScheduler {
	return &EventScheduler{
		events: make([]Event, 0, 1024),
	}
}

func (s *EventScheduler) Schedule(e Event) {
	s.events = append(s.events, e)
	s.size++
}

func (s *EventScheduler) Len() int {
	return s.size
}

func (s *EventScheduler) Tick(now int64) []Event {
	var ready []Event
	j := 0
	for i := 0; i < s.size; i++ {
		if s.events[i].ScheduledAt <= now {
			ready = append(ready, s.events[i])
		} else {
			s.events[j] = s.events[i]
			j++
		}
	}
	s.size = j
	s.events = s.events[:j]
	return ready
}
