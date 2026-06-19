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
