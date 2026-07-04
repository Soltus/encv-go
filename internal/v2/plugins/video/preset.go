package video

import "fmt"

type StreamPreset string

const (
	StreamPresetBalanced    StreamPreset = "balanced"
	StreamPresetQuality     StreamPreset = "quality"
	StreamPresetHighQuality StreamPreset = "high_quality"
)

type PresetParams struct {
	Name             string
	Quality          int32
	BitrateMode      string
	KeyFrameInterval int32
	LowLatency       bool
	Profile          string
	Description      string
}

var MobilePresets = map[StreamPreset]PresetParams{
	StreamPresetBalanced: {
		Name:             "平衡（推荐）",
		Quality:          28,
		BitrateMode:      "VBR",
		KeyFrameInterval: 2,
		LowLatency:       true,
		Profile:          "main",
		Description:      "画质较好、体积适中、延迟可接受",
	},
	StreamPresetQuality: {
		Name:             "高质量",
		Quality:          24,
		BitrateMode:      "VBR",
		KeyFrameInterval: 2,
		LowLatency:       true,
		Profile:          "high",
		Description:      "画质更好，体积稍大，延迟略高",
	},
	StreamPresetHighQuality: {
		Name:             "极致画质",
		Quality:          20,
		BitrateMode:      "VBR",
		KeyFrameInterval: 3,
		LowLatency:       false,
		Profile:          "high",
		Description:      "画质最佳，体积控制好，但延迟更高",
	},
}

type DesktopPresetParams struct {
	Name             string
	NVENCPreset      string
	NVENCTune        string
	KeyFrameInterval int32
	Profile          string
	Description      string
}

var DesktopPresets = map[StreamPreset]DesktopPresetParams{
	StreamPresetBalanced: {
		Name:             "平衡（推荐）",
		NVENCPreset:      "p4",
		NVENCTune:        "ull",
		KeyFrameInterval: 2,
		Profile:          "main",
		Description:      "画质较好、体积适中、延迟可接受",
	},
	StreamPresetQuality: {
		Name:             "高质量",
		NVENCPreset:      "p5",
		NVENCTune:        "ull",
		KeyFrameInterval: 2,
		Profile:          "high",
		Description:      "画质更好，体积稍大，延迟略高",
	},
	StreamPresetHighQuality: {
		Name:             "极致画质",
		NVENCPreset:      "p7",
		NVENCTune:        "hq",
		KeyFrameInterval: 3,
		Profile:          "high",
		Description:      "画质最佳，体积控制好，但延迟更高",
	},
}

func GetPreset(presetName string) (StreamPreset, error) {
	if presetName == "" {
		return StreamPresetBalanced, nil
	}
	p := StreamPreset(presetName)
	switch p {
	case StreamPresetBalanced, StreamPresetQuality, StreamPresetHighQuality:
		return p, nil
	default:
		return "", fmt.Errorf("unknown stream preset: %q", presetName)
	}
}

func GetMobilePresetParams(preset StreamPreset) (PresetParams, error) {
	params, ok := MobilePresets[preset]
	if !ok {
		return PresetParams{}, fmt.Errorf("mobile preset not found: %q", preset)
	}
	return params, nil
}

func GetDesktopPresetParams(preset StreamPreset) (DesktopPresetParams, error) {
	params, ok := DesktopPresets[preset]
	if !ok {
		return DesktopPresetParams{}, fmt.Errorf("desktop preset not found: %q", preset)
	}
	return params, nil
}
