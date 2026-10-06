package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"time"

	"example.com/paleo-prep-station/internal/contracts"
	"example.com/paleo-prep-station/internal/httpapi"
	"example.com/paleo-prep-station/internal/repository"
	"example.com/paleo-prep-station/internal/station"
)

type specimenView struct {
	ID            string `json:"id"`
	CatalogNumber string `json:"catalog_number"`
	Status        string `json:"status"`
}

type planView struct {
	ID         string `json:"id"`
	SpecimenID string `json:"specimen_id"`
	Status     string `json:"status"`
	AssigneeID string `json:"assignee_id"`
}

type assignmentView struct {
	ID         string `json:"id"`
	AssigneeID string `json:"assignee_id"`
	Status     string `json:"status"`
	Sequence   int    `json:"sequence"`
	ReleasedBy string `json:"released_by"`
}

type planSnapshot struct {
	Plan     planView `json:"plan"`
	Progress []struct {
		Percent int `json:"percent"`
	} `json:"progress"`
	Assignment  *assignmentView  `json:"assignment"`
	Assignments []assignmentView `json:"assignments"`
}

type staffView struct {
	ID                string `json:"id"`
	ActiveAssignments int    `json:"active_assignments"`
	AssignmentCap     int    `json:"assignment_cap"`
}

type releaseView struct {
	ID         string `json:"id"`
	SpecimenID string `json:"specimen_id"`
}

func main() {
	workflow := flag.String("workflow", "", "workflow id to check")
	flag.Parse()

	if err := run(*workflow); err != nil {
		fmt.Fprintln(os.Stderr, "workflow check failed:", err)
		os.Exit(1)
	}
	fmt.Printf("workflow check passed: %s\n", *workflow)
}

func run(workflow string) error {
	handler := httpapi.NewRouter(station.New(repository.NewMemory()))
	server := httptest.NewServer(handler)
	defer server.Close()
	client := &http.Client{Timeout: 5 * time.Second}

	switch workflow {
	case "register-specimen":
		specimen, err := registerSpecimen(client, server.URL, "PPS-2026-001")
		if err != nil {
			return err
		}
		if specimen.Status != "registered" {
			return fmt.Errorf("expected registered status, got %s", specimen.Status)
		}
		var fetched specimenView
		if err := requestJSON(client, http.MethodGet, server.URL+"/specimens/"+specimen.ID, nil, http.StatusOK, &fetched); err != nil {
			return err
		}
		if fetched.ID != specimen.ID {
			return errors.New("registered specimen cannot be read back")
		}
		return nil

	case "approve-prep-plan":
		specimen, err := registerSpecimen(client, server.URL, "PPS-2026-002")
		if err != nil {
			return err
		}
		plan, err := createPlan(client, server.URL, specimen.ID)
		if err != nil {
			return err
		}
		var approved planView
		if err := requestJSON(
			client,
			http.MethodPost,
			server.URL+"/plans/"+plan.ID+"/approval",
			contracts.ApprovalRequest{ApprovedBy: "curator-lin"},
			http.StatusOK,
			&approved,
		); err != nil {
			return err
		}
		if approved.Status != "approved" {
			return fmt.Errorf("expected approved plan, got %s", approved.Status)
		}
		var fetchedSpecimen specimenView
		if err := requestJSON(client, http.MethodGet, server.URL+"/specimens/"+specimen.ID, nil, http.StatusOK, &fetchedSpecimen); err != nil {
			return err
		}
		if fetchedSpecimen.Status != "preparing" {
			return fmt.Errorf("expected preparing specimen, got %s", fetchedSpecimen.Status)
		}
		return nil

	case "log-preparation":
		specimen, plan, err := claimedPlanSetup(client, server.URL, "PPS-2026-003", "preparator-chen")
		if err != nil {
			return err
		}
		if err := recordProgress(client, server.URL, plan.ID, 35, "矩阵剥离", "preparator-chen"); err != nil {
			return err
		}
		if err := recordProgress(client, server.URL, plan.ID, 100, "表面加固", "preparator-chen"); err != nil {
			return err
		}
		var snapshot planSnapshot
		if err := requestJSON(client, http.MethodGet, server.URL+"/plans/"+plan.ID, nil, http.StatusOK, &snapshot); err != nil {
			return err
		}
		if snapshot.Plan.Status != "completed" || len(snapshot.Progress) != 2 {
			return errors.New("plan did not complete with exactly two progress entries")
		}
		if snapshot.Assignment == nil || snapshot.Assignment.Status != "completed" {
			return errors.New("completed plan did not close the current responsibility")
		}
		var fetchedSpecimen specimenView
		if err := requestJSON(client, http.MethodGet, server.URL+"/specimens/"+specimen.ID, nil, http.StatusOK, &fetchedSpecimen); err != nil {
			return err
		}
		if fetchedSpecimen.Status != "stabilized" {
			return fmt.Errorf("expected stabilized specimen, got %s", fetchedSpecimen.Status)
		}
		return nil

	case "release-for-study":
		specimen, plan, err := claimedPlanSetup(client, server.URL, "PPS-2026-004", "preparator-chen")
		if err != nil {
			return err
		}
		if err := recordProgress(client, server.URL, plan.ID, 100, "表面加固", "preparator-chen"); err != nil {
			return err
		}
		var release releaseView
		if err := requestJSON(
			client,
			http.MethodPost,
			server.URL+"/specimens/"+specimen.ID+"/release",
			contracts.ReleaseRequest{
				Purpose:    "比较解剖研究",
				Recipient:  "地层研究组",
				ApprovedBy: "keeper-zhou",
			},
			http.StatusCreated,
			&release,
		); err != nil {
			return err
		}
		if release.SpecimenID != specimen.ID {
			return errors.New("release record references the wrong specimen")
		}
		var fetched specimenView
		if err := requestJSON(client, http.MethodGet, server.URL+"/specimens/"+specimen.ID, nil, http.StatusOK, &fetched); err != nil {
			return err
		}
		if fetched.Status != "available" {
			return fmt.Errorf("expected available specimen, got %s", fetched.Status)
		}
		return nil

	case "assign-prep-plan":
		return checkAssignmentWorkflow(client, server.URL)
	default:
		return fmt.Errorf("unknown workflow %q", workflow)
	}
}

