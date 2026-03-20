package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ExtractBearerToken parses `Authorization: Bearer <token>` and returns the token.
func ExtractBearerToken(header string) (string, bool) {
	fields := strings.Fields(header)
	if len(fields) != 2 {
		return "", false
	}
	if fields[0] != "Bearer" || fields[1] == "" {
		return "", false
	}
	return fields[1], true
}

// BearerTokenMiddleware validates Bearer token from Authorization header.
// If expectedToken is empty, auth is disabled and all requests pass through.
func BearerTokenMiddleware(expectedToken string) gin.HandlerFunc {
	token := strings.TrimSpace(expectedToken)
	return func(c *gin.Context) {
		if token == "" {
			c.Next()
			return
		}

		incomingToken, ok := ExtractBearerToken(c.GetHeader("Authorization"))
		if !ok || incomingToken != token {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		c.Next()
	}
}
