package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/models"
	"github.com/umangagarwal/vedx-backend/repository"
)

// RequireFeature blocks a route unless the caller's college has featureKey
// enabled. super_admin always bypasses (manages every college, not scoped to
// one). A caller with no college_id — legacy accounts that predate
// multi-tenancy, or the auto-created Default College's members before it's
// been reconfigured — also bypasses, since that college is backfilled with
// every feature enabled by the migration; this keeps existing users' access
// unchanged until a college is deliberately reconfigured.
//
// Must be placed after JWTAuth in the middleware chain.
func RequireFeature(collegeRepo *repository.CollegeRepository, featureKey models.FeatureKey) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString("role") == string(models.RoleSuperAdmin) {
			c.Next()
			return
		}

		collegeID := c.GetString("college_id")
		if collegeID == "" {
			c.Next()
			return
		}

		enabled, err := collegeRepo.HasFeature(c.Request.Context(), collegeID, featureKey)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "could not verify feature access"})
			return
		}
		if !enabled {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "the " + featureKey + " feature is not enabled for your college"})
			return
		}
		c.Next()
	}
}
