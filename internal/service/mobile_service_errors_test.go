package service

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWSHub_New(t *testing.T) {
	hub := NewWSHub()
	assert.NotNil(t, hub)
	assert.NotNil(t, hub.clients)
	assert.Empty(t, hub.clients)
}

func TestWSHub_Broadcast_NoClients(t *testing.T) {
	hub := NewWSHub()

	hub.Broadcast("test:type", map[string]string{"key": "value"})

	assert.Empty(t, hub.clients)
}

func TestWSMessage_Marshal(t *testing.T) {
	msg := WSMessage{
		Type: "task:created",
		Data: map[string]interface{}{"id": "123", "status": "queued"},
	}

	assert.Equal(t, "task:created", msg.Type)
	assert.NotNil(t, msg.Data)
}

func TestWSHub_ImplementsBroadcaster(t *testing.T) {
	var _ Broadcaster = NewWSHub()
}

func TestMobileServiceErrorTypes(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{"forbidden", &ForbiddenError{Err: assert.AnError}, assert.AnError.Error()},
		{"not found", &NotFoundError{Err: assert.AnError}, assert.AnError.Error()},
		{"bad request", &BadRequestError{Err: assert.AnError}, assert.AnError.Error()},
		{"permission", &PermissionError{Err: assert.AnError}, assert.AnError.Error()},
		{"unsupported media", &UnsupportedMediaTypeError{Err: assert.AnError}, assert.AnError.Error()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestIsValidUTF8(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"ascii", "hello world", true},
		{"utf8 chinese", "你好世界", true},
		{"utf8 emoji", "🎉🚀", true},
		{"mixed", "Hello 你好 🌍", true},
		{"empty", "", true},
		{"invalid continuation", string([]byte{0xc0, 0x00}), false},
		{"invalid start byte", string([]byte{0xfe}), false},
		{"truncated 2-byte", string([]byte{0xc2}), false},
		{"truncated 3-byte", string([]byte{0xe4, 0xb8}), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidUTF8([]byte(tt.input))
			assert.Equal(t, tt.valid, result)
		})
	}
}

func TestIsPermissionError(t *testing.T) {
	err := &os.PathError{Err: syscall.EACCES}
	assert.True(t, isPermissionError(err))
	assert.False(t, isPermissionError(assert.AnError))
}