func registerSpecimen(client *http.Client, baseURL, catalogNumber string) (specimenView, error) {
	var specimen specimenView
	err := requestJSON(
		client,
		http.MethodPost,
		baseURL+"/specimens",
		contracts.RegisterSpecimenRequest{
			CatalogNumber: catalogNumber,
			FieldLabel:    "北坡-07",
			TaxonHint:     "大型脊椎动物",
			MatrixKind:    "粉砂岩",
			JacketNotes:   "夹克边角完好",
		},
		http.StatusCreated,
		&specimen,
	)
	return specimen, err
}

func createPlan(client *http.Client, baseURL, specimenID string) (planView, error) {
	var plan planView
	err := requestJSON(
		client,
		http.MethodPost,
		baseURL+"/specimens/"+specimenID+"/plans",
		contracts.CreatePlanRequest{
			Objective:      "暴露主要关节面",
			Method:         "手工显微剥离",
			Adhesive:       "可逆树脂",
			TargetExposure: 80,
		},
		http.StatusCreated,
		&plan,
	)
	return plan, err
}

// approvePlan approves the draft and returns the resulting approved plan.
func approvePlan(client *http.Client, baseURL, planID string) (planView, error) {
	var approved planView
	err := requestJSON(
		client,
		http.MethodPost,
		baseURL+"/plans/"+planID+"/approval",
		contracts.ApprovalRequest{ApprovedBy: "curator-lin"},
		http.StatusOK,
		&approved,
	)
	return approved, err
}

// claimPlan performs a self-claim by the given preparator.
func claimPlan(client *http.Client, baseURL, planID, actorID string) (planSnapshot, error) {
	var snapshot planSnapshot
	err := requestJSON(
		client,
		http.MethodPost,
		baseURL+"/plans/"+planID+"/claim",
		contracts.ClaimPlanRequest{ActorID: actorID},
		http.StatusOK,
		&snapshot,
	)
	return snapshot, err
}

