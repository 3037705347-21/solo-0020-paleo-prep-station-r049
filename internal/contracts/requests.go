package contracts

type RegisterSpecimenRequest struct {
	CatalogNumber string `json:"catalog_number"`
	FieldLabel    string `json:"field_label"`
	TaxonHint     string `json:"taxon_hint"`
	MatrixKind    string `json:"matrix_kind"`
	JacketNotes   string `json:"jacket_notes"`
}

type CreatePlanRequest struct {
	Objective      string `json:"objective"`
	Method         string `json:"method"`
	Adhesive       string `json:"adhesive"`
	TargetExposure int    `json:"target_exposure"`
}

type ApprovalRequest struct {
	ApprovedBy string `json:"approved_by"`
}

type ProgressRequest struct {
	Stage      string `json:"stage"`
	Percent    int    `json:"percent"`
	Note       string `json:"note"`
	RecordedBy string `json:"recorded_by"`
}

// ClaimPlanRequest assigns an approved unassigned plan. When AssignedBy is empty
// ActorID claims the plan for themselves; otherwise ActorID must be a prep lead
// directing the plan to AssigneeID.
type ClaimPlanRequest struct {
	ActorID    string `json:"actor_id"`
	AssigneeID string `json:"assignee_id"`
	AssignedBy string `json:"assigned_by,omitempty"`
}

// ReassignPlanRequest moves a held plan. FromID must be the current holder; the
// release and the new responsibility commit together.
type ReassignPlanRequest struct {
	FromID string `json:"from_id"`
	ToID   string `json:"to_id"`
}

type ReleaseRequest struct {
	Purpose    string `json:"purpose"`
	Recipient  string `json:"recipient"`
	ApprovedBy string `json:"approved_by"`
}
