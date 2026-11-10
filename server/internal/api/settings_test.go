package api

import "testing"

func TestPlatformSettingsNormalizeAndValidate(t *testing.T) {
	settings := platformSettings{
		DeploymentMode:  " split_apps ",
		StorefrontURL:   "https://store.example.com/",
		ConsoleURL:      "https://console.example.com/",
		APIBaseURL:      "https://api.example.com/",
		OperatorWebPath: " /Ops_Team ",
		OperatorAPIPath: "/Control_Team/",
	}
	settings.normalize()
	if settings.OperatorWebPath != "/ops_team" || settings.OperatorAPIPath != "control_team" {
		t.Fatalf("normalized settings = %#v", settings)
	}
	if !settings.valid() {
		t.Fatal("valid platform settings were rejected")
	}

	settings.APIBaseURL = "javascript:alert(1)"
	if settings.valid() {
		t.Fatal("non-HTTP API URL was accepted")
	}
}
