package util

import "github.com/umangagarwal/vedx-backend/models"

// Portal login URLs used in account-creation and password-reset emails.
// Students land on the learning portal, the Employee role lands on the
// employee-management portal, and every other internal/admin-tier role
// (super admin, platform admin, college admin, college staff, mentor,
// team lead) lands on the admin-management portal.
const (
	studentLoginURL  = "https://learn.vedxlence.in/login"
	employeeLoginURL = "https://portal-employee-management.vedxlence.in/login"
	adminLoginURL    = "https://portal-admin-management.vedxlence.in/login"
)

// LoginURLForRole returns the portal URL that belongs in a welcome/reset
// email's "Log in" link for the given role.
func LoginURLForRole(role models.Role) string {
	switch role {
	case models.RoleStudent:
		return studentLoginURL
	case models.RoleEmployee:
		return employeeLoginURL
	default:
		return adminLoginURL
	}
}
