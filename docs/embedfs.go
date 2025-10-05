package docs

import "embed"

// EmbeddedDocs contains all documentation files
//
//go:embed docs
var EmbeddedDocs embed.FS

// EmbeddedSchemas contains all JSON schema files
//
//go:embed schemas
var EmbeddedSchemas embed.FS
