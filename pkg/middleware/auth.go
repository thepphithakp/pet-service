package middleware

import (
	"crypto/rsa"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/vertex/pet-service/internal/domain"
	"github.com/vertex/pet-service/pkg/apperror"
)

// AuthConfig ควบคุมความเข้มของการตรวจ token
type AuthConfig struct {
	// PublicKeys คือ public key ทุกใบที่ยอมรับได้
	//
	// รับหลายใบเพื่อให้ rotate key ได้โดยไม่มี downtime:
	// ระหว่างเปลี่ยนผ่านต้องยอมรับทั้งใบเก่า (สำหรับ token ที่ยังไม่หมดอายุ)
	// และใบใหม่ไปพร้อมกัน
	PublicKeys []*rsa.PublicKey

	// Issuer / Audience ตรวจเมื่อไม่ว่างเท่านั้น
	//
	// ⚠️ ต้องปล่อยแบบ 2 เฟส: ให้ auth-service เริ่มออก iss/aud ก่อน
	// รออย่างน้อยเท่าอายุ token เดิม (72 ชม.) แล้วค่อยเปิดค่าสองตัวนี้
	// ไม่งั้น token ที่ผู้ใช้ถืออยู่จะใช้ไม่ได้ทันทีทั้งระบบ
	Issuer   string
	Audience string

	// AdminUserIDs เป็นสะพานชั่วคราวสำหรับช่วงที่ auth-service ยังไม่ออก roles claim
	//
	// user id ในรายการนี้จะได้ SUPER_ADMIN เพื่อให้ backoffice ใช้งานได้
	// ระหว่างรอ Phase 1A — ถอดออกทันทีเมื่อ roles claim พร้อมใช้
	AdminUserIDs map[uuid.UUID]bool
}

// NewAuthMiddleware ตรวจ JWT แล้วผูก domain.Actor เข้ากับ context
func NewAuthMiddleware(cfg AuthConfig) fiber.Handler {
	opts := []jwt.ParserOption{
		// defense-in-depth คู่กับการเช็ค method ใน keyfunc — กัน alg confusion
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithLeeway(30 * time.Second),
	}
	if cfg.Issuer != "" {
		opts = append(opts, jwt.WithIssuer(cfg.Issuer))
	}
	if cfg.Audience != "" {
		opts = append(opts, jwt.WithAudience(cfg.Audience))
	}

	keys := newKeySet(cfg.PublicKeys)

	return func(c *fiber.Ctx) error {
		tokenString, ok := bearerToken(c.Get("Authorization"))
		if !ok {
			return apperror.Unauthorized("Missing or invalid authorization header")
		}

		token, err := jwt.Parse(tokenString, keys.keyfunc, opts...)
		if err != nil || !token.Valid {
			return apperror.Unauthorized("Invalid or expired token")
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return apperror.Unauthorized("Invalid token claims")
		}

		sub, ok := claims["sub"].(string)
		if !ok {
			return apperror.Unauthorized("Invalid token subject")
		}
		userID, err := uuid.Parse(sub)
		if err != nil {
			return apperror.Unauthorized("Invalid user ID in token")
		}

		name, _ := claims["name"].(string)
		email, _ := claims["email"].(string)

		actor := domain.Actor{
			UserID:   userID,
			Username: name,
			Email:    email,
			Roles:    rolesFromClaims(claims, cfg.AdminUserIDs, userID),
		}

		c.SetUserContext(domain.WithActor(c.UserContext(), actor))

		// คงไว้เพื่อความเข้ากันได้กับโค้ดเดิมที่ยังอ่าน Locals และ access log
		c.Locals("userId", sub)
		c.Locals("userName", name)
		return c.Next()
	}
}

// rolesFromClaims อ่าน roles จาก token
//
// token ที่ยังไม่มี roles (ออกก่อน Phase 1A) ถือเป็น USER ธรรมดา
// ทำให้ deploy ได้โดยผู้ใช้เดิมไม่พัง
func rolesFromClaims(claims jwt.MapClaims, adminIDs map[uuid.UUID]bool, userID uuid.UUID) []string {
	var roles []string
	if raw, ok := claims["roles"].([]interface{}); ok {
		for _, r := range raw {
			if s, ok := r.(string); ok && s != "" {
				roles = append(roles, s)
			}
		}
	}

	if adminIDs[userID] && !contains(roles, domain.RoleSuperAdmin) {
		roles = append(roles, domain.RoleSuperAdmin)
	}
	if len(roles) == 0 {
		roles = []string{domain.RoleUser}
	}
	return roles
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// bearerToken ดึง token ออกจาก Authorization header
// RFC 7235 บอกว่า scheme เป็น case-insensitive — ของเดิมเทียบแบบ case-sensitive
func bearerToken(header string) (string, bool) {
	const prefix = "bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	return token, token != ""
}
