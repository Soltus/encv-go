package packer

import (
	"io"
	"log"
	"os"

	"github.com/Soltus/encv-go/internal/v2/namer"
	"github.com/Soltus/encv-go/internal/v2/physical"
	"github.com/Soltus/encv-go/internal/v2/types"
)

// PackParams 通用打包所需的所有参数
type PackParams struct {
	// --- 核心数据 ---
	Manifest       *types.Manifest
	PhysicalPacker physical.PhysicalPacker
	TempEncPath    string

	// --- 加密参数 ---
	Salt                 []byte
	IV                   []byte
	SaltIVHeaderSize     int64 // Salt + IV 头的大小 (通常是 32)
	EncryptedPayloadSize int64 // 加密数据载荷的大小

	// --- Packer 配置字段 (原 PackRequest 字段) ---
	// Plugin 直接填充这些字段，Helper 负责搬运到 physical.PackRequest
	BaseName              string
	OutputDir             string
	Index                 types.Index
	Namer                 namer.ChunkNamer
	StartIdx              int
	LightMainChunkEnabled bool
	HeaderVersion         int
	ContainerType         uint16
	IsSeekable            bool
	SpecialID             []byte
	SpecialIDType         types.IDType
	FinalFileName         string

	PasswordHint [16]byte

	WrappedDEK *types.WrappedDEK
}

// StandardPostEncrypt 执行通用的打包流程
// 【唯一代理】所有插件都必须调用此函数来完成打包
// 职责：
// 1. 根据 TempEncPath 和 SaltIVHeaderSize 生成 EncryptedDataReader
// 2. 将 PackParams 中的配置组装成 physical.PackRequest
// 3. 调用 PhysicalPacker
func StandardPostEncrypt(params *PackParams) (string, error) {
	// 1. 打开加密文件
	f, err := os.Open(params.TempEncPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// 2. 【性能优化】根据显式大小生成 EncryptedDataReader
	var encryptedDataReader io.Reader = f

	if params.SaltIVHeaderSize > 0 && params.EncryptedPayloadSize > 0 {
		// 显式模式：直接创建 SectionReader，性能最佳
		log.Printf("DEBUG: [Helper] Using explicit sizes. Skipping %d bytes (Payload: %d).\n", params.SaltIVHeaderSize, params.EncryptedPayloadSize)
		encryptedDataReader = io.NewSectionReader(f, params.SaltIVHeaderSize, params.EncryptedPayloadSize)
	} else {
		// 【回退模式】兼容旧逻辑，如果没有传入显式大小，则进行智能判断
		fileInfo, err := f.Stat()
		if err != nil {
			return "", err
		}
		totalSize := fileInfo.Size()

		var totalLogicalSize int64 = 0
		for _, frag := range params.Manifest.Fragments {
			totalLogicalSize += int64(frag.Length)
		}

		saltIVSize := int64(len(params.Salt) + len(params.IV))
		expectedSizeWithHeader := totalLogicalSize + saltIVSize

		switch totalSize {
		case expectedSizeWithHeader:
			log.Printf("DEBUG: [Helper] Encrypted file contains Salt/IV header. Skipping %d bytes.\n", saltIVSize)
			encryptedDataReader = io.NewSectionReader(f, saltIVSize, totalLogicalSize)
		case totalLogicalSize:
			log.Printf("DEBUG: [Helper] Encrypted file does NOT contain Salt/IV header (or stripped). Reading directly.\n")
			encryptedDataReader = f
		default:
			// 异常情况
			diff := totalSize - totalLogicalSize
			if diff == saltIVSize {
				log.Printf("DEBUG: [Helper] File size matches logical size + header. Skipping %d bytes.\n", saltIVSize)
				encryptedDataReader = io.NewSectionReader(f, saltIVSize, totalLogicalSize)
			} else {
				log.Printf("WARN: [Helper] Unexpected file size %d (Logical: %d, Diff: %d). Reading full file.\n", totalSize, totalLogicalSize, diff)
				encryptedDataReader = f
			}
		}
	}

	// 3. 【关键】组装 physical.PackRequest
	// Helper 负责将 Plugin 提供的配置“翻译”给 Packer
	packReq := &physical.PackRequest{
		EncryptedDataReader:   encryptedDataReader,
		Index:                 params.Index,
		Salt:                  params.Salt,
		IV:                    params.IV,
		BaseName:              params.BaseName,
		OutputDir:             params.OutputDir,
		Namer:                 params.Namer,
		StartIdx:              params.StartIdx,
		LightMainChunkEnabled: params.LightMainChunkEnabled,
		HeaderVersion:         params.HeaderVersion,
		ContainerType:         params.ContainerType,
		IsSeekable:            params.IsSeekable,
		SpecialID:             params.SpecialID,
		SpecialIDType:         params.SpecialIDType,
		FinalFileName:         params.FinalFileName,
		PasswordHint:          params.PasswordHint,
		WrappedDEK:            params.WrappedDEK,
	}

	outputPath, err := params.PhysicalPacker.Pack(params.Manifest, packReq)
	if err != nil {
		return "", err
	}
	return outputPath, nil
}
