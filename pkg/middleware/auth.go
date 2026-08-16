package middleware

import (
	"crypto/rsa"
	"fmt"
    "log"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gofiber/fiber/v2"
	"github.com/vertex/pet-service/pkg/apperror"
)

// NewAuthMiddleware returns a JWT auth middleware using the given RSA public key.
func NewAuthMiddleware(publicKey *rsa.PublicKey) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			return apperror.Unauthorized("Missing or invalid authorization header")
		}
		tokenString := authHeader[7:]

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return publicKey, nil
		})
		if err != nil || !token.Valid {
			return apperror.Unauthorized("Invalid or expired token")
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return apperror.Unauthorized("Invalid token claims")
		}

		userIDStr, ok := claims["sub"].(string)
		if !ok {
			return apperror.Unauthorized("Invalid token subject")
		}
		
		nameStr, _ := claims["name"].(string)

        log.Printf("[DEBUG] Auth middleware extracted sub: %s", userIDStr)
		c.Locals("userId", userIDStr)
		c.Locals("userName", nameStr)
		return c.Next()
	}
}
