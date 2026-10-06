package station

import (
	"context"

	"example.com/paleo-prep-station/internal/domain"
	"example.com/paleo-prep-station/internal/repository"
)

// ListStaff returns the roster with each member's open assignment count and cap,
// so the workbench can show who may take more critical work.
func (s *Service) ListStaff(ctx context.Context) ([]StaffLoad, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var loads []StaffLoad
	err := s.repo.Read(func(m *repository.Mutation) error {
		members := m.ListStaff()
		loads = make([]StaffLoad, 0, len(members))
		for _, member := range members {
			cap := 0
			if member.CanReceivePlan() {
				cap = domain.MaxActiveAssignments
			}
			loads = append(loads, StaffLoad{
				StaffMember:       member,
				ActiveAssignments: m.CountActiveAssignments(member.ID),
				AssignmentCap:     cap,
			})
		}
		return nil
	})
	return loads, err
}

// loadStaff resolves a member or returns a stable not-found error for unknown actors.
func loadStaff(m *repository.Mutation, id string) (domain.StaffMember, error) {
	member, ok := m.GetStaff(id)
	if !ok {
		return domain.StaffMember{}, notFoundStaff(id)
	}
	return member, nil
}
