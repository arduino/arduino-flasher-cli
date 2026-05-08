package artifacts

import (
	_ "embed"
)

//go:embed resources_darwin_amd64/qdl
var QdlBinary []byte
