package metrics

import (
	"strings"
	"testing"
)

func TestRenderIncludesCountersAndBuildInfo(t *testing.T) {
	registry := New("v0.4.0", "abc1234")
	done := registry.Begin()
	done("GET", "/api/v1/products", 200)
	doneSecond := registry.Begin()
	doneSecond("POST", "/api/v1/redeem", 500)

	output := registry.Render()
	for _, want := range []string{
		`nover_http_requests_total{method="GET",route="/api/v1/products",status="200"} 1`,
		`nover_http_requests_total{method="POST",route="/api/v1/redeem",status="500"} 1`,
		`nover_http_requests_in_flight 0`,
		`nover_build_info{version="v0.4.0",commit="abc1234"} 1`,
		"# TYPE nover_http_requests_total counter",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("Render() missing %q:\n%s", want, output)
		}
	}
}

func TestRenderBoundsLabels(t *testing.T) {
	registry := New("dev", "unknown")
	done := registry.Begin()
	done("GET", "/api/v1/"+strings.Repeat("x", 400), 404)
	output := registry.Render()
	if len(output) > 2000 {
		t.Fatalf("Render() output unexpectedly large: %d bytes", len(output))
	}
	if strings.Contains(output, strings.Repeat("x", 300)) {
		t.Fatal("Render() did not bound the route label")
	}
}
