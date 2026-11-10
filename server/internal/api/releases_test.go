package api

import "testing"

func TestProductReleaseInputValidation(t *testing.T) {
	input := productReleaseInput{Version: " v1.4.0 ", Changelog: "  Added tenant-aware delivery notes.  "}
	input.normalize()
	if input.Version != "v1.4.0" || input.Changelog != "Added tenant-aware delivery notes." || !input.valid() {
		t.Fatalf("normalized release input = %#v", input)
	}
	input.Version = "latest"
	if input.valid() {
		t.Fatal("non-semver release was accepted")
	}
}
