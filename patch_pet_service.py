import re

with open("main.go", "r") as f:
    code = f.read()

# Add imports
imports_to_add = """
	"crypto/rsa"
	"strings"
	"github.com/golang-jwt/jwt/v5"
"""
code = code.replace('"github.com/google/uuid"', '"github.com/google/uuid"\n' + imports_to_add)

# Add OwnerID to Pet
code = code.replace(
"""	// Relationships
	Caregivers       []PetCaregiver `gorm:"foreignKey:PetID;constraint:OnDelete:CASCADE;" json:"caregivers"`""",
"""	OwnerID          uuid.UUID      `gorm:"type:uuid;not null;index" json:"ownerId"`
	// Relationships
	Caregivers       []PetCaregiver `gorm:"foreignKey:PetID;constraint:OnDelete:CASCADE;" json:"caregivers"`""")


# Add JWT middleware logic
jwt_middleware = """
var parsedPublicKey *rsa.PublicKey

func initKeys() {
	publicKeyBytes, err := os.ReadFile("keys/public.pem")
	if err != nil {
		log.Println("Warning: Failed to read public key:", err)
		return
	}
	parsedPublicKey, err = jwt.ParseRSAPublicKeyFromPEM(publicKeyBytes)
	if err != nil {
		log.Println("Warning: Failed to parse public key:", err)
	}
}

func authMiddleware(c *fiber.Ctx) error {
	authHeader := c.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return sendError(c, 401, "Missing or invalid token")
	}
	tokenString := authHeader[7:]

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return parsedPublicKey, nil
	})

	if err != nil || !token.Valid {
		return sendError(c, 401, "Invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return sendError(c, 401, "Invalid token claims")
	}

	userIDStr := claims["sub"].(string)
	c.Locals("userId", userIDStr)
	return c.Next()
}
"""

code = code.replace("func main() {", jwt_middleware + "\nfunc main() {\n\tinitKeys()")

# Replace getPets to filter by owner OR caregiver
get_pets_new = """func getPets(c *fiber.Ctx) error {
	userIDStr := c.Locals("userId").(string)
	var pets []Pet
	
	// Fetch pets where user is Owner OR user is in Caregivers
	DB.Preload("Caregivers.Permissions").
		Where("owner_id = ?", userIDStr).
		Or("id IN (SELECT pet_id FROM pet_caregivers WHERE user_id = ?)", userIDStr).
		Find(&pets)
		
	return c.JSON(pets)
}"""
code = re.sub(r'func getPets\(c \*fiber\.Ctx\) error \{[\s\S]*?return c\.JSON\(pets\)\n\}', get_pets_new, code)

# Set OwnerID in createPet
create_pet_new = """func createPet(c *fiber.Ctx) error {
	userIDStr := c.Locals("userId").(string)
	userID, _ := uuid.Parse(userIDStr)
	
	var pet Pet
	if err := c.BodyParser(&pet); err != nil {
		return sendError(c, 400, "Cannot parse JSON")
	}
	
	if pet.ID == uuid.Nil {
		pet.ID = uuid.New()
	}
	pet.OwnerID = userID
	pet.CreatedBy = &userIDStr

	DB.Create(&pet)
	return c.Status(201).JSON(pet)
}"""
code = re.sub(r'func createPet\(c \*fiber\.Ctx\) error \{[\s\S]*?return c\.Status\(201\)\.JSON\(pet\)\n\}', create_pet_new, code)

# Apply middleware in main
main_api = """	api := app.Group("/api/v1", func(c *fiber.Ctx) error {
		reqID := c.Get("X-Request-Id")
		if reqID == "" {
			reqID = uuid.New().String()
			c.Request().Header.Set("X-Request-Id", reqID)
		}
		c.Locals("X-Request-Id", reqID)
		return c.Next()
	})
	
	api.Use(authMiddleware)"""
code = code.replace("""	api := app.Group("/api/v1", func(c *fiber.Ctx) error {
		reqID := c.Get("X-Request-Id")
		if reqID == "" {
			reqID = uuid.New().String()
			c.Request().Header.Set("X-Request-Id", reqID)
		}
		c.Locals("X-Request-Id", reqID)
		return c.Next()
	})""", main_api)

with open("main.go", "w") as f:
    f.write(code)

print("Patch applied successfully")
