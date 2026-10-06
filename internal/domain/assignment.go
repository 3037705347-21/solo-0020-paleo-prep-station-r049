package domain

import (
	"time"

	"example.com/paleo-prep-station/internal/contracts"
)

type AssignmentKind string

const (
	AssignmentClaim    AssignmentKind = "claim"
	AssignmentDirected AssignmentKind = "directed"
	AssignmentTransfer AssignmentKind = "transfer"
)

type AssignmentStatus string

const (
	AssignmentActive    AssignmentStatus = "active"
	AssignmentReleased  AssignmentStatus = "released"
	AssignmentCompleted AssignmentStatus = "completed"
)

// PlanAssignment records one period of responsibility over a plan. Only a single
// record per plan may be active; transfer releases the old one and opens a new
// one inside the same commit.
type PlanAssignment struct {
	ID           string           `json:"id"`
	PlanID       string           `json:"plan_id"`
	AssigneeID   string           `json:"assignee_id"`
	GrantedBy    string           `json:"granted_by"`
	Kind         AssignmentKind   `json:"kind"`
	Status       AssignmentStatus `json:"status"`
	Sequence     int              `json:"sequence"`
	GrantedAt    time.Time        `json:"granted_at"`
	ReleasedAt   *time.Time       `json:"released_at,omitempty"`
	ReleasedBy   string           `json:"released_by,omitempty"`
	ReleaseCause string           `json:"release_cause,omitempty"`
}

func NewPlanAssignment(
	id string,
	plan PrepPlan,
	sequence int,
	assigneeID string,
	grantedBy string,
	kind AssignmentKind,
	now time.Time,
) (PlanAssignment, error) {
	if id == "" {
		return PlanAssignment{}, contracts.Internal("missing_generated_id", "领用标识未生成", nil)
	}
	holder, err := requiredText("责任人", assigneeID, 80)
	if err != nil {
		return PlanAssignment{}, err
	}
	grantor, err := requiredText("发派人", grantedBy, 80)
	if err != nil {
		return PlanAssignment{}, err
	}
	switch kind {
	case AssignmentClaim, AssignmentDirected, AssignmentTransfer:
	default:
		return PlanAssignment{}, contracts.Invalid("unknown_assignment_kind", "未知的领用方式")
	}
	if sequence < 1 {
		return PlanAssignment{}, contracts.Internal("invalid_assignment_sequence", "领用序号必须从一开始", nil)
	}
	stamp := now.UTC()
	return PlanAssignment{
		ID:         id,
		PlanID:     plan.ID,
		AssigneeID: holder,
		GrantedBy:  grantor,
		Kind:       kind,
		Status:     AssignmentActive,
		Sequence:   sequence,
		GrantedAt:  stamp,
	}, nil
}

// Release marks the responsibility ended by a transfer. Only an active record can
// be released; the successor record is committed together with this change.
func (a *PlanAssignment) Release(releasedBy string, cause string, now time.Time) error {
	if a.Status != AssignmentActive {
		return contracts.Conflict("assignment_not_active", "只有当前责任可以释放")
	}
	actor, err := requiredText("转派人", releasedBy, 80)
	if err != nil {
		return err
	}
	stamp := now.UTC()
	a.Status = AssignmentReleased
	a.ReleasedAt = &stamp
	a.ReleasedBy = actor
	a.ReleaseCause = cause
	return nil
}

// Complete closes the responsibility because the plan reached one hundred percent.
func (a *PlanAssignment) Complete(now time.Time) error {
	if a.Status != AssignmentActive {
		return contracts.Conflict("assignment_not_active", "只有当前责任可以完成")
	}
	stamp := now.UTC()
	a.Status = AssignmentCompleted
	a.ReleasedAt = &stamp
	a.ReleaseCause = string(AssignmentCompleted)
	return nil
}

func (a PlanAssignment) IsActive() bool {
	return a.Status == AssignmentActive
}
