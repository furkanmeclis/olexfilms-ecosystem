package docs

import "embed"

// FS embeds the OpenAPI 3.1 contract served at /docs/openapi.yaml.
//
// The canonical spec lives at the repo root (docs/openapi.yaml). This file is
// a synced copy so the backend Docker build (context ./backend) can embed it;
// run `make openapi-sync` after editing the canonical spec. TestSpecInSync
// fails when the two drift apart.
//
//go:embed openapi.yaml
var FS embed.FS
