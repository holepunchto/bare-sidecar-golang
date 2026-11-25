package sidecar

import (
	_ "embed"
	"encoding/json"
	"maps"
)

//go:embed versions.json
var manifest []byte

var versions map[string]string

func init() {
	if err := json.Unmarshal(manifest, &versions); err != nil {
		panic(err)
	}
}

// Versions returns the versions of the components bundled with the
// executable, keyed by name like `Bare.versions`. The "bare" key holds the
// version of Bare itself. The map is a copy, so changing it has no effect.
func Versions() map[string]string {
	return maps.Clone(versions)
}
