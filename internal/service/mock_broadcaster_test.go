package service

import (
	"sync"

	"github.com/stretchr/testify/mock"
)

type MockBroadcaster struct {
	mock.Mock
	mu     sync.Mutex
	calls_ []BroadcastCall
}

type BroadcastCall struct {
	MsgType string
	Data    interface{}
}

func (m *MockBroadcaster) Broadcast(msgType string, data interface{}) {
	m.mu.Lock()
	m.calls_ = append(m.calls_, BroadcastCall{MsgType: msgType, Data: data})
	m.mu.Unlock()
	m.Called(msgType, data)
}

func (m *MockBroadcaster) GetCalls() []BroadcastCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]BroadcastCall, len(m.calls_))
	copy(result, m.calls_)
	return result
}

func (m *MockBroadcaster) FindCalls(msgType string) []BroadcastCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []BroadcastCall
	for _, c := range m.calls_ {
		if c.MsgType == msgType {
			result = append(result, c)
		}
	}
	return result
}
