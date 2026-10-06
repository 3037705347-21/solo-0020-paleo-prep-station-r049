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
	return &Service{
		repo: repo,
		now:  func() time.Time { return time.Now().UTC() },
	}
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
	Plan              domain.PrepPlan        `json:"plan"`
	Progress          []domain.ProgressEntry `json:"progress"`
	CurrentAssignment *domain.PlanAssignment `json:"current_assignment,omitempty"`
}

// WorkbenchItem shows who currently owns each open plan so responsibility is
// visible without opening the plan history.
type WorkbenchItem struct {
	PlanID            string                 `json:"plan_id"`
	SpecimenID        string                 `json:"specimen_id"`
	PlanStatus        domain.PlanStatus      `json:"plan_status"`
	CurrentAssignment *domain.PlanAssignment `json:"current_assignment,omitempty"`
}

// StaffWorkload reports a preparator's current load against the claim limit.
type StaffWorkload struct {
	Member        domain.StaffMember `json:"member"`
	OpenClaims    int                `json:"open_claims"`
	AssignmentCap int                `json:"assignment_cap"`
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
