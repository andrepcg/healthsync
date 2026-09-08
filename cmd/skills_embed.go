package cmd

import "embed"

// EmbeddedSkills contains the agent skills files baked into the binary:
//   - skills/healthsync      — CLI/SQLite skill installed with `healthsync skills install`
//   - skills/healthsync-api  — HTTP API skill served by the server at /skill/
//
//go:embed skills/healthsync skills/healthsync-api
var EmbeddedSkills embed.FS
