package domain

import (
	"time"

	"example.com/paleo-prep-station/internal/contracts"
)

// PreparatorAssignmentLimit is the maximum number of open critical tasks a
// single preparator can be responsible for at the same time. Finishing a plan
// releases the slot even though the last holder remains recorded.
const PreparatorAssignmentLimit = 3

type StaffRole string

const (
	StaffRolePrepLead   StaffRole = "prep_lead"
	StaffRolePreparator StaffRole = "preparator"
)

type StaffMember struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Role      StaffRole `json:"role"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

func NewStaffMember(id string, name string, role string, now time.Time) (StaffMember, error) {
	identifier, err := requiredText("人员标识", id, 64)
	if err != nil {
		return StaffMember{}, err
	}
	displayName, err := requiredText("人员姓名", name, 80)
	if err != nil {
		return StaffMember{}, err
	}
	staffRole := StaffRole(role)
	switch staffRole {
	case StaffRolePrepLead, StaffRolePreparator:
	default:
		return StaffMember{}, contracts.Invalid("invalid_staff_role", "人员角色必须是 prep_lead 或 preparator")
	}
	return StaffMember{
		ID:        identifier,
		Name:      displayName,
		Role:      staffRole,
		Active:    true,
		CreatedAt: now.UTC(),
	}, nil
}