// directPlan has the prep lead assign the plan to another preparator.
func directPlan(client *http.Client, baseURL, planID, leadID, assigneeID string) (planSnapshot, error) {
	var snapshot planSnapshot
	err := requestJSON(
		client,
		http.MethodPost,
		baseURL+"/plans/"+planID+"/claim",
		contracts.ClaimPlanRequest{
			ActorID:    leadID,
			AssigneeID: assigneeID,
			AssignedBy: leadID,
		},
		http.StatusOK,
		&snapshot,
	)
	return snapshot, err
}

func reassignPlan(client *http.Client, baseURL, planID, fromID, toID string) (planSnapshot, error) {
	var snapshot planSnapshot
	err := requestJSON(
		client,
		http.MethodPost,
		baseURL+"/plans/"+planID+"/reassign",
		contracts.ReassignPlanRequest{FromID: fromID, ToID: toID},
		http.StatusOK,
		&snapshot,
	)
	return snapshot, err
}

// claimedPlanSetup registers, plans, approves and self-claims a plan.
func claimedPlanSetup(client *http.Client, baseURL, catalogNumber, assigneeID string) (specimenView, planView, error) {
	specimen, err := registerSpecimen(client, baseURL, catalogNumber)
	if err != nil {
		return specimenView{}, planView{}, err
	}
	plan, err := createPlan(client, baseURL, specimen.ID)
	if err != nil {
		return specimenView{}, planView{}, err
	}
	approved, err := approvePlan(client, baseURL, plan.ID)
	if err != nil {
		return specimenView{}, planView{}, err
	}
	snapshot, err := claimPlan(client, baseURL, approved.ID, assigneeID)
	if err != nil {
		return specimenView{}, planView{}, err
	}
	if snapshot.Plan.AssigneeID != assigneeID {
		return specimenView{}, planView{}, errors.New("plan was not assigned to the claiming preparator")
	}
	return specimen, snapshot.Plan, nil
}

// approvedPlanSetup registers, plans and approves a plan without claiming it.
func approvedPlanSetup(client *http.Client, baseURL, catalogNumber string) (specimenView, planView, error) {
	specimen, err := registerSpecimen(client, baseURL, catalogNumber)
	if err != nil {
		return specimenView{}, planView{}, err
	}
	plan, err := createPlan(client, baseURL, specimen.ID)
	if err != nil {
		return specimenView{}, planView{}, err
	}
	approved, err := approvePlan(client, baseURL, plan.ID)
	if err != nil {
		return specimenView{}, planView{}, err
	}
	return specimen, approved, nil
}

func recordProgress(client *http.Client, baseURL, planID string, percent int, stage, recordedBy string) error {
	var ignored struct{}
	return requestJSON(
		client,
		http.MethodPost,
		baseURL+"/plans/"+planID+"/progress",
		contracts.ProgressRequest{
			Stage:      stage,
			Percent:    percent,
			Note:       "本地链路检查",
			RecordedBy: recordedBy,
		},
		http.StatusCreated,
		&ignored,
	)
}

func requestJSON(
	client *http.Client,
	method string,
	url string,
	body any,
	wantStatus int,
	target any,
) error {
	status, raw, err := doRequest(client, method, url, body)
	if err != nil {
		return err
	}
	if status != wantStatus {
		return fmt.Errorf("request %s %s returned %d, want %d: %s", method, url, status, wantStatus, string(raw))
	}
	if target == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("decode response for %s %s: %w", method, url, err)
	}
	return nil
}

func doRequest(client *http.Client, method, url string, body any) (int, []byte, error) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		payload = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, url, payload)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return 0, nil, err
	}
	return response.StatusCode, raw, nil
}

func expectStatus(client *http.Client, method, url string, body any, wantStatus int) error {
	status, raw, err := doRequest(client, method, url, body)
	if err != nil {
		return err
	}
	if status != wantStatus {
		return fmt.Errorf("request %s %s returned %d, want %d: %s", method, url, status, wantStatus, string(raw))
	}
	return nil
}

