package filename

import (
	"context"

	"github.com/Soltus/encv-go/internal/v2/types"
)

type ManifestV4Ref interface {
	GetOriginalName() string
	GetFilenameAlgorithm() string
}

func ResolveDisplayName(
	ctx context.Context,
	physicalName string,
	manifest ManifestV4Ref,
	headerFlags uint16,
	password string,
	cfg FNConfig,
) (string, error) {
	if manifest == nil || manifest.GetOriginalName() == "" {
		return physicalName, nil
	}

	if headerFlags&types.FlagFilenameEncrypted == 0 {
		return manifest.GetOriginalName(), nil
	}

	cfg.Password = []byte(password)
	decoded, err := cfg.Decode(manifest.GetOriginalName())
	if err != nil {
		return physicalName, nil
	}
	return string(decoded), nil
}
