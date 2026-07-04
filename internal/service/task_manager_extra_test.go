package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCreateWithExtras_PreservesAllFields(t *testing.T) {
	mb := new(MockBroadcaster)
	mb.On("Broadcast", "task:created", mock.Anything).Return()
	tm := newTestTaskManager(mb)

	extras := map[string]string{
		"plugin_password": "test123",
		"custom_field":    "value",
	}
	task := tm.CreateWithExtras("encrypt", "/test/file.mp4", "", "override-pw", "secondary-pw", 4, "", extras)

	require.NotNil(t, task)
	assert.Equal(t, "encrypt", task.Type)
	assert.Equal(t, "/test/file.mp4", task.SourcePath)
	assert.Equal(t, "override-pw", task.Password, "primary password should be preserved")
	assert.Equal(t, "secondary-pw", task.SecondaryPassword, "secondary password should be preserved")
	assert.Equal(t, "test123", task.ExtraFields["plugin_password"], "extra field plugin_password should be preserved")
	assert.Equal(t, "value", task.ExtraFields["custom_field"], "extra field custom_field should be preserved")
	assert.Equal(t, 4, task.ContainerVersion, "version should be preserved")
}

func TestCreateWithExtras_NilExtras(t *testing.T) {
	mb := new(MockBroadcaster)
	mb.On("Broadcast", "task:created", mock.Anything).Return()
	tm := newTestTaskManager(mb)

	task := tm.CreateWithExtras("decrypt", "/test/file.bin", "", "", "", 0, "", nil)

	require.NotNil(t, task)
	assert.Nil(t, task.ExtraFields, "nil extras should remain nil for backward compat")
	assert.Empty(t, task.SecondaryPassword, "empty secondary password should be empty string")
}

func TestCreateWithExtras_CompatWithCreate(t *testing.T) {
	mb := new(MockBroadcaster)
	mb.On("Broadcast", "task:created", mock.Anything).Return()
	tm := newTestTaskManager(mb)

	taskViaCreate := tm.Create("encrypt", "/test/file.mp4", "", "pw", 3, "")
	mb.On("Broadcast", "task:created", mock.Anything).Return()
	taskViaExtras := tm.CreateWithExtras("encrypt", "/test/file2.mp4", "", "pw", "", 3, "", nil)

	assert.Equal(t, taskViaCreate.Password, taskViaExtras.Password, "password should match")
	assert.Equal(t, taskViaCreate.ContainerVersion, taskViaExtras.ContainerVersion, "version should match")
	assert.Nil(t, taskViaCreate.ExtraFields, "old Create() should have nil ExtraFields")
	assert.Empty(t, taskViaExtras.SecondaryPassword, "CreateWithExtras with empty secondary should have empty string")
}
