package schemas

import "embed"

//go:embed **/*.json
var EmbeddedSchemas embed.FS
