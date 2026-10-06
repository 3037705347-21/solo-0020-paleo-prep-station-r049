package station

import (
	"context"

	"example.com/paleo-prep-station/internal/contracts"
	"example.com/paleo-prep-station/internal/domain"
	"example.com/paleo-prep-station/internal/repository"
)

// requirePrepLead resolves the assigning actor and guarantees they hold the
// preparation lead role. Unknown actors are rejected the same way as actors
// with the wrong role so the outcome stays deterministic.
func requirePrepLead(m *repository.Mutation, actorID string) error {
	actor, ok := m.GetStaff(actorID)
	if !ok || actor.Role != domain.StaffRolePrepLead || !actor.Active {
		return contracts.Conflict("actor_not_prep_lead", "只有制备负责人可以分配或转派方案")
	}
	return nil
}

// requireActivePreparator resolves the target and guarantees they are an
// active preparator; membership and role are the qualification for holding a
// critical task.
func requireActivePreparator(m *repository.Mutation, preparatorID string) error {
	member, ok := m.GetStaff(preparatorID)
	if !ok || member.Role != domain.StaffRolePreparator || !member.Active {
		return contracts.Conflict("preparator_not_qualified", "目标人员不具备制备人员资格")
	}
	return nil
}

func (s *Service) ClaimAssignment(
	ctx context.Context,
	planID string,
	request contracts.ClaimAssignmentRequest,
) (domain.PlanAssignment, error) {
	if err := ctx.Err(); err != nil {
		return domain.PlanAssignment{}, err
	}

	var created domain.PlanAssignment
	err := s.repo.Mutate(func(m *repository.Mutation) error {
		plan, ok := m.GetPlan(planID)
		if !ok {
			return notFoundPlan(planID)
		}
		if err := requirePrepLead(m, request.AssignedBy); err != nil {
			return err
		}
		if err := requireActivePreparator(m, request.PreparatorID); err != nil {
			return err
		}
		history := m.AssignmentsForPlan(planID)
		assignment, err := domain.ClaimPlanAssignment(
			s.nextID("assignment"),
			plan,
			history,
			request.PreparatorID,
			request.AssignedBy,
			s.now(),
		)
		if err != nil {
			return err
		}
		if m.CountOpenAssignments(request.PreparatorID) >= domain.PreparatorAssignmentLimit {
			return contracts.Conflict("assignment_limit_reached", "该制备人员的领用数已达上限")
		}
		m.PutAssignment(assignment)
		created = assignment
		return nil
	})
	return created, err
}

func (s *Service) TransferAssignment(
	ctx context.Context,
	planID string,
	request contracts.TransferAssignmentRequest,
) (domain.PlanAssignment, error) {
	if err := ctx.Err(); err != nil {
		return domain.PlanAssignment{}, err
	}

	var created domain.PlanAssignment
	err := s.repo.Mutate(func(m *repository.Mutation) error {
		plan, ok := m.GetPlan(planID)
		if !ok {
			return notFoundPlan(planID)
		}
		if err := requirePrepLead(m, request.AssignedBy); err != nil {
			return err
		}
		if err := requireActivePreparator(m, request.TargetPreparatorID); err != nil {
			return err
		}
		history := m.AssignmentsForPlan(planID)
		released, claimed, err := domain.TransferPlanAssignment(
			s.nextID("assignment"),
			plan,
			history,
			request.TargetPreparatorID,
			request.AssignedBy,
			s.now(),
		)
		if err != nil {
			return err
		}
		// Reject when the target's other open critical tasks already fill the
		// cap; the current plan is held by someone else so it is not counted.
		if m.CountOpenAssignments(request.TargetPreparatorID) >= domain.PreparatorAssignmentLimit {
			return contracts.Conflict("assignment_limit_reached", "目标制备人员的领用数已达上限")
		}
		// Release the old holder and establish the new holder in this single
		// commit; a failure before commit leaves neither half applied.
		m.PutAssignment(released)
		m.PutAssignment(claimed)
		created = claimed
		return nil
	})
	return created, err
}

func (s *Service) ListAssignments(
	ctx context.Context,
	planID string,
) ([]domain.PlanAssignment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var values []domain.PlanAssignment
	err := s.repo.Read(func(m *repository.Mutation) error {
		if _, ok := m.GetPlan(planID); !ok {
			return notFoundPlan(planID)
		}
		values = m.AssignmentsForPlan(planID)
		return nil
	})
	return values, err
}
