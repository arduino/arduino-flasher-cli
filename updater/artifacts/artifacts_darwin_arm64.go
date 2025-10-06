package artifacts

import (
	_ "embed"
)

//go:embed resources_darwin_arm64/qdl
var QdlBinary []byte
