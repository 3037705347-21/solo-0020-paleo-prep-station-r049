package repository

import (
	"sort"
	"sync"

	"example.com/paleo-prep-station/internal/domain"
)

type state struct {
	specimens   map[string]domain.Specimen
	plans       map[string]domain.PrepPlan
	progress    map[string][]domain.ProgressEntry
	releases    map[string]domain.ReleaseRecord
	staff       map[string]domain.StaffMember
	assignments map[string][]domain.PlanAssignment
}

type MemoryRepository struct {
	mu    sync.Mutex
	state state
}

func NewMemory() *MemoryRepository {
	return &MemoryRepository{
		state: state{
			specimens:   make(map[string]domain.Specimen),
			plans:       make(map[string]domain.PrepPlan),
			progress:    make(map[string][]domain.ProgressEntry),
			releases:    make(map[string]domain.ReleaseRecord),
			staff:       make(map[string]domain.StaffMember),
			assignments: make(map[string][]domain.PlanAssignment),
		},
	}
}

func (r *MemoryRepository) Read(fn func(*Mutation) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return fn(cloneMutation(r.state))
}

func (r *MemoryRepository) Mutate(fn func(*Mutation) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	tx := cloneMutation(r.state)
	if err := fn(tx); err != nil {
		return err
	}
	r.state = tx.snapshot()
	return nil
}

func cloneMutation(source state) *Mutation {
	copied := state{
		specimens:   make(map[string]domain.Specimen, len(source.specimens)),
		plans:       make(map[string]domain.PrepPlan, len(source.plans)),
		progress:    make(map[string][]domain.ProgressEntry, len(source.progress)),
		releases:    make(map[string]domain.ReleaseRecord, len(source.releases)),
		staff:       make(map[string]domain.StaffMember, len(source.staff)),
		assignments: make(map[string][]domain.PlanAssignment, len(source.assignments)),
	}
	for key, value := range source.specimens {
		copied.specimens[key] = value
	}
	for key, value := range source.plans {
		copied.plans[key] = value
	}
	for key, values := range source.progress {
		copied.progress[key] = append([]domain.ProgressEntry(nil), values...)
	}
	for key, value := range source.releases {
		copied.releases[key] = value
	}
	for key, value := range source.staff {
		copied.staff[key] = value
	}
	for key, values := range source.assignments {
		copied.assignments[key] = append([]domain.PlanAssignment(nil), values...)
	}
	return &Mutation{state: copied}
}

type Mutation struct {
	state state
}

func (m *Mutation) snapshot() state {
	return m.state
}

func (m *Mutation) GetSpecimen(id string) (domain.Specimen, bool) {
	value, ok := m.state.specimens[id]
	return value, ok
}

func (m *Mutation) FindSpecimenByCatalog(catalogNumber string) (domain.Specimen, bool) {
	for _, value := range m.state.specimens {
		if value.CatalogNumber == catalogNumber {
			return value, true
		}
	}
	return domain.Specimen{}, false
}

func (m *Mutation) ListSpecimens() []domain.Specimen {
	values := make([]domain.Specimen, 0, len(m.state.specimens))
	for _, value := range m.state.specimens {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].CreatedAt.Equal(values[j].CreatedAt) {
			return values[i].ID < values[j].ID
		}
		return values[i].CreatedAt.Before(values[j].CreatedAt)
	})
	return values
}

func (m *Mutation) PutSpecimen(value domain.Specimen) {
	m.state.specimens[value.ID] = value
}

func (m *Mutation) GetPlan(id string) (domain.PrepPlan, bool) {
	value, ok := m.state.plans[id]
	return value, ok
}

func (m *Mutation) ListPlansForSpecimen(specimenID string) []domain.PrepPlan {
	values := make([]domain.PrepPlan, 0)
	for _, value := range m.state.plans {
		if value.SpecimenID == specimenID {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].CreatedAt.Equal(values[j].CreatedAt) {
			return values[i].ID < values[j].ID
		}
		return values[i].CreatedAt.Before(values[j].CreatedAt)
	})
	return values
}

func (m *Mutation) PutPlan(value domain.PrepPlan) {
	m.state.plans[value.ID] = value
}

func (m *Mutation) HasOpenPlan(specimenID string) bool {
	for _, value := range m.state.plans {
		if value.SpecimenID == specimenID && value.IsOpen() {
			return true
		}
	}
	return false
}

func (m *Mutation) ProgressForPlan(planID string) []domain.ProgressEntry {
	return append([]domain.ProgressEntry(nil), m.state.progress[planID]...)
}

func (m *Mutation) AppendProgress(value domain.ProgressEntry) {
	m.state.progress[value.PlanID] = append(m.state.progress[value.PlanID], value)
}

func (m *Mutation) ReleaseForSpecimen(specimenID string) (domain.ReleaseRecord, bool) {
	for _, value := range m.state.releases {
		if value.SpecimenID == specimenID {
			return value, true
		}
	}
	return domain.ReleaseRecord{}, false
}

func (m *Mutation) PutRelease(value domain.ReleaseRecord) {
	m.state.releases[value.ID] = value
}

func (m *Mutation) GetStaff(id string) (domain.StaffMember, bool) {
	value, ok := m.state.staff[id]
	return value, ok
}

func (m *Mutation) ListStaff() []domain.StaffMember {
	values := make([]domain.StaffMember, 0, len(m.state.staff))
	for _, value := range m.state.staff {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool {
		return values[i].ID < values[j].ID
	})
	return values
}

func (m *Mutation) PutStaff(value domain.StaffMember) {
	m.state.staff[value.ID] = value
}

func (m *Mutation) AssignmentsForPlan(planID string) []domain.PlanAssignment {
	return append([]domain.PlanAssignment(nil), m.state.assignments[planID]...)
}

func (m *Mutation) PutAssignment(value domain.PlanAssignment) {
	rows := m.state.assignments[value.PlanID]
	for i := range rows {
		if rows[i].ID == value.ID {
			rows[i] = value
			m.state.assignments[value.PlanID] = rows
			return
		}
	}
	m.state.assignments[value.PlanID] = append(rows, value)
}

// CountOpenAssignments counts active responsibility rows for a preparator that
// still bind an open plan. Released rows and rows on completed plans free the
// preparator's capacity.
func (m *Mutation) CountOpenAssignments(preparatorID string) int {
	count := 0
	for planID, rows := range m.state.assignments {
		if domain.FindActiveAssignment(rows) == nil {
			continue
		}
		plan, ok := m.state.plans[planID]
		if !ok || !plan.IsOpen() {
			continue
		}
		if active := domain.FindActiveAssignment(rows); active.PreparatorID == preparatorID {
			count++
		}
	}
	return count
}

// ListOpenPlans returns every plan that has not reached a terminal state,
// ordered by creation time then id to match the specimen listings.
func (m *Mutation) ListOpenPlans() []domain.PrepPlan {
	values := make([]domain.PrepPlan, 0)
	for _, value := range m.state.plans {
		if value.IsOpen() {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].CreatedAt.Equal(values[j].CreatedAt) {
			return values[i].ID < values[j].ID
		}
		return values[i].CreatedAt.Before(values[j].CreatedAt)
	})
	return values
}
