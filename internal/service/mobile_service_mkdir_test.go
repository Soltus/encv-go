package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateDirectory_Success(t *testing.T) {
	svc, dir := newTestMobileService(t)

	err := svc.CreateDirectory("/", "newfolder")
	require.NoError(t, err)

	fullPath := filepath.Join(dir, "newfolder")
	info, err := os.Stat(fullPath)
	require.NoError(t, err)
	assert.True(t, info.IsDir(), "应创建目录")
}

func TestCreateDirectory_InSubdirectory(t *testing.T) {
	svc, dir := newTestMobileService(t)

	os.Mkdir(filepath.Join(dir, "parent"), 0755)

	err := svc.CreateDirectory("/parent", "child")
	require.NoError(t, err)

	fullPath := filepath.Join(dir, "parent", "child")
	info, err := os.Stat(fullPath)
	require.NoError(t, err)
	assert.True(t, info.IsDir(), "应在子目录中创建目录")
}

func TestCreateDirectory_EmptyName(t *testing.T) {
	svc, _ := newTestMobileService(t)

	err := svc.CreateDirectory("/", "")
	require.Error(t, err)
	var badReq *BadRequestError
	require.ErrorAs(t, err, &badReq)
	assert.Contains(t, err.Error(), "cannot be empty")
}

func TestCreateDirectory_NameTooLong(t *testing.T) {
	svc, _ := newTestMobileService(t)

	longName := strings.Repeat("a", 256)
	err := svc.CreateDirectory("/", longName)
	require.Error(t, err)
	var badReq *BadRequestError
	require.ErrorAs(t, err, &badReq)
	assert.Contains(t, err.Error(), "too long")
}

func TestCreateDirectory_PathTraversal(t *testing.T) {
	svc, _ := newTestMobileService(t)

	err := svc.CreateDirectory("/", "..evil")
	require.Error(t, err)
	var forbidden *ForbiddenError
	require.ErrorAs(t, err, &forbidden)
}

func TestCreateDirectory_PathTraversalInParent(t *testing.T) {
	svc, _ := newTestMobileService(t)

	err := svc.CreateDirectory("../../etc", "payload")
	require.Error(t, err)
	var forbidden *ForbiddenError
	require.ErrorAs(t, err, &forbidden)
}

func TestCreateDirectory_AlreadyExists(t *testing.T) {
	svc, dir := newTestMobileService(t)

	os.Mkdir(filepath.Join(dir, "existing"), 0755)

	err := svc.CreateDirectory("/", "existing")
	require.Error(t, err)
	var badReq *BadRequestError
	require.ErrorAs(t, err, &badReq)
	assert.Contains(t, err.Error(), "already exists")
}

func TestCreateDirectory_IllegalCharacters_Slash(t *testing.T) {
	svc, _ := newTestMobileService(t)

	err := svc.CreateDirectory("/", "folder/name")
	require.Error(t, err)
	var badReq *BadRequestError
	require.ErrorAs(t, err, &badReq)
	assert.Contains(t, err.Error(), "illegal characters")
}

func TestCreateDirectory_IllegalCharacters_NullByte(t *testing.T) {
	svc, _ := newTestMobileService(t)

	err := svc.CreateDirectory("/", "folder\x00name")
	require.Error(t, err)
	var badReq *BadRequestError
	require.ErrorAs(t, err, &badReq)
	assert.Contains(t, err.Error(), "illegal characters")
}

func TestCreateDirectory_ParentPathWithDots(t *testing.T) {
	svc, dir := newTestMobileService(t)

	os.Mkdir(filepath.Join(dir, "normal"), 0755)

	err := svc.CreateDirectory("/normal", "..")
	require.Error(t, err)
	var forbidden *ForbiddenError
	require.ErrorAs(t, err, &forbidden)
}
