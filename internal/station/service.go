package station

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync/atomic"
	"time"

	"example.com/paleo-prep-station/internal/domain"
	"example.com/paleo-prep-station/internal/repository"
)

type Service struct {
	repo *repository.MemoryRepository
	now  func() time.Time
	seq  atomic.Uint64
}

func New(repo *repository.MemoryRepository) *Service {
	if repo == nil {
		repo = repository.NewMemory()
	}
	service := &Service{
		repo: repo,
		now:  func() time.Time { return time.Now().UTC() },
	}
	service.seedStaff()
	return service
}

// seedStaff installs the baseline roster so role checks and assignment work on a
// fresh in-memory repository. It never replaces a member that already exists.
func (s *Service) seedStaff() {
	roster := []struct {
		id    string
		name  string
		roles []domain.StaffRole
	}{
		{"registrar-wu", "登记员吴敏", []domain.StaffRole{domain.RoleRegistrar}},
		{"curator-lin", "制备负责人林岚", []domain.StaffRole{domain.RolePrepLead}},
		{"preparator-chen", "制备员陈晨", []domain.StaffRole{domain.RolePreparator}},
		{"preparator-zhao", "制备员赵岩", []domain.StaffRole{domain.RolePreparator}},
		{"preparator-sun", "制备员孙砾", []domain.StaffRole{domain.RolePreparator}},
		{"keeper-zhou", "标本管理员周衡", []domain.StaffRole{domain.RoleCollection}},
	}
	_ = s.repo.Mutate(func(m *repository.Mutation) error {
		now := s.now()
		for _, member := range roster {
			if _, ok := m.GetStaff(member.id); ok {
				continue
			}
			staff, err := domain.NewStaffMember(member.id, member.name, member.roles, now)
			if err != nil {
				return err
			}
			m.PutStaff(staff)
		}
		return nil
	})
}

func (s *Service) nextID(prefix string) string {
	var random [6]byte
	if _, err := rand.Read(random[:]); err == nil {
		return prefix + "-" + hex.EncodeToString(random[:])
	}
	local := s.seq.Add(1)
	return prefix + "-" + time.Now().UTC().Format("20060102150405") + "-" + hex.EncodeToString([]byte{byte(local)})
}

type PlanSnapshot struct {
	Plan        domain.PrepPlan         `json:"plan"`
	Progress    []domain.ProgressEntry  `json:"progress"`
	Assignment  *domain.PlanAssignment  `json:"assignment,omitempty"`
	Assignments []domain.PlanAssignment `json:"assignments"`
}

// StaffLoad is a directory entry together with the member's current workload.
type StaffLoad struct {
	domain.StaffMember
	ActiveAssignments int `json:"active_assignments"`
	AssignmentCap     int `json:"assignment_cap"`
}

func (s *Service) ListSpecimens(ctx context.Context) ([]domain.Specimen, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var values []domain.Specimen
	err := s.repo.Read(func(m *repository.Mutation) error {
		values = m.ListSpecimens()
		return nil
	})
	return values, err
}

func (s *Service) GetSpecimen(ctx context.Context, specimenID string) (domain.Specimen, error) {
	if err := ctx.Err(); err != nil {
		return domain.Specimen{}, err
	}
	var value domain.Specimen
	err := s.repo.Read(func(m *repository.Mutation) error {
		found, ok := m.GetSpecimen(specimenID)
		if !ok {
			return notFoundSpecimen(specimenID)
		}
		value = found
		return nil
	})
	return value, err
}
