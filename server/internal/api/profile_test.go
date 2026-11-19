package api

import (
	"strings"
	"testing"
	"time"
)

func TestProductProfileUpdatePolicyValidation(t *testing.T) {
	until := time.Date(2027, 1, 31, 0, 0, 0, 0, time.UTC)
	base := productInput{
		Name:          "Profile product",
		Description:   "short",
		Detail:        "detail",
		Highlights:    []string{"  one  ", "", "two"},
		LicenseTerms:  "license",
		Requirements:  "requirements",
		SupportPeriod: "30 days",
	}
	tests := []struct {
		name     string
		policy   string
		until    *time.Time
		wantOK   bool
		wantHigh []string
	}{
		{name: "default lifetime", policy: "", wantOK: true, wantHigh: []string{"one", "two"}},
		{name: "lifetime", policy: "LIFETIME", wantOK: true},
		{name: "current only", policy: "CURRENT_ONLY", wantOK: true},
		{name: "till date", policy: "TILL_DATE", until: &until, wantOK: true},
		{name: "till date without timestamp", policy: "TILL_DATE", wantOK: false},
		{name: "unknown policy", policy: "ALWAYS", wantOK: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := base
			input.Highlights = append([]string(nil), base.Highlights...)
			input.UpdatePolicy = test.policy
			input.UpdateUntil = test.until
			input.normalize()
			if got := input.valid(); got != test.wantOK {
				t.Fatalf("valid() = %v, want %v (input=%#v)", got, test.wantOK, input)
			}
			if test.wantHigh != nil && strings.Join(input.Highlights, ",") != strings.Join(test.wantHigh, ",") {
				t.Fatalf("highlights = %v, want %v", input.Highlights, test.wantHigh)
			}
			if test.policy != "TILL_DATE" && input.UpdateUntil != nil && test.wantOK {
				t.Fatalf("update_until should be cleared for policy %q", test.policy)
			}
		})
	}
}

func TestProductProfileLimits(t *testing.T) {
	tooMany := productInput{Name: "P", Description: "", Detail: "", UpdatePolicy: "LIFETIME", Highlights: []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"}}
	tooMany.normalize()
	if tooMany.valid() {
		t.Fatal("more than 8 highlights should be rejected")
	}

	tooLong := productInput{Name: "P", UpdatePolicy: "LIFETIME", Highlights: []string{strings.Repeat("a", 121)}}
	tooLong.normalize()
	if tooLong.valid() {
		t.Fatal("highlight longer than 120 characters should be rejected")
	}

	empty := productInput{Name: "P", UpdatePolicy: "LIFETIME", Highlights: []string{"   ", ""}}
	empty.normalize()
	if !empty.valid() || len(empty.Highlights) != 0 {
		t.Fatalf("blank highlights should be dropped, got %v (valid=%v)", empty.Highlights, empty.valid())
	}
}

func TestMemberCreateInputValidation(t *testing.T) {
	valid := memberCreateInput{Username: "EditorOne", DisplayName: "运营编辑", Password: "strong-password", Role: "editor"}
	valid.normalize()
	if valid.Username != "editorone" || valid.Role != "EDITOR" || !valid.valid() {
		t.Fatalf("normalized member input = %#v valid=%v", valid, valid.valid())
	}

	for _, input := range []memberCreateInput{
		{Username: "ab", DisplayName: "Name", Password: "strong-password", Role: "EDITOR"},
		{Username: "validname", DisplayName: "N", Password: "strong-password", Role: "EDITOR"},
		{Username: "validname", DisplayName: "Name", Password: "short", Role: "EDITOR"},
		{Username: "validname", DisplayName: "Name", Password: "strong-password", Role: "OWNER"},
		{Username: "validname", DisplayName: "Name", Password: "strong-password", Role: "ROOT"},
	} {
		candidate := input
		candidate.normalize()
		if candidate.valid() {
			t.Fatalf("invalid member input accepted: %#v", candidate)
		}
	}
}

func TestPermissionMatrixIncludesMemberAndAuditManagement(t *testing.T) {
	for _, role := range []string{"OWNER", "ADMIN"} {
		identity := operatorIdentity{Role: role, MembershipStatus: "ACTIVE"}
		names := permissionNames(identity)
		joined := strings.Join(names, ",")
		if !strings.Contains(joined, "member.manage") || !strings.Contains(joined, "audit.read") {
			t.Fatalf("role %s permissions = %v, want member.manage and audit.read", role, names)
		}
	}
	for _, role := range []string{"EDITOR", "FULFILLMENT", "ANALYST"} {
		identity := operatorIdentity{Role: role, MembershipStatus: "ACTIVE"}
		names := strings.Join(permissionNames(identity), ",")
		if strings.Contains(names, "member.manage") || strings.Contains(names, "audit.read") {
			t.Fatalf("role %s should not manage members or audit logs: %v", role, names)
		}
	}
}
