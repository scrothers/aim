package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/scrothers/aim/internal/hooks"
	"github.com/scrothers/aim/internal/mail"
)

// Approver holds agent permission prompts waiting for the operator;
// *hooks.Server implements it.
type Approver interface {
	Approvals() []hooks.Approval
	Decide(id, verdict, reason string) error
}

// ApprovalsBody lists pending permission prompts, oldest first.
type ApprovalsBody struct {
	Items []hooks.Approval `json:"items"`
}

// DecisionRequest answers a pending permission prompt.
type DecisionRequest struct {
	Decision string `json:"decision" enum:"allow,deny" doc:"Allow or deny the tool call."`
	Reason   string `json:"reason,omitempty" maxLength:"500" doc:"Optional note shown to the agent."`
}

// DecisionBody confirms a decision.
type DecisionBody struct {
	ID       string `json:"id"`
	Decision string `json:"decision" enum:"allow,deny"`
}

// ApprovalSettingsBody is the remote approval setting.
type ApprovalSettingsBody struct {
	Enabled        bool `json:"enabled" doc:"Wait for the console on agent permission prompts. Off, agents prompt in their terminal as usual."`
	TimeoutSeconds int  `json:"timeout_seconds" minimum:"10" maximum:"3600" doc:"Seconds a prompt waits for the console before falling back to the terminal (default 300)."`
}

// ApprovalSettingsRequest replaces the remote approval setting.
type ApprovalSettingsRequest struct {
	Enabled        bool `json:"enabled" doc:"Wait for the console on agent permission prompts."`
	TimeoutSeconds int  `json:"timeout_seconds,omitempty" minimum:"10" maximum:"3600" doc:"Seconds a prompt waits for the console (default 300)."`
}

type decideInput struct {
	ID   string `path:"id" doc:"Approval id."`
	Body DecisionRequest
}

func registerApprovals(api huma.API, h *handlers) {
	op := func(method, id, path, summary string) huma.Operation {
		return huma.Operation{OperationID: id, Method: method, Path: path, Summary: summary, Tags: []string{"aim"}}
	}
	huma.Register(api, op(http.MethodGet, "listApprovals", "/api/approvals", "Agent permission prompts waiting for the operator"), h.listApprovals)
	huma.Register(api, op(http.MethodPost, "decideApproval", "/api/approvals/{id}", "Allow or deny a pending permission prompt"), h.decideApproval)
	huma.Register(api, op(http.MethodGet, "getApprovalSettings", "/api/settings/approvals", "Remote approval setting"), h.approvalSettings)
	huma.Register(api, op(http.MethodPut, "updateApprovalSettings", "/api/settings/approvals", "Turn remote approvals on or off and set the timeout"), h.updateApprovalSettings)
}

func (h *handlers) listApprovals(_ context.Context, _ *struct{}) (*out[ApprovalsBody], error) {
	if h.approvals == nil {
		return nil, unavailable("approvals")
	}
	return &out[ApprovalsBody]{Body: ApprovalsBody{Items: h.approvals.Approvals()}}, nil
}

func (h *handlers) decideApproval(_ context.Context, in *decideInput) (*out[DecisionBody], error) {
	if h.approvals == nil {
		return nil, unavailable("approvals")
	}
	err := h.approvals.Decide(in.ID, in.Body.Decision, in.Body.Reason)
	switch {
	case errors.Is(err, hooks.ErrApprovalNotFound):
		return nil, huma.NewError(http.StatusNotFound, "approval not pending: it was decided, timed out or its agent stopped waiting")
	case errors.Is(err, hooks.ErrInvalidDecision):
		return nil, huma.NewError(http.StatusBadRequest, err.Error())
	case err != nil:
		return nil, huma.NewError(http.StatusInternalServerError, "internal error", err)
	}
	return &out[DecisionBody]{Body: DecisionBody{ID: in.ID, Decision: in.Body.Decision}}, nil
}

func (h *handlers) approvalSettings(ctx context.Context, _ *struct{}) (*out[ApprovalSettingsBody], error) {
	a, err := h.mail.ApprovalSettings(ctx)
	if err != nil {
		return nil, apiError(err)
	}
	return &out[ApprovalSettingsBody]{Body: ApprovalSettingsBody{Enabled: a.Enabled, TimeoutSeconds: a.TimeoutSeconds}}, nil
}

func (h *handlers) updateApprovalSettings(ctx context.Context, in *struct{ Body ApprovalSettingsRequest }) (*out[ApprovalSettingsBody], error) {
	a, err := h.mail.SetApprovalSettings(ctx, mail.ApprovalSettings{Enabled: in.Body.Enabled, TimeoutSeconds: in.Body.TimeoutSeconds})
	if err != nil {
		return nil, apiError(err)
	}
	return &out[ApprovalSettingsBody]{Body: ApprovalSettingsBody{Enabled: a.Enabled, TimeoutSeconds: a.TimeoutSeconds}}, nil
}
