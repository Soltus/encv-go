package testutil

import (
	"fmt"

	containerhandle "github.com/Soltus/encv-go/internal/v2/container/handle"
	"github.com/Soltus/encv-go/internal/v2/types"
)

// MockContainerHandle 实现 containerhandle.ContainerHandle 接口
// 用于测试中注入可控的容器数据，避免依赖真实文件
type MockContainerHandle struct {
	VersionVal          int
	HeaderSizeVal       int64
	ContainerTypeVal    uint16
	IsSeekableVal       bool
	ContainerIDVal      string
	OriginalDurationVal float64
	ManifestVal         *types.Manifest
	ManifestV4Val       *types.Manifest_v4
	HeaderV2Val         *types.EnvelopeHeader_v2
	HeaderV3Val         *types.EnvelopeHeaderV3
	HeaderV4Val         *types.EnvelopeHeaderV4
	FooterV2Val         *types.EnvelopeFooter_v2
	FooterV4Val         *types.EnvelopeFooterV4
	SourceVal           containerhandle.ContainerSource
	CloseErr            error
}

func (m *MockContainerHandle) Version() int                            { return m.VersionVal }
func (m *MockContainerHandle) HeaderSize() int64                       { return m.HeaderSizeVal }
func (m *MockContainerHandle) ContainerType() uint16                   { return m.ContainerTypeVal }
func (m *MockContainerHandle) IsSeekable() bool                        { return m.IsSeekableVal }
func (m *MockContainerHandle) ContainerID() string                     { return m.ContainerIDVal }
func (m *MockContainerHandle) OriginalDuration() float64               { return m.OriginalDurationVal }
func (m *MockContainerHandle) Manifest() *types.Manifest               { return m.ManifestVal }
func (m *MockContainerHandle) ManifestV4() *types.Manifest_v4          { return m.ManifestV4Val }
func (m *MockContainerHandle) HeaderV2() *types.EnvelopeHeader_v2      { return m.HeaderV2Val }
func (m *MockContainerHandle) HeaderV3() *types.EnvelopeHeaderV3       { return m.HeaderV3Val }
func (m *MockContainerHandle) HeaderV4() *types.EnvelopeHeaderV4       { return m.HeaderV4Val }
func (m *MockContainerHandle) FooterV2() *types.EnvelopeFooter_v2      { return m.FooterV2Val }
func (m *MockContainerHandle) FooterV4() *types.EnvelopeFooterV4       { return m.FooterV4Val }
func (m *MockContainerHandle) Source() containerhandle.ContainerSource { return m.SourceVal }
func (m *MockContainerHandle) Close() error                            { return m.CloseErr }

// NewMockV4Handle 创建一个模拟 V4 容器句柄（常用快捷方法）
func NewMockV4Handle(containerType uint16, isSeekable bool, segmentCount int, manifestSize uint32) *MockContainerHandle {
	h := &MockContainerHandle{
		VersionVal:       4,
		HeaderSizeVal:    2048,
		ContainerTypeVal: containerType,
		IsSeekableVal:    isSeekable,
		ContainerIDVal:   "test-container-id",
		HeaderV4Val: &types.EnvelopeHeaderV4{
			Flags:          1,
			ManifestOffset: 2048,
			ManifestLength: uint32(manifestSize),
			ContainerType:  containerType,
			IsSeekable: func() uint8 {
				if isSeekable {
					return 1
				}
				return 0
			}(),
		},
		FooterV4Val: &types.EnvelopeFooterV4{},
		ManifestV4Val: &types.Manifest_v4{
			Version:          4,
			ContainerID:      "test-container-id",
			ContainerType:    "video",
			IsSeekable:       isSeekable,
			OriginalDuration: 0,
			Segments:         make([]types.Segment_v4, 0),
		},
	}
	for i := 0; i < segmentCount; i++ {
		h.ManifestV4Val.Segments = append(h.ManifestV4Val.Segments, types.Segment_v4{
			ID:     fmt.Sprintf("seg_%d", i),
			Offset: uint64(i * 1024),
			Size:   1024,
		})
	}
	return h
}

// NewMockV3Handle 创建一个模拟 V3 容器句柄
func NewMockV3Handle(containerType uint16, isSeekable bool, fragCount int) *MockContainerHandle {
	frags := make([]types.Fragment, 0, fragCount)
	var offset uint64
	for i := 0; i < fragCount; i++ {
		frags = append(frags, types.Fragment{
			ID:                fmt.Sprintf("logical_fragment_%d", i),
			Type:              types.FragmentType_SeekableStream,
			Length:            4096,
			GlobalStartOffset: offset,
		})
		offset += 4096
	}
	mf := &types.Manifest{
		Kind:      "video",
		Fragments: frags,
	}
	return &MockContainerHandle{
		VersionVal:       3,
		HeaderSizeVal:    2048,
		ContainerTypeVal: containerType,
		IsSeekableVal:    isSeekable,
		ManifestVal:      mf,
		HeaderV3Val:      &types.EnvelopeHeaderV3{},
		FooterV2Val:      &types.EnvelopeFooter_v2{},
	}
}
