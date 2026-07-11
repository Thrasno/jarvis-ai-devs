package model

// ChangePasswordRequest is the self-service password-change request. Identity
// fields are accepted for forward-compatible clients but are never authoritative.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
	UserID          string `json:"user_id,omitempty"`
	Username        string `json:"username,omitempty"`
	Email           string `json:"email,omitempty"`
}
