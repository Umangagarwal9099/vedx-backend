package controller

import (
	"context"
	"log"

	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
	"github.com/umangagarwal/vedx-backend/service"
)

// emailUsersByRoles emails every user holding any of the given roles, deduped
// by address. Mirrors the recipient set used by NotifyRoles for the same
// events, so email volume matches in-app notification volume — for
// broadcast-style events (e.g. module creation, which notifies every
// student) that means one email per user in the system, not just staff.
func emailUsersByRoles(ctx context.Context, userRepo *repository.UserRepository, emailSvc *service.EmailService, roles []models.Role, subject, html string) {
	if !emailSvc.Configured() {
		return
	}
	seen := make(map[string]bool)
	for _, role := range roles {
		users, err := userRepo.FindByRole(ctx, role)
		if err != nil {
			log.Printf("fetch users by role %s for email: %v", role, err)
			continue
		}
		for _, u := range users {
			if u.Email == "" || seen[u.Email] {
				continue
			}
			seen[u.Email] = true
			emailSvc.SendAsync(u.Email, subject, html)
		}
	}
}
