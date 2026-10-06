package station

import (
	"context"
	"strings"

	"example.com/paleo-prep-station/internal/contracts"
	"example.com/paleo-prep-station/internal/domain"
	"example.com/paleo-prep-station/internal/repository"
)

// ClaimPlan assigns an approved, unassigned plan. ActorID claims for themselves;
// a prep lead may instead direct the plan to another preparator via AssignedBy.
// The whole decision commits in one mutex-serialized mutation, so duplicate and
// concurrent claims have a deterministic single winner.
func (s *Service) ClaimPlan(
	ctx context.Context,
	planID string,
	request contracts.ClaimPlanRequest,
) (PlanSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return PlanSnapshot{}, err
	}

	var snapshot PlanSnapshot
	err := s.repo.Mutate(func(m *repository.Mutation) error {
		plan, ok := m.GetPlan(planID)
		if !ok {
			return notFoundPlan(planID)
		}
		actorID := strings.TrimSpace(request.ActorID)
		if actorID == "" {
			return contracts.Invalid("required_field", "领用人不能为空")
		}
		actor, err := loadStaff(m, actorID)
		if err != nil {
			return err
		}

		target := actor
		kind := domain.AssignmentClaim
		if strings.TrimSpace(request.AssignedBy) != "" {
			if !actor.HasRole(domain.RolePrepLead) {
				return contracts.Forbidden("actor_not_prep_lead", "只有制备负责人可以指派方案")
			}
			assigneeID := strings.TrimSpace(request.AssigneeID)
			if assigneeID == "" {
				return contracts.Invalid("required_field", "被指派人不能为空")
			}
			target, err = loadStaff(m, assigneeID)
			if err != nil {
				return err
			}
			kind = domain.AssignmentDirected
		}
		if !target.CanReceivePlan() {
			return contracts.Invalid("assignee_not_preparator", "责任人必须具备制备人员角色")
		}
		if m.CountActiveAssignments(target.ID) >= domain.MaxActiveAssignments {
			return contracts.Conflict("assignee_at_capacity", "责任人的领用数量已达上限")
		}

		trail := m.AssignmentsForPlan(planID)
		assignmentID := s.nextID("assign")
		assignment, err := domain.NewPlanAssignment(
			assignmentID,
			plan,
			len(trail)+1,
			target.ID,
			actor.ID,
			kind,
			s.now(),
		)
		if err != nil {
			return err
		}
		if err := plan.GrantResponsibility(target.ID, assignmentID, s.now()); err != nil {
			return err
		}
		m.AppendAssignment(assignment)
		m.PutPlan(plan)
		snapshot = buildPlanSnapshot(plan, m)
		return nil
	})
	return snapshot, err
}

// ReassignPlan atomically releases the current holder's responsibility and
// establishes the new holder's. A completed plan keeps its last holder and can no
// longer be transferred.
func (s *Service) ReassignPlan(
	ctx context.Context,
	planID string,
	request contracts.ReassignPlanRequest,
) (PlanSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return PlanSnapshot{}, err
	}

	var snapshot PlanSnapshot
	err := s.repo.Mutate(func(m *repository.Mutation) error {
		plan, ok := m.GetPlan(planID)
		if !ok {
			return notFoundPlan(planID)
		}
		if plan.Status != domain.PlanApproved && plan.Status != domain.PlanActive {
			return contracts.Conflict("plan_not_reassignable", "只有已领用且未完成的方案可以转派")
		}
		fromID := strings.TrimSpace(request.FromID)
		toID := strings.TrimSpace(request.ToID)
		if fromID == "" {
			return contracts.Invalid("required_field", "当前责任人不能为空")
		}
		if toID == "" {
			return contracts.Invalid("required_field", "新责任人不能为空")
		}
		if _, err := loadStaff(m, fromID); err != nil {
			return err
		}
		target, err := loadStaff(m, toID)
		if err != nil {
			return err
		}
		if !target.CanReceivePlan() {
			return contracts.Invalid("assignee_not_preparator", "新责任人必须具备制备人员角色")
		}

		active, hasActive := m.ActiveAssignmentForPlan(planID)
		if !hasActive {
			return contracts.Conflict("plan_not_assigned", "方案尚未领用，不能转派")
		}
		if active.AssigneeID != fromID {
			return contracts.Conflict("not_current_assignee", "只有当前责任人可以转派该方案")
		}
		// The plan being transferred does not count toward the successor's load;
		// it still belongs to the current holder until this commit succeeds.
		if m.CountActiveAssignments(target.ID) >= domain.MaxActiveAssignments {
			return contracts.Conflict("assignee_at_capacity", "新责任人的领用数量已达上限")
		}

		trail := m.AssignmentsForPlan(planID)
		successorID := s.nextID("assign")
		successor, err := domain.NewPlanAssignment(
			successorID,
			plan,
			len(trail)+1,
			target.ID,
			fromID,
			domain.AssignmentTransfer,
			s.now(),
		)
		if err != nil {
			return err
		}
		if err := active.Release(fromID, string(domain.AssignmentTransfer), s.now()); err != nil {
			return err
		}
		if err := plan.TransferResponsibility(fromID, target.ID, successorID, s.now()); err != nil {
			return err
		}
		m.PutAssignment(active)
		m.AppendAssignment(successor)
		m.PutPlan(plan)
		snapshot = buildPlanSnapshot(plan, m)
		return nil
	})
	return snapshot, err
}

// currentAssignment resolves the responsibility the plan currently points at. It
// is active for open plans and completed for finished plans; it is absent only
// while an approved plan has never been claimed.
func currentAssignment(plan domain.PrepPlan, m *repository.Mutation) (*domain.PlanAssignment, bool) {
	if plan.AssignmentID == "" {
		return nil, false
	}
	for _, candidate := range m.AssignmentsForPlan(plan.ID) {
		if candidate.ID == plan.AssignmentID {
			holder := candidate
			return &holder, true
		}
	}
	return nil, false
}

// buildPlanSnapshot projects the plan, its progress trail and responsibility trail.
func buildPlanSnapshot(plan domain.PrepPlan, m *repository.Mutation) PlanSnapshot {
	snapshot := PlanSnapshot{
		Plan:        plan,
		Progress:    m.ProgressForPlan(plan.ID),
		Assignments: m.AssignmentsForPlan(plan.ID),
	}
	if holder, ok := currentAssignment(plan, m); ok {
		snapshot.Assignment = holder
	}
	return snapshot
}
