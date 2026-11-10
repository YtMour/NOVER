package api

import "net/http"

type permission string

const (
	permissionProductRead       permission = "product.read"
	permissionProductWrite      permission = "product.write"
	permissionProductPublish    permission = "product.publish"
	permissionFulfillmentWrite  permission = "fulfillment.write"
	permissionCodeIssue         permission = "code.issue"
	permissionCodeManage        permission = "code.manage"
	permissionActivityRead      permission = "activity.read"
	permissionMemberRead        permission = "member.read"
	permissionApplicationRead   permission = "application.read"
	permissionApplicationReview permission = "application.review"
	permissionSettingsRead      permission = "settings.read"
	permissionSettingsManage    permission = "settings.manage"
)

var tenantRolePermissions = map[string][]permission{
	"OWNER":       {permissionProductRead, permissionProductWrite, permissionProductPublish, permissionFulfillmentWrite, permissionCodeIssue, permissionCodeManage, permissionActivityRead, permissionMemberRead},
	"ADMIN":       {permissionProductRead, permissionProductWrite, permissionProductPublish, permissionFulfillmentWrite, permissionCodeIssue, permissionCodeManage, permissionActivityRead, permissionMemberRead},
	"EDITOR":      {permissionProductRead, permissionProductWrite},
	"FULFILLMENT": {permissionProductRead, permissionFulfillmentWrite, permissionCodeIssue, permissionCodeManage, permissionActivityRead},
	"ANALYST":     {permissionProductRead, permissionActivityRead},
}

var platformRolePermissions = map[string][]permission{
	"PLATFORM_OWNER":   {permissionApplicationRead, permissionApplicationReview, permissionSettingsRead, permissionSettingsManage},
	"PLATFORM_SUPPORT": {permissionApplicationRead, permissionSettingsRead},
	"PLATFORM_AUDITOR": {permissionApplicationRead, permissionSettingsRead},
}

func permissionSet(identity operatorIdentity) map[permission]struct{} {
	result := make(map[permission]struct{})
	// Pending applicants may inspect their session only; activation is granted
	// by the approval workflow before any tenant permission is restored.
	if identity.MembershipStatus != "" && identity.MembershipStatus != "ACTIVE" {
		return result
	}
	for _, item := range tenantRolePermissions[identity.Role] {
		result[item] = struct{}{}
	}
	for _, item := range platformRolePermissions[identity.PlatformRole] {
		result[item] = struct{}{}
	}
	return result
}

func permissionNames(identity operatorIdentity) []string {
	set := permissionSet(identity)
	ordered := []permission{
		permissionProductRead, permissionProductWrite, permissionProductPublish, permissionFulfillmentWrite,
		permissionCodeIssue, permissionCodeManage, permissionActivityRead, permissionMemberRead,
		permissionApplicationRead, permissionApplicationReview, permissionSettingsRead, permissionSettingsManage,
	}
	result := make([]string, 0, len(set))
	for _, item := range ordered {
		if _, ok := set[item]; ok {
			result = append(result, string(item))
		}
	}
	return result
}

func (a *App) requirePermission(required permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity, ok := operatorIdentityFromContext(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "operator_auth_required", "Operator sign-in is required.")
				return
			}
			if _, allowed := permissionSet(identity)[required]; !allowed {
				writeError(w, http.StatusForbidden, "operator_permission_required", "This account does not have permission for the requested operation.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
