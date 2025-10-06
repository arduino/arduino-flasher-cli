package artifacts

import (
	_ "embed"
)

//go:embed resources_windows_amd64/qdl.exe
var QdlBinary []byte
