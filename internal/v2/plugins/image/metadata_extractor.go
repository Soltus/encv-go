package image

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Soltus/encv-go/internal/utils"
	"github.com/Soltus/encv-go/internal/v2/types"
	exif "github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
)

// 实现 plugins.MetadataExtractor 接口
type ImageMetadataExtractor struct {
	// 可以在这里注入依赖，例如配置
}

// 提取元数据
func (e *ImageMetadataExtractor) ExtractMetadata(inputPath string) (types.Index, error) {
	// 1. 获取基础文件信息
	fileInfo, err := os.Stat(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}
	mimeType, err := utils.DetectFileMIMEType(inputPath)
	if err != nil {
		log.Printf("failed to detect MIME type: %v", err)
	}
	index := &ImageIndex{
		OriginalFilename: fileInfo.Name(),
		OriginalFileSize: fileInfo.Size(),
		MimeType:         mimeType,
		ExifTags:         make(map[string]interface{}),
	}

	// 2. 提取并解析 EXIF 数据
	rawExif, err := exif.SearchFileAndExtractExif(inputPath)
	if err != nil {
		// 如果文件没有 EXIF 数据，这不是一个错误，只是缺少信息
		fmt.Printf("INFO: [ImageMetadataExtractor] No EXIF data found in '%s'.\n", inputPath)
		return index, nil
	}

	im, err := exifcommon.NewIfdMappingWithStandard()
	if err != nil {
		return nil, fmt.Errorf("failed to create IFD mapping: %w", err)
	}

	ti := exif.NewTagIndex()
	_, indexData, err := exif.Collect(im, ti, rawExif)
	if err != nil {
		return nil, fmt.Errorf("failed to parse EXIF data: %w", err)
	}

	rootIfd := indexData.RootIfd

	// 3. 填充我们关心的特定字段
	index.Make, _ = getTagValue(rootIfd, "Make")
	index.Model, _ = getTagValue(rootIfd, "Model")
	index.Software, _ = getTagValue(rootIfd, "Software")

	// 处理日期时间
	if dateStr, found := getTagValue(rootIfd, "DateTimeOriginal"); found {
		// EXIF 日期格式通常是 "2006:01:02 15:04:05"
		if parsedTime, err := time.Parse("2006:01:02 15:04:05", dateStr); err == nil {
			index.DateTimeOriginal = &parsedTime
		}
	}

	// 处理图片尺寸
	if widthStr, found := getTagValue(rootIfd, "PixelXDimension"); found {
		fmt.Sscanf(widthStr, "%d", &index.Width)
	}
	if heightStr, found := getTagValue(rootIfd, "PixelYDimension"); found {
		fmt.Sscanf(heightStr, "%d", &index.Height)
	}
	if orientationStr, found := getTagValue(rootIfd, "Orientation"); found {
		fmt.Sscanf(orientationStr, "%d", &index.Orientation)
	}

	gpsInfo, err := rootIfd.GpsInfo()
	if err == nil {
		// GpsInfo 结构体直接提供了 float64 类型的经纬度
		index.GPSLatitude = &gpsInfo.Latitude
		index.GPSLongitude = &gpsInfo.Longitude
	} else {
		// 如果 err != nil，说明没有 GPS 信息，这不是一个错误
		// 可以选择性地记录日志
		// fmt.Printf("INFO: [ImageMetadataExtractor] No GPS data found.\n")
	}

	// 5. (可选) 将所有 EXIF 标签存入 map
	// 使用 IFD 路径作为前缀来创建唯一的键，以避免不同 IFD 中的同名标签冲突。
	for _, ifd := range indexData.Ifds {
		// 【关键修复】调用 Entries() 方法来获取标签条目切片
		tagEntries := ifd.Entries()
		for _, tag := range tagEntries {
			tagName := tag.TagName()
			value, err := tag.Value()
			if err != nil {
				// 如果无法获取值，则跳过
				continue
			}

			// 创建一个唯一的键，格式为 "IFD路径|标签名"
			// 例如: "IFD0|Make", "Exif|DateTimeOriginal"
			ifdPath := tag.IfdPath()
			uniqueKey := fmt.Sprintf("%s|%s", ifdPath, tagName)

			index.ExifTags[uniqueKey] = value
		}
	}

	return index, nil
}

// getTagValue 是一个辅助函数，用于安全地从标签条目切片中获取第一个值
func getTagValue(ifd *exif.Ifd, tagName string) (string, bool) {
	// 【关键修复】FindTagWithName 返回一个切片
	tagEntries, err := ifd.FindTagWithName(tagName)
	if err != nil || len(tagEntries) == 0 {
		return "", false
	}

	// 获取第一个条目的值
	value, err := tagEntries[0].Value()
	if err != nil {
		return "", false
	}
	return fmt.Sprintf("%v", value), true
}
