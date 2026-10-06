package domain

import (
	"time"

	"example.com/paleo-prep-station/internal/contracts"
)

type StaffRole string

const (
	RoleRegistrar  StaffRole = "registrar"
	RolePrepLead   StaffRole = "prep_lead"
	RolePreparator StaffRole = "preparator"
	RoleCollection StaffRole = "collection_manager"
)

// MaxActiveAssignments limits how many unfinished plans one preparator can hold.
const MaxActiveAssignments = 3

// StaffMember is a person known to the station and the roles they may act in.
type StaffMember struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Roles     []StaffRole `json:"roles"`
	CreatedAt time.Time   `json:"created_at"`
}

func NewStaffMember(id string, name string, roles []StaffRole, now time.Time) (StaffMember, error) {
	if id == "" {
		return StaffMember{}, contracts.Invalid("required_field", "人员编号不能为空")
	}
	if !catalogPattern.MatchString(id) {
		return StaffMember{}, contracts.Invalid(
			"invalid_staff_id",
			"人员编号只能包含字母、数字、点、下划线和连字符",
		)
	}
	memberName, err := requiredText("人员姓名", name, 80)
	if err != nil {
		return StaffMember{}, err
	}
	if len(roles) == 0 {
		return StaffMember{}, contracts.Invalid("required_role", "人员至少需要一个角色")
	}
	seen := make(map[StaffRole]bool, len(roles))
	cleanedRoles := make([]StaffRole, 0, len(roles))
	for _, role := range roles {
		if !role.Known() {
			return StaffMember{}, contracts.Invalid("unknown_role", "未知人员角色: "+string(role))
		}
		if seen[role] {
			continue
		}
		seen[role] = true
		cleanedRoles = append(cleanedRoles, role)
	}
	return StaffMember{
		ID:        id,
		Name:      memberName,
		Roles:     cleanedRoles,
		CreatedAt: now.UTC(),
	}, nil
}

func (r StaffRole) Known() bool {
	switch r {
	case RoleRegistrar, RolePrepLead, RolePreparator, RoleCollection:
		return true
	default:
		return false
	}
}

func (s StaffMember) HasRole(role StaffRole) bool {
	for _, candidate := range s.Roles {
		if candidate == role {
			return true
		}
	}
	return false
}

// CanReceivePlan reports whether the member may hold responsibility for a plan.
func (s StaffMember) CanReceivePlan() bool {
	return s.HasRole(RolePreparator)
}
