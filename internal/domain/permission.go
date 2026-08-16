package domain

// PetPermission is master data for what a caregiver can do.
type PetPermission struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsActive    bool   `json:"isActive"`
}
