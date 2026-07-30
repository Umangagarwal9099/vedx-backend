package middleware

import "github.com/gin-gonic/gin"

// CORS allows cross-origin requests. Requests from an origin in
// allowedOrigins get the response echoed back with
// Access-Control-Allow-Credentials so the browser will send/store the
// httpOnly session cookie used to authenticate video-streaming requests —
// per the CORS spec, a credentialed request can't be paired with a wildcard
// Access-Control-Allow-Origin, it must be the exact requesting origin.
// Requests from any other (or no) origin still get a wide-open,
// credential-less response, preserving this API's previous behavior for
// plain bearer-token callers (mobile apps, curl, tools, etc.).
func CORS(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = true
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if allowed[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Vary", "Origin")
		} else {
			c.Header("Access-Control-Allow-Origin", "*")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Accept, Content-Type, Authorization, X-Requested-With, X-Device-Id")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
