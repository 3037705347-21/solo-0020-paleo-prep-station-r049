package domain

import (
	"time"

	"example.com/paleo-prep-station/internal/contracts"
)

type PlanStatus string

const (
	PlanDraft     PlanStatus = "draft"
	PlanApproved  PlanStatus = "approved"
	PlanActive    PlanStatus = "active"
	PlanCompleted PlanStatus = "completed"
	PlanCancelled PlanStatus = "cancelled"
)

type PrepPlan struct {
	ID             string     `json:"id"`
	SpecimenID     string     `json:"specimen_id"`
	Objective      string     `json:"objective"`
	Method         string     `json:"method"`
	Adhesive       string     `json:"adhesive"`
	TargetExposure int        `json:"target_exposure"`
	Status         PlanStatus `json:"status"`
	ApprovedBy     string     `json:"approved_by,omitempty"`
	ApprovedAt     *time.Time `json:"approved_at,omitempty"`
	AssigneeID     string     `json:"assignee_id,omitempty"`
	AssignmentID   string     `json:"assignment_id,omitempty"`
	Version        int        `json:"version"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func NewPrepPlan(
	id string,
	specimen Specimen,
	objective string,
	method string,
	adhesive string,
	targetExposure int,
	now time.Time,
) (PrepPlan, error) {
	if id == "" {
		return PrepPlan{}, contracts.Internal("missing_generated_id", "方案标识未生成", nil)
	}
	if !specimen.CanCreatePlan() {
		return PrepPlan{}, contracts.Conflict("specimen_not_plannable", "当前标本状态不能创建修复方案")
	}
	goal, err := requiredText("修复目标", objective, 200)
	if err != nil {
		return PrepPlan{}, err
	}
	approach, err := requiredText("修复方法", method, 160)
	if err != nil {
		return PrepPlan{}, err
	}
	adhesiveValue, err := optionalText("黏结材料", adhesive, 120)
	if err != nil {
		return PrepPlan{}, err
	}
	if err := validatePercent(targetExposure); err != nil {
		return PrepPlan{}, contracts.Invalid("invalid_target_exposure", "目标暴露比例必须位于 0 到 100 之间")
	}
	if targetExposure == 0 {
		return PrepPlan{}, contracts.Invalid("invalid_target_exposure", "目标暴露比例必须大于零")
	}
	return PrepPlan{
		ID:             id,
		SpecimenID:     specimen.ID,
		Objective:      goal,
		Method:         approach,
		Adhesive:       adhesiveValue,
		TargetExposure: targetExposure,
		Status:         PlanDraft,
		Version:        1,
		CreatedAt:      now.UTC(),
		UpdatedAt:      now.UTC(),
	}, nil
}

func (p PrepPlan) IsOpen() bool {
	return p.Status == PlanDraft || p.Status == PlanApproved || p.Status == PlanActive
}

func (p PrepPlan) IsCompleted() bool {
	return p.Status == PlanCompleted
}

// GrantResponsibility assigns an unclaimed approved or active plan to a preparator.
// Eligibility of the holder is enforced by the service against the staff directory.
func (p *PrepPlan) GrantResponsibility(assigneeID, assignmentID string, now time.Time) error {
	if p.Status != PlanApproved && p.Status != PlanActive {
		return contracts.Conflict("plan_not_claimable", "只有已审批且未完成的方案可以领用")
	}
	if p.AssigneeID != "" {
		return contracts.Conflict("plan_already_assigned", "方案已被领用，不能重复领用")
	}
	holder, err := requiredText("责任人", assigneeID, 80)
	if err != nil {
		return err
	}
	if assignmentID == "" {
		return contracts.Internal("missing_generated_id", "领用标识未生成", nil)
	}
	stamp := now.UTC()
	p.AssigneeID = holder
	p.AssignmentID = assignmentID
	p.Version++
	p.UpdatedAt = stamp
	return nil
}

// TransferResponsibility hands a held plan from its current holder to a new one.
// The old responsibility is released and the new one established by the caller in
// the same commit; same-holder transfers are rejected.
func (p *PrepPlan) TransferResponsibility(fromID, toID, assignmentID string, now time.Time) error {
	if p.Status == PlanCompleted || p.Status == PlanCancelled {
		return contracts.Conflict("plan_not_reassignable", "已结束的方案不能继续转派")
	}
	if p.Status != PlanApproved && p.Status != PlanActive {
		return contracts.Conflict("plan_not_reassignable", "只有已领用的方案可以转派")
	}
	if p.AssigneeID == "" {
		return contracts.Conflict("plan_not_assigned", "方案尚未领用，不能转派")
	}
	current, err := requiredText("当前责任人", fromID, 80)
	if err != nil {
		return err
	}
	if p.AssigneeID != current {
		return contracts.Conflict("not_current_assignee", "只有当前责任人可以转派该方案")
	}
	target, err := requiredText("新责任人", toID, 80)
	if err != nil {
		return err
	}
	if target == current {
		return contracts.Invalid("reassignment_same_assignee", "新责任人不能与当前责任人相同")
	}
	if assignmentID == "" {
		return contracts.Internal("missing_generated_id", "领用标识未生成", nil)
	}
	stamp := now.UTC()
	p.AssigneeID = target
	p.AssignmentID = assignmentID
	p.Version++
	p.UpdatedAt = stamp
	return nil
}

func (p *PrepPlan) Approve(approvedBy string, now time.Time) error {
	if p.Status != PlanDraft {
		return contracts.Conflict("plan_not_approvable", "只有草稿方案可以审批")
	}
	approver, err := requiredText("审批人", approvedBy, 80)
	if err != nil {
		return err
	}
	stamp := now.UTC()
	p.Status = PlanApproved
	p.ApprovedBy = approver
	p.ApprovedAt = &stamp
	p.Version++
	p.UpdatedAt = stamp
	return nil
}

func (p *PrepPlan) Begin(now time.Time) error {
	if p.Status == PlanActive {
		return nil
	}
	if p.Status != PlanApproved {
		return contracts.Conflict("plan_not_active", "只有已审批方案可以开始制备")
	}
	p.Status = PlanActive
	p.Version++
	p.UpdatedAt = now.UTC()
	return nil
}

func (p *PrepPlan) Complete(now time.Time) error {
	if p.Status == PlanCompleted {
		return nil
	}
	if p.Status != PlanActive {
		return contracts.Conflict("plan_not_completable", "只有进行中的方案可以完成")
	}
	p.Status = PlanCompleted
	p.Version++
	p.UpdatedAt = now.UTC()
	return nil
}

func (p *PrepPlan) Cancel(now time.Time) error {
	if p.Status != PlanDraft {
		return contracts.Conflict("plan_not_cancellable", "只有草稿方案可以取消")
	}
	p.Status = PlanCancelled
	p.Version++
	p.UpdatedAt = now.UTC()
	return nil
}
