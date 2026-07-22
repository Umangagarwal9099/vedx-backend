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
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": "FEATURE_NOT_ENABLED", "error": "the " + featureKey + " feature is not enabled for your college"})
			return
		}
		c.Next()
	}
}

// RequireActiveSubscription blocks a route unless the caller's college has
// an active subscription — composed BEFORE RequireFeature (subscription
// gates access to ALL features, not just one). super_admin always bypasses.
// A college with no college_subscriptions rows at all is treated as
// unrestricted (see CollegeRepository.IsSubscriptionActive) so this can be
// adopted gradually. Blocking access here never deletes or hides any data —
// only routes gated by this middleware become inaccessible; everything
// already stored is preserved and becomes reachable again once renewed.
//
// Must be placed after JWTAuth in the middleware chain.
func RequireActiveSubscription(collegeRepo *repository.CollegeRepository) gin.HandlerFunc {
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

		active, ok := collegeRepo.IsSubscriptionActive(c.Request.Context(), collegeID)
		if !ok {
			// Migration not applied yet, or a transient error — fail open,
			// never block access over an unverifiable subscription state.
			c.Next()
			return
		}
		if !active {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code": "COLLEGE_SUBSCRIPTION_EXPIRED",
				"error": "Your college subscription has expired. Contact the platform administrator.",
			})
			return
		}
		c.Next()
	}
}
