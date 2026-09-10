package helpers

import (
	"github.com/google/uuid"

	"crm-auth-service/models"
)

// DataScope represents the resolved row-level authorization boundaries for the current user.
type DataScope struct {
	UserID         uuid.UUID
	Role           string
	IsUnrestricted bool // true for admin and leader for sales data
	AllowedUserIDs []uuid.UUID
}

// CanAccessLead checks if the given lead can be read or modified by the caller.
func (s DataScope) CanAccessLead(lead *models.Lead) bool {
	if s.IsUnrestricted {
		return true
	}
	if lead == nil {
		return false
	}
	for _, id := range s.AllowedUserIDs {
		if lead.AssignedTo != nil && *lead.AssignedTo == id {
			return true
		}
		if lead.CreatedBy != nil && *lead.CreatedBy == id {
			return true
		}
	}
	return false
}

// CanAssignLead checks whether the caller can assign/reassign a lead to targetUserID.
func (s DataScope) CanAssignLead(targetUserID uuid.UUID) bool {
	if s.IsUnrestricted {
		return true
	}
	for _, id := range s.AllowedUserIDs {
		if id == targetUserID {
			return true
		}
	}
	return false
}

// UUIDPtrToStringPtr converts a *uuid.UUID to a *string.
func UUIDPtrToStringPtr(u *uuid.UUID) *string {
	if u == nil {
		return nil
	}
	s := u.String()
	return &s
}