func staffLoad(client *http.Client, baseURL, staffID string) (staffView, error) {
	var envelope struct {
		Items []staffView `json:"items"`
	}
	if err := requestJSON(client, http.MethodGet, baseURL+"/staff", nil, http.StatusOK, &envelope); err != nil {
		return staffView{}, err
	}
	for _, member := range envelope.Items {
		if member.ID == staffID {
			return member, nil
		}
	}
	return staffView{}, fmt.Errorf("staff member %s missing from workbench", staffID)
}

func getPlanSnapshot(client *http.Client, baseURL, planID string) (planSnapshot, error) {
	var snapshot planSnapshot
	err := requestJSON(client, http.MethodGet, baseURL+"/plans/"+planID, nil, http.StatusOK, &snapshot)
	return snapshot, err
}

// checkAssignmentWorkflow exercises claim, directed assignment, transfer, the
// assignment cap, concurrent claims and the closed-plan rules with fixed outcomes.
func checkAssignmentWorkflow(client *http.Client, baseURL string) error {
	const chen = "preparator-chen"
	const zhao = "preparator-zhao"
	const sun = "preparator-sun"
	const lead = "curator-lin"

	// A plan must exist and be approved before it can be claimed.
	draftSpecimen, err := registerSpecimen(client, baseURL, "PPS-2026-010")
	if err != nil {
		return err
	}
	draftPlan, err := createPlan(client, baseURL, draftSpecimen.ID)
	if err != nil {
		return err
	}
	if err := expectStatus(client, http.MethodPost,
		baseURL+"/plans/"+draftPlan.ID+"/claim",
		contracts.ClaimPlanRequest{ActorID: chen}, http.StatusConflict); err != nil {
		return fmt.Errorf("draft claim should conflict: %w", err)
	}

	// Self-claim by a preparator, and the workbench reflects the new load.
	_, planA, err := approvedPlanSetup(client, baseURL, "PPS-2026-011")
	if err != nil {
		return err
	}
	claimedA, err := claimPlan(client, baseURL, planA.ID, chen)
	if err != nil {
		return err
	}
	if claimedA.Plan.AssigneeID != chen || claimedA.Assignment == nil || claimedA.Assignment.AssigneeID != chen {
		return errors.New("self-claim did not record chen as the current holder")
	}
	if load, err := staffLoad(client, baseURL, chen); err != nil || load.ActiveAssignments != 1 || load.AssignmentCap != 3 {
		return fmt.Errorf("workbench load after claim wrong: %+v %v", load, err)
	}

	// Duplicate claim by anyone is rejected; the first holder stays in place.
	if err := expectStatus(client, http.MethodPost,
		baseURL+"/plans/"+planA.ID+"/claim",
		contracts.ClaimPlanRequest{ActorID: zhao}, http.StatusConflict); err != nil {
		return fmt.Errorf("duplicate claim should conflict: %w", err)
	}

	// Concurrent claims on one fresh plan: exactly one winner.
	_, racePlan, err := approvedPlanSetup(client, baseURL, "PPS-2026-012")
	if err != nil {
		return err
	}
	const racers = 8
	var wg sync.WaitGroup
	statuses := make([]int, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			status, _, reqErr := doRequest(client, http.MethodPost,
				baseURL+"/plans/"+racePlan.ID+"/claim",
				contracts.ClaimPlanRequest{ActorID: sun})
			if reqErr != nil {
				status = 0
			}
			statuses[index] = status
		}(i)
	}
	wg.Wait()
	winners, losers := 0, 0
	for _, status := range statuses {
		switch status {
		case http.StatusOK:
			winners++
		case http.StatusConflict:
			losers++
		default:
			return fmt.Errorf("concurrent claim returned unexpected status %d", status)
		}
	}
	if winners != 1 || losers != racers-1 {
		return fmt.Errorf("concurrent claim winners=%d losers=%d", winners, losers)
	}
	raceSnapshot, err := getPlanSnapshot(client, baseURL, racePlan.ID)
	if err != nil {
		return err
	}
	if raceSnapshot.Plan.AssigneeID != sun {
		return errors.New("race plan holder changed unexpectedly")
	}

	// Progress requires the current holder; another preparator is refused.
	if err := recordProgress(client, baseURL, planA.ID, 20, "基质去除", chen); err != nil {
		return err
	}
	if err := expectStatus(client, http.MethodPost,
		baseURL+"/plans/"+planA.ID+"/progress",
		contracts.ProgressRequest{Stage: "基质去除", Percent: 30, RecordedBy: zhao},
		http.StatusConflict); err != nil {
		return fmt.Errorf("non-holder progress should conflict: %w", err)
	}

	// A non-lead cannot direct a plan to someone else; unknown actor is 404.
	if err := expectStatus(client, http.MethodPost,
		baseURL+"/plans/"+planA.ID+"/claim",
		contracts.ClaimPlanRequest{ActorID: chen, AssigneeID: zhao, AssignedBy: chen},
		http.StatusForbidden); err != nil {
		return fmt.Errorf("non-lead direction should be forbidden: %w", err)
	}
	if err := expectStatus(client, http.MethodPost,
		baseURL+"/plans/"+planA.ID+"/claim",
		contracts.ClaimPlanRequest{ActorID: "preparator-nobody"},
		http.StatusNotFound); err != nil {
		return fmt.Errorf("unknown claim actor should be not found: %w", err)
	}

	// Directed assignment by the prep lead to an ineligible role is rejected.
	_, planB, err := approvedPlanSetup(client, baseURL, "PPS-2026-013")
	if err != nil {
		return err
	}
	if err := expectStatus(client, http.MethodPost,
		baseURL+"/plans/"+planB.ID+"/claim",
		contracts.ClaimPlanRequest{ActorID: lead, AssigneeID: "keeper-zhou", AssignedBy: lead},
		http.StatusBadRequest); err != nil {
		return fmt.Errorf("directing to a non-preparator should be invalid: %w", err)
	}
	if _, err := directPlan(client, baseURL, planB.ID, lead, zhao); err != nil {
		return err
	}

	// Assignment cap: fill zhao up to three open plans, then a fourth is refused.
	for _, catalog := range []string{"PPS-2026-014", "PPS-2026-015"} {
		_, plan, setupErr := approvedPlanSetup(client, baseURL, catalog)
		if setupErr != nil {
			return setupErr
		}
		if _, err := directPlan(client, baseURL, plan.ID, lead, zhao); err != nil {
			return err
		}
	}
	if load, err := staffLoad(client, baseURL, zhao); err != nil || load.ActiveAssignments != 3 {
		return fmt.Errorf("zhao should hold three open plans: %+v %v", load, err)
	}
	_, planCap, err := approvedPlanSetup(client, baseURL, "PPS-2026-016")
	if err != nil {
		return err
	}
	if err := expectStatus(client, http.MethodPost,
		baseURL+"/plans/"+planCap.ID+"/claim",
		contracts.ClaimPlanRequest{ActorID: zhao}, http.StatusConflict); err != nil {
		return fmt.Errorf("claim over the cap should conflict: %w", err)
	}
	if err := expectStatus(client, http.MethodPost,
		baseURL+"/plans/"+planCap.ID+"/claim",
		contracts.ClaimPlanRequest{ActorID: lead, AssigneeID: zhao, AssignedBy: lead},
		http.StatusConflict); err != nil {
		return fmt.Errorf("directing over the cap should conflict: %w", err)
	}

	// Unknown plan for claim, transfer and progress has a fixed 404 outcome.
	missingURL := baseURL + "/plans/plan-missing"
	if err := expectStatus(client, http.MethodPost, missingURL+"/claim",
		contracts.ClaimPlanRequest{ActorID: chen}, http.StatusNotFound); err != nil {
		return fmt.Errorf("claim of unknown plan should be not found: %w", err)
	}
	if err := expectStatus(client, http.MethodPost, missingURL+"/reassign",
		contracts.ReassignPlanRequest{FromID: chen, ToID: zhao}, http.StatusNotFound); err != nil {
		return fmt.Errorf("transfer of unknown plan should be not found: %w", err)
	}
	if err := expectStatus(client, http.MethodPost, missingURL+"/progress",
		contracts.ProgressRequest{Stage: "x", Percent: 1, RecordedBy: chen}, http.StatusNotFound); err != nil {
		return fmt.Errorf("progress on unknown plan should be not found: %w", err)
	}

	// Only the current holder can transfer; transferring to an ineligible role fails.
	if err := expectStatus(client, http.MethodPost,
		baseURL+"/plans/"+planA.ID+"/reassign",
		contracts.ReassignPlanRequest{FromID: zhao, ToID: sun}, http.StatusConflict); err != nil {
		return fmt.Errorf("non-holder transfer should conflict: %w", err)
	}
	if err := expectStatus(client, http.MethodPost,
		baseURL+"/plans/"+planA.ID+"/reassign",
		contracts.ReassignPlanRequest{FromID: chen, ToID: "keeper-zhou"}, http.StatusBadRequest); err != nil {
		return fmt.Errorf("transfer to a non-preparator should be invalid: %w", err)
	}

	// Transfer releases chen and establishes sun in one commit. chen's load drops.
	transferred, err := reassignPlan(client, baseURL, planA.ID, chen, sun)
	if err != nil {
		return err
	}
	if transferred.Plan.AssigneeID != sun {
		return errors.New("plan did not move to the new holder")
	}
	if len(transferred.Assignments) != 2 {
		return fmt.Errorf("expected two responsibility records, got %d", len(transferred.Assignments))
	}
	if transferred.Assignment == nil || transferred.Assignment.Status != "active" ||
		transferred.Assignment.AssigneeID != sun || transferred.Assignment.Sequence != 2 {
		return errors.New("new responsibility is not the single active record")
	}
	first := transferred.Assignments[0]
	if first.Status != "released" || first.AssigneeID != chen || first.ReleasedBy != chen {
		return errors.New("old responsibility was not released by the current holder in the same commit")
	}
	if load, err := staffLoad(client, baseURL, chen); err != nil || load.ActiveAssignments != 0 {
		return fmt.Errorf("chen load after transfer should be zero: %+v %v", load, err)
	}

	// The old holder can no longer log progress; the successor can.
	if err := expectStatus(client, http.MethodPost,
		baseURL+"/plans/"+planA.ID+"/progress",
		contracts.ProgressRequest{Stage: "关节加固", Percent: 25, RecordedBy: chen},
		http.StatusConflict); err != nil {
		return fmt.Errorf("old holder progress after transfer should conflict: %w", err)
	}
	if err := recordProgress(client, baseURL, planA.ID, 40, "关节加固", sun); err != nil {
		return err
	}

	// An unassigned plan rejects progress even from a qualified preparator.
	if err := expectStatus(client, http.MethodPost,
		baseURL+"/plans/"+planCap.ID+"/progress",
		contracts.ProgressRequest{Stage: "表面清理", Percent: 10, RecordedBy: chen},
		http.StatusConflict); err != nil {
		return fmt.Errorf("progress on unassigned plan should conflict: %w", err)
	}

	// A completed plan cannot be transferred and keeps its last holder.
	_, completedPlan, err := claimedPlanSetup(client, baseURL, "PPS-2026-017", chen)
	if err != nil {
		return err
	}
	if err := recordProgress(client, baseURL, completedPlan.ID, 100, "表面加固", chen); err != nil {
		return err
	}
	if err := expectStatus(client, http.MethodPost,
		baseURL+"/plans/"+completedPlan.ID+"/reassign",
		contracts.ReassignPlanRequest{FromID: chen, ToID: sun}, http.StatusConflict); err != nil {
		return fmt.Errorf("completed plan transfer should conflict: %w", err)
	}
	finalSnapshot, err := getPlanSnapshot(client, baseURL, completedPlan.ID)
	if err != nil {
		return err
	}
	if finalSnapshot.Plan.AssigneeID != chen {
		return errors.New("completed plan should retain its last holder")
	}
	if finalSnapshot.Assignment == nil || finalSnapshot.Assignment.Status != "completed" {
		return errors.New("completed plan responsibility should be closed as completed")
	}
	return nil
}
