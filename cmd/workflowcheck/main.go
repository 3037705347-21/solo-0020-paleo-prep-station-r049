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

const (
	leadID       = "curator-lin"
	preparatorA  = "preparator-chen"
	preparatorB  = "preparator-yu"
	preparatorC  = "preparator-gao"
	preparatorD  = "preparator-he"
	catalogStart = 2026001
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
}

type assignmentView struct {
	ID           string `json:"id"`
	PlanID       string `json:"plan_id"`
	Kind         string `json:"kind"`
	PreparatorID string `json:"preparator_id"`
	State        string `json:"state"`
}

type planSnapshot struct {
	Plan     planView `json:"plan"`
	Progress []struct {
		Percent int `json:"percent"`
	} `json:"progress"`
	CurrentAssignment *assignmentView `json:"current_assignment"`
}

type workbenchResponse struct {
	Items []struct {
		PlanID            string          `json:"plan_id"`
		CurrentAssignment *assignmentView `json:"current_assignment"`
	} `json:"items"`
}

type workloadView struct {
	OpenClaims    int `json:"open_claims"`
	AssignmentCap int `json:"assignment_cap"`
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
		specimen, err := registerSpecimen(client, server.URL, catalogStart)
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
		specimen, err := registerSpecimen(client, server.URL, catalogStart)
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
			contracts.ApprovalRequest{ApprovedBy: leadID},
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
		specimen, plan, err := claimedPlanSetup(client, server.URL, catalogStart, preparatorA)
		if err != nil {
			return err
		}
		if err := recordProgress(client, server.URL, plan.ID, 35, "矩阵剥离", preparatorA); err != nil {
			return err
		}
		if err := recordProgress(client, server.URL, plan.ID, 100, "表面加固", preparatorA); err != nil {
			return err
		}
		var snapshot planSnapshot
		if err := requestJSON(client, http.MethodGet, server.URL+"/plans/"+plan.ID, nil, http.StatusOK, &snapshot); err != nil {
			return err
		}
		if snapshot.Plan.Status != "completed" || len(snapshot.Progress) != 2 {
			return errors.New("plan did not complete with exactly two progress entries")
		}
		if snapshot.CurrentAssignment == nil || snapshot.CurrentAssignment.PreparatorID != preparatorA {
			return errors.New("completed plan must retain its last responsible preparator")
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
		specimen, plan, err := claimedPlanSetup(client, server.URL, catalogStart, preparatorA)
		if err != nil {
			return err
		}
		if err := recordProgress(client, server.URL, plan.ID, 100, "表面加固", preparatorA); err != nil {
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

	case "assign-and-transfer":
		return checkAssignAndTransfer(client, server.URL)
	default:
		return fmt.Errorf("unknown workflow %q", workflow)
	}
}

func checkAssignAndTransfer(client *http.Client, baseURL string) error {
	if err := registerStaffRoster(client, baseURL); err != nil {
		return err
	}

	// Unknown plans produce a deterministic 404 on every assignment route.
	if err := expectStatus(client, http.MethodPost, baseURL+"/plans/plan-missing/assignments",
		contracts.ClaimAssignmentRequest{PreparatorID: preparatorA, AssignedBy: leadID}, http.StatusNotFound); err != nil {
		return err
	}
	if err := expectStatus(client, http.MethodPost, baseURL+"/plans/plan-missing/assignment/transfer",
		contracts.TransferAssignmentRequest{TargetPreparatorID: preparatorB, AssignedBy: leadID}, http.StatusNotFound); err != nil {
		return err
	}

	specimen, plan, err := claimedPlanSetup(client, baseURL, catalogStart, preparatorA)
	if err != nil {
		return err
	}

	// Repeated claims are rejected as conflicts.
	if err := expectStatus(client, http.MethodPost, baseURL+"/plans/"+plan.ID+"/assignments",
		contracts.ClaimAssignmentRequest{PreparatorID: preparatorB, AssignedBy: leadID}, http.StatusConflict); err != nil {
		return err
	}

	// Concurrent claims on a fresh plan: exactly one must win.
	if err := checkConcurrentClaim(client, baseURL); err != nil {
		return err
	}

	// Only the current holder can append progress.
	if err := expectStatus(client, http.MethodPost, baseURL+"/plans/"+plan.ID+"/progress",
		contracts.ProgressRequest{Stage: "清理", Percent: 10, RecordedBy: preparatorB}, http.StatusConflict); err != nil {
		return errors.New("non-holder progress must be rejected: " + err.Error())
	}
	if err := recordProgress(client, baseURL, plan.ID, 20, "初步清理", preparatorA); err != nil {
		return err
	}

	// Transferring to unqualified actors (lead role, unknown staff) is rejected.
	if err := expectStatus(client, http.MethodPost, baseURL+"/plans/"+plan.ID+"/assignment/transfer",
		contracts.TransferAssignmentRequest{TargetPreparatorID: leadID, AssignedBy: leadID}, http.StatusConflict); err != nil {
		return errors.New("transfer to a non-preparator role must be rejected: " + err.Error())
	}
	if err := expectStatus(client, http.MethodPost, baseURL+"/plans/"+plan.ID+"/assignment/transfer",
		contracts.TransferAssignmentRequest{TargetPreparatorID: "preparator-ghost", AssignedBy: leadID}, http.StatusConflict); err != nil {
		return errors.New("transfer to unknown staff must be rejected: " + err.Error())
	}
	if err := expectStatus(client, http.MethodPost, baseURL+"/plans/"+plan.ID+"/assignments",
		contracts.ClaimAssignmentRequest{PreparatorID: preparatorA, AssignedBy: "preparator-ghost"}, http.StatusConflict); err != nil {
		return errors.New("assignment by a non-lead actor must be rejected: " + err.Error())
	}

	// A valid transfer releases the old holder and establishes the new one.
	var current assignmentView
	if err := requestJSON(client, http.MethodPost, baseURL+"/plans/"+plan.ID+"/assignment/transfer",
		contracts.TransferAssignmentRequest{TargetPreparatorID: preparatorB, AssignedBy: leadID},
		http.StatusCreated, &current); err != nil {
		return err
	}
	if current.PreparatorID != preparatorB || current.State != "active" || current.Kind != "transfer" {
		return fmt.Errorf("transfer returned unexpected assignment %+v", current)
	}
	if err := expectStatus(client, http.MethodPost, baseURL+"/plans/"+plan.ID+"/progress",
		contracts.ProgressRequest{Stage: "清理", Percent: 30, RecordedBy: preparatorA}, http.StatusConflict); err != nil {
		return errors.New("old holder must lose progress rights after transfer: " + err.Error())
	}
	if err := recordProgress(client, baseURL, plan.ID, 40, "基质去除", preparatorB); err != nil {
		return err
	}
	var history struct {
		Items []assignmentView `json:"items"`
	}
	if err := requestJSON(client, http.MethodGet, baseURL+"/plans/"+plan.ID+"/assignments", nil, http.StatusOK, &history); err != nil {
		return err
	}
	if len(history.Items) != 2 || history.Items[0].State != "released" || history.Items[1].State != "active" {
		return fmt.Errorf("assignment history must show released then active row, got %+v", history.Items)
	}

	// Workbench surfaces the current responsibility.
	var bench workbenchResponse
	if err := requestJSON(client, http.MethodGet, baseURL+"/workbench", nil, http.StatusOK, &bench); err != nil {
		return err
	}
	found := false
	for _, item := range bench.Items {
		if item.PlanID == plan.ID {
			found = item.CurrentAssignment != nil && item.CurrentAssignment.PreparatorID == preparatorB
		}
	}
	if !found {
		return errors.New("workbench does not show the current holder of the transferred plan")
	}

	// The claim cap is fixed and reflected on the staff workload view.
	var workload workloadView
	if err := requestJSON(client, http.MethodGet, baseURL+"/staff/"+preparatorB, nil, http.StatusOK, &workload); err != nil {
		return err
	}
	if workload.AssignmentCap != 3 || workload.OpenClaims != 1 {
		return fmt.Errorf("unexpected workload %+v", workload)
	}
	if err := checkAssignmentCap(client, baseURL); err != nil {
		return err
	}

	// Once a plan is completed it can no longer be transferred.
	if err := recordProgress(client, baseURL, plan.ID, 100, "完成", preparatorB); err != nil {
		return err
	}
	if err := expectStatus(client, http.MethodPost, baseURL+"/plans/"+plan.ID+"/assignment/transfer",
		contracts.TransferAssignmentRequest{TargetPreparatorID: preparatorA, AssignedBy: leadID}, http.StatusConflict); err != nil {
		return errors.New("transfer after completion must be rejected: " + err.Error())
	}

	_ = specimen
	return nil
}

func checkConcurrentClaim(client *http.Client, baseURL string) error {
	_, plan, err := approvedPlanSetup(client, baseURL, catalogStart+9)
	if err != nil {
		return err
	}
	type outcome struct {
		status int
	}
	start := make(chan struct{})
	results := make(chan outcome, 2)
	var wg sync.WaitGroup
	for _, holder := range []string{preparatorC, preparatorD} {
		wg.Add(1)
		go func(preparator string) {
			defer wg.Done()
			<-start
			status, _ := rawJSON(client, http.MethodPost, baseURL+"/plans/"+plan.ID+"/assignments",
				contracts.ClaimAssignmentRequest{PreparatorID: preparator, AssignedBy: leadID})
			results <- outcome{status: status}
		}(holder)
	}
	close(start)
	wg.Wait()
	close(results)
	created, conflict := 0, 0
	for result := range results {
		switch result.status {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			conflict++
		default:
			return fmt.Errorf("concurrent claim returned unexpected status %d", result.status)
		}
	}
	if created != 1 || conflict != 1 {
		return fmt.Errorf("concurrent claim needs exactly one winner, got %d created and %d conflicts", created, conflict)
	}
	return nil
}

func checkAssignmentCap(client *http.Client, baseURL string) error {
	// preparatorB already holds one open plan from the transfer scenario; fill
	// the remaining two slots, then a fourth open plan must be rejected.
	var fillIDs []string
	for i := 0; i < 2; i++ {
		_, plan, err := claimedPlanSetup(client, baseURL, catalogStart+20+i, preparatorB)
		if err != nil {
			return err
		}
		fillIDs = append(fillIDs, plan.ID)
		if err := recordProgress(client, baseURL, plan.ID, 10, "起步", preparatorB); err != nil {
			return err
		}
	}
	_, extra, err := approvedPlanSetup(client, baseURL, catalogStart+30)
	if err != nil {
		return err
	}
	if err := expectStatus(client, http.MethodPost, baseURL+"/plans/"+extra.ID+"/assignments",
		contracts.ClaimAssignmentRequest{PreparatorID: preparatorB, AssignedBy: leadID}, http.StatusConflict); err != nil {
		return errors.New("claim beyond the limit must be rejected: " + err.Error())
	}
	// Completing a held plan releases one slot; the previously rejected plan
	// can now be claimed deterministically.
	if err := recordProgress(client, baseURL, fillIDs[0], 100, "收尾", preparatorB); err != nil {
		return err
	}
	if err := claimPlan(client, baseURL, extra.ID, preparatorB); err != nil {
		return errors.New("claim after capacity release must succeed: " + err.Error())
	}
	var workload workloadView
	if err := requestJSON(client, http.MethodGet, baseURL+"/staff/"+preparatorB, nil, http.StatusOK, &workload); err != nil {
		return err
	}
	if workload.OpenClaims != 3 {
		return fmt.Errorf("expected three open claims after refill, got %d", workload.OpenClaims)
	}
	return nil
}

func registerStaffRoster(client *http.Client, baseURL string) error {
	members := []contracts.RegisterStaffRequest{
		{ID: leadID, Name: "林负责人", Role: "prep_lead"},
		{ID: preparatorA, Name: "陈制备", Role: "preparator"},
		{ID: preparatorB, Name: "余制备", Role: "preparator"},
		{ID: preparatorC, Name: "高制备", Role: "preparator"},
		{ID: preparatorD, Name: "何制备", Role: "preparator"},
	}
	for _, member := range members {
		status, _ := rawJSON(client, http.MethodPost, baseURL+"/staff", member)
		if status != http.StatusCreated && status != http.StatusConflict {
			return fmt.Errorf("register staff %s returned %d", member.ID, status)
		}
	}
	return nil
}

func registerSpecimen(client *http.Client, baseURL string, catalog int) (specimenView, error) {
	var specimen specimenView
	err := requestJSON(
		client,
		http.MethodPost,
		baseURL+"/specimens",
		contracts.RegisterSpecimenRequest{
			CatalogNumber: fmt.Sprintf("PPS-%d", catalog),
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

func approvedPlanSetup(client *http.Client, baseURL string, catalog int) (specimenView, planView, error) {
	specimen, err := registerSpecimen(client, baseURL, catalog)
	if err != nil {
		return specimenView{}, planView{}, err
	}
	plan, err := createPlan(client, baseURL, specimen.ID)
	if err != nil {
		return specimenView{}, planView{}, err
	}
	if err := requestJSON(
		client,
		http.MethodPost,
		baseURL+"/plans/"+plan.ID+"/approval",
		contracts.ApprovalRequest{ApprovedBy: leadID},
		http.StatusOK,
		&plan,
	); err != nil {
		return specimenView{}, planView{}, err
	}
	return specimen, plan, nil
}

func claimedPlanSetup(client *http.Client, baseURL string, catalog int, preparator string) (specimenView, planView, error) {
	if err := registerStaffRoster(client, baseURL); err != nil {
		return specimenView{}, planView{}, err
	}
	specimen, plan, err := approvedPlanSetup(client, baseURL, catalog)
	if err != nil {
		return specimenView{}, planView{}, err
	}
	if err := claimPlan(client, baseURL, plan.ID, preparator); err != nil {
		return specimenView{}, planView{}, err
	}
	return specimen, plan, nil
}

func claimPlan(client *http.Client, baseURL, planID, preparator string) error {
	return requestJSON(
		client,
		http.MethodPost,
		baseURL+"/plans/"+planID+"/assignments",
		contracts.ClaimAssignmentRequest{PreparatorID: preparator, AssignedBy: leadID},
		http.StatusCreated,
		nil,
	)
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

func expectStatus(
	client *http.Client,
	method string,
	url string,
	body any,
	wantStatus int,
) error {
	status, raw := rawJSON(client, method, url, body)
	if status != wantStatus {
		return fmt.Errorf("request %s %s returned %d (want %d): %s", method, url, status, wantStatus, string(raw))
	}
	return nil
}

func rawJSON(client *http.Client, method, url string, body any) (int, []byte) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, nil
		}
		payload = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, url, payload)
	if err != nil {
		return 0, nil
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, nil
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	return response.StatusCode, raw
}

func requestJSON(
	client *http.Client,
	method string,
	url string,
	body any,
	wantStatus int,
	target any,
) error {
	status, raw := rawJSON(client, method, url, body)
	if status == 0 {
		return fmt.Errorf("request %s %s failed to execute", method, url)
	}
	if status != wantStatus {
		return fmt.Errorf("request %s %s returned %d: %s", method, url, status, string(raw))
	}
	if target == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("decode response for %s %s: %w", method, url, err)
	}
	return nil
}
