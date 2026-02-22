package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/termia/termia/internal/agent"
)

type approvalRequest struct {
	prompt   agent.ApprovalPrompt
	response chan agent.ApprovalDecision
}

type approvalRequestMsg struct {
	request approvalRequest
}

type tuiApprovalProvider struct {
	requests chan<- approvalRequest
}

func newTUIApprovalProvider(requests chan<- approvalRequest) agent.ApprovalProvider {
	return &tuiApprovalProvider{requests: requests}
}

func (p *tuiApprovalProvider) RequestApproval(ctx context.Context, prompt agent.ApprovalPrompt) (agent.ApprovalDecision, error) {
	if ctx.Err() != nil {
		return agent.ApprovalDecision{Type: agent.ApprovalDecisionReject, Reason: ctx.Err().Error()}, ctx.Err()
	}
	response := make(chan agent.ApprovalDecision, 1)
	select {
	case p.requests <- approvalRequest{prompt: prompt, response: response}:
	case <-ctx.Done():
		return agent.ApprovalDecision{Type: agent.ApprovalDecisionReject, Reason: ctx.Err().Error()}, ctx.Err()
	}
	select {
	case decision := <-response:
		return decision, nil
	case <-ctx.Done():
		return agent.ApprovalDecision{Type: agent.ApprovalDecisionReject, Reason: ctx.Err().Error()}, ctx.Err()
	}
}

func waitForApprovalRequestCmd(ch <-chan approvalRequest) tea.Cmd {
	return func() tea.Msg {
		request, ok := <-ch
		if !ok {
			return nil
		}
		return approvalRequestMsg{request: request}
	}
}
