package artifacts

import (
	_ "embed"
)

//go:embed resources_linux_amd64/qdl
var QdlBinary []byte
