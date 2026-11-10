package api

import "testing"

func TestJoinApplicationNormalizeAndValidate(t *testing.T) {
	input := joinApplicationInput{
		OrganizationName: "  NOVER Studio  ",
		ContactName:      "  Operator  ",
		Email:            "  TEAM@EXAMPLE.COM  ",
		DesiredUsername:  "  Team_Operator  ",
		ExpectedPassword: "local-test-password",
		UseCase:          "  Deliver source packages and updates to customers.  ",
	}
	input.normalize()
	if input.Email != "team@example.com" || input.DesiredUsername != "team_operator" {
		t.Fatalf("normalized application = %#v", input)
	}
	if !input.valid() {
		t.Fatal("valid join application was rejected")
	}

	input.DesiredUsername = "Admin"
	if input.valid() {
		t.Fatal("invalid desired username was accepted")
	}
	input.DesiredUsername = "team_operator"
	input.ExpectedPassword = "short"
	if input.valid() {
		t.Fatal("short expected password was accepted")
	}
}
