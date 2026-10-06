package domain

import (
	"time"

	"example.com/paleo-prep-station/internal/contracts"
)

// AssignmentKind describes how a responsibility row came into existence.
type AssignmentKind string

const (
	AssignmentClaim    AssignmentKind = "claim"
	AssignmentTransfer AssignmentKind = "transfer"
)

// AssignmentState marks whether a responsibility row currently binds the
// preparator to the plan. Exactly one active row exists per assigned plan.
type AssignmentState string

const (
	AssignmentActive   AssignmentState = "active"
	AssignmentReleased AssignmentState = "released"
)

// PlanAssignment is one row in the responsibility ledger. A transfer releases
// the previous row and appends a new row in the same commit, so history is
// preserved while only the active row confers progress rights.
type PlanAssignment struct {
	ID           string          `json:"id"`
	PlanID       string          `json:"plan_id"`
	Sequence     int             `json:"sequence"`
	Kind         AssignmentKind  `json:"kind"`
	PreparatorID string          `json:"preparator_id"`
	AssignedBy   string          `json:"assigned_by"`
	State        AssignmentState `json:"state"`
	AssignedAt   time.Time       `json:"assigned_at"`
	ReleasedAt   *time.Time      `json:"released_at,omitempty"`
}

// ClaimPlanAssignment validates a first assignment of an approved, still
// unassigned plan. Draft, closed or already held plans are rejected.
func ClaimPlanAssignment(
	id string,
	plan PrepPlan,
	history []PlanAssignment,
	preparatorID string,
	assignedBy string,
	now time.Time,
) (PlanAssignment, error) {
	if id == "" {
		return PlanAssignment{}, contracts.Internal("missing_generated_id", "责任标识未生成", nil)
	}
	if plan.Status != PlanApproved && plan.Status != PlanActive {
		return PlanAssignment{}, contracts.Conflict("plan_not_assignable", "只有已审批且未完成的方案可以领用")
	}
	if active := FindActiveAssignment(history); active != nil {
		return PlanAssignment{}, contracts.Conflict("plan_already_claimed", "方案已被领用，不能重复领用")
	}
	preparator, err := requiredText("制备人员", preparatorID, 64)
	if err != nil {
		return PlanAssignment{}, err
	}
	leader, err := requiredText("分配负责人", assignedBy, 64)
	if err != nil {
		return PlanAssignment{}, err
	}
	return PlanAssignment{
		ID:           id,
		PlanID:       plan.ID,
		Sequence:     len(history) + 1,
		Kind:         AssignmentClaim,
		PreparatorID: preparator,
		AssignedBy:   leader,
		State:        AssignmentActive,
		AssignedAt:   now.UTC(),
	}, nil
}

// TransferPlanAssignment validates handing a held plan to another preparator.
// The returned released row is a mutated copy of the current active row; the
// caller must persist both the release and the new active row atomically.
func TransferPlanAssignment(
	id string,
	plan PrepPlan,
	history []PlanAssignment,
	targetPreparatorID string,
	assignedBy string,
	now time.Time,
) (released PlanAssignment, claimed PlanAssignment, err error) {
	if id == "" {
		return PlanAssignment{}, PlanAssignment{}, contracts.Internal("missing_generated_id", "责任标识未生成", nil)
	}
	if plan.Status == PlanCompleted || plan.Status == PlanCancelled {
		return PlanAssignment{}, PlanAssignment{}, contracts.Conflict("plan_not_transferable", "方案已结束，不能继续转派")
	}
	if plan.Status != PlanApproved && plan.Status != PlanActive {
		return PlanAssignment{}, PlanAssignment{}, contracts.Conflict("plan_not_transferable", "只有已领用的方案可以转派")
	}
	active := FindActiveAssignment(history)
	if active == nil {
		return PlanAssignment{}, PlanAssignment{}, contracts.Conflict("plan_not_claimed", "方案尚未领用，不能转派")
	}
	target, textErr := requiredText("目标制备人员", targetPreparatorID, 64)
	if textErr != nil {
		return PlanAssignment{}, PlanAssignment{}, textErr
	}
	if target == active.PreparatorID {
		return PlanAssignment{}, PlanAssignment{}, contracts.Conflict("assignment_unchanged", "目标责任人与当前责任人相同，无需转派")
	}
	leader, textErr := requiredText("转派负责人", assignedBy, 64)
	if textErr != nil {
		return PlanAssignment{}, PlanAssignment{}, textErr
	}
	stamp := now.UTC()
	releasedRow := *active
	releasedRow.State = AssignmentReleased
	releasedRow.ReleasedAt = &stamp
	newRow := PlanAssignment{
		ID:           id,
		PlanID:       plan.ID,
		Sequence:     len(history) + 1,
		Kind:         AssignmentTransfer,
		PreparatorID: target,
		AssignedBy:   leader,
		State:        AssignmentActive,
		AssignedAt:   stamp,
	}
	return releasedRow, newRow, nil
}

// FindActiveAssignment returns the row currently binding a preparator to the
// plan, or nil when the plan is unassigned or fully released.
func FindActiveAssignment(history []PlanAssignment) *PlanAssignment {
	for i := range history {
		if history[i].State == AssignmentActive {
			return &history[i]
		}
	}
	return nil
}
