// Package buildinfo carries release metadata injected at build time.
//
// Release builds set these through -ldflags, for example:
//
//	go build -ldflags "-X github.com/YtMour/NOVER/server/internal/buildinfo.Version=v0.2.0 \
//	  -X github.com/YtMour/NOVER/server/internal/buildinfo.Commit=$(git rev-parse --short HEAD) \
//	  -X github.com/YtMour/NOVER/server/internal/buildinfo.BuiltAt=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
package buildinfo

// Values default to development placeholders so `go run ./cmd/api` works
// without a release pipeline.
var (
	Version = "dev"
	Commit  = "unknown"
	BuiltAt = "unknown"
)

// Info returns the build metadata for diagnostics and the public meta endpoint.
func Info() map[string]string {
	return map[string]string{
		"version":  Version,
		"commit":   Commit,
		"built_at": BuiltAt,
	}
}
