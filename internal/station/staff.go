package station

import (
	"context"

	"example.com/paleo-prep-station/internal/contracts"
	"example.com/paleo-prep-station/internal/domain"
	"example.com/paleo-prep-station/internal/repository"
)

func (s *Service) RegisterStaff(
	ctx context.Context,
	request contracts.RegisterStaffRequest,
) (domain.StaffMember, error) {
	if err := ctx.Err(); err != nil {
		return domain.StaffMember{}, err
	}

	var created domain.StaffMember
	err := s.repo.Mutate(func(m *repository.Mutation) error {
		member, err := domain.NewStaffMember(request.ID, request.Name, request.Role, s.now())
		if err != nil {
			return err
		}
		if _, exists := m.GetStaff(member.ID); exists {
			return contracts.Conflict("staff_exists", "人员标识已存在，不能重复登记")
		}
		m.PutStaff(member)
		created = member
		return nil
	})
	return created, err
}

func (s *Service) ListStaff(ctx context.Context) ([]domain.StaffMember, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var values []domain.StaffMember
	err := s.repo.Read(func(m *repository.Mutation) error {
		values = m.ListStaff()
		return nil
	})
	return values, err
}

func (s *Service) GetStaffWorkload(ctx context.Context, staffID string) (StaffWorkload, error) {
	if err := ctx.Err(); err != nil {
		return StaffWorkload{}, err
	}
	var workload StaffWorkload
	err := s.repo.Read(func(m *repository.Mutation) error {
		member, ok := m.GetStaff(staffID)
		if !ok {
			return notFoundStaff(staffID)
		}
		workload = StaffWorkload{
			Member:        member,
			OpenClaims:    m.CountOpenAssignments(staffID),
			AssignmentCap: domain.PreparatorAssignmentLimit,
		}
		return nil
	})
	return workload, err
}

func (s *Service) GetWorkbench(ctx context.Context) ([]WorkbenchItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var items []WorkbenchItem
	err := s.repo.Read(func(m *repository.Mutation) error {
		plans := m.ListOpenPlans()
		items = make([]WorkbenchItem, 0, len(plans))
		for _, plan := range plans {
			items = append(items, WorkbenchItem{
				PlanID:            plan.ID,
				SpecimenID:        plan.SpecimenID,
				PlanStatus:        plan.Status,
				CurrentAssignment: domain.FindActiveAssignment(m.AssignmentsForPlan(plan.ID)),
			})
		}
		return nil
	})
	return items, err
}
