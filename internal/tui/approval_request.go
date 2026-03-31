package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/termia/termia/internal/agent"
)

type approvalRequest struct {
	request  agent.HITLRequest
	response chan agent.HITLResponse
}

type approvalRequestMsg struct {
	request approvalRequest
}

type askRequest struct {
	request  agent.HITLRequest
	response chan agent.HITLResponse
}

type askRequestMsg struct {
	request askRequest
}

type tuiResponder struct {
	approvals chan<- approvalRequest
	asks      chan<- askRequest
}

func newTUIResponder(approvals chan<- approvalRequest, asks chan<- askRequest) agent.HITLResponder {
	return &tuiResponder{
		approvals: approvals,
		asks:      asks,
	}
}

func (p *tuiResponder) Handle(ctx context.Context, request agent.HITLRequest) (agent.HITLResponse, error) {
	response := make(chan agent.HITLResponse, 1)
	switch request.Kind {
	case agent.HITLKindConfirm:
		select {
		case p.approvals <- approvalRequest{request: request, response: response}:
		case <-ctx.Done():
			return agent.HITLResponse{}, ctx.Err()
		}
	default:
		select {
		case p.asks <- askRequest{request: request, response: response}:
		case <-ctx.Done():
			return agent.HITLResponse{}, ctx.Err()
		}
	}

	select {
	case resp := <-response:
		return resp, nil
	case <-ctx.Done():
		return agent.HITLResponse{}, ctx.Err()
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

func waitForAskRequestCmd(ch <-chan askRequest) tea.Cmd {
	return func() tea.Msg {
		request, ok := <-ch
		if !ok {
			return nil
		}
		return askRequestMsg{request: request}
	}
}
