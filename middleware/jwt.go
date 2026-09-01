package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/umangagarwal/vedx-backend/auth"
	"github.com/umangagarwal/vedx-backend/models"
)

// JWTAuth validates the Bearer token in the Authorization header.
// Downstream handlers can read user_id, email, role from the Gin context.
func JWTAuth(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid authorization header"})
			return
		}

		claims, err := auth.ValidateToken(strings.TrimPrefix(header, "Bearer "), jwtSecret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)
		c.Set("role", claims.Role)
		c.Set("college_id", claims.CollegeID)
		c.Set("department", claims.Department)
		c.Next()
	}
}

// StreamCookieName is the httpOnly session cookie set at login, used only to
// authenticate requests a browser makes without a custom Authorization
// header — namely <video src> for recording streaming. It carries the same
// JWT as the Bearer token; it's just delivered a second way for the one kind
// of request that can't attach a header.
const StreamCookieName = "vedx_stream_session"

// JWTAuthCookieOrHeader validates either the Bearer token in the
// Authorization header (same as JWTAuth) or, when that's absent, the
// StreamCookieName cookie. Reserved for routes a <video> element hits
// directly — every other route keeps using JWTAuth, so the cookie's blast
// radius stays limited to streaming instead of becoming a second, parallel
// auth path for the whole API.
func JWTAuthCookieOrHeader(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := ""
		if header := c.GetHeader("Authorization"); strings.HasPrefix(header, "Bearer ") {
			tokenStr = strings.TrimPrefix(header, "Bearer ")
		} else if cookie, err := c.Cookie(StreamCookieName); err == nil {
			tokenStr = cookie
		}
		if tokenStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid authorization"})
			return
		}

		claims, err := auth.ValidateToken(tokenStr, jwtSecret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)
		c.Set("role", claims.Role)
		c.Set("college_id", claims.CollegeID)
		c.Set("department", claims.Department)
		c.Next()
	}
}

// RequireRole restricts a route to users whose role is in the allowed list.
// Must be placed after JWTAuth in the middleware chain.
//
// Example: router.GET("/admin", middleware.JWTAuth(secret), middleware.RequireRole(models.RoleSuperAdmin), handler)
func RequireRole(roles ...models.Role) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[string(r)] = struct{}{}
	}

	return func(c *gin.Context) {
		role, _ := c.Get("role")
		if _, ok := allowed[role.(string)]; !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "you do not have permission to access this resource"})
			return
		}
		c.Next()
	}
}

// RequireRoleOrDepartment allows a request through if EITHER the caller's
// role is in the allowed list OR their department matches (case: an
// employee whose department is "hr" needs access to routes that would
// otherwise be gated to super_admin/team_lead, e.g. the leave-request
// review screen, without granting them every other adminOrAbove route).
// Must be placed after JWTAuth in the middleware chain.
func RequireRoleOrDepartment(department string, roles ...models.Role) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[string(r)] = struct{}{}
	}

	return func(c *gin.Context) {
		if c.GetString("department") == department {
			c.Next()
			return
		}
		role, _ := c.Get("role")
		if _, ok := allowed[role.(string)]; !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "you do not have permission to access this resource"})
			return
		}
		c.Next()
	}
}

// RequireSelfOrRole allows a request through if EITHER the caller's own
// user_id matches the route param named paramName (e.g. "id" in
// "/users/:id/streak" — a user reading their own data), OR their role is in
// the allowed list (staff reading someone else's data). Must be placed
// after JWTAuth in the middleware chain.
func RequireSelfOrRole(paramName string, roles ...models.Role) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[string(r)] = struct{}{}
	}

	return func(c *gin.Context) {
		if c.Param(paramName) == c.GetString("user_id") {
			c.Next()
			return
		}
		role, _ := c.Get("role")
		if _, ok := allowed[role.(string)]; !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "you do not have permission to access this resource"})
			return
		}
		c.Next()
	}
}
