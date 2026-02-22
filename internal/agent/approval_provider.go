package agent

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
)

type ApprovalProvider interface {
	RequestApproval(ctx context.Context, prompt ApprovalPrompt) (ApprovalDecision, error)
}

type CLIApprovalProvider struct{}

func NewCLIApprovalProvider() *CLIApprovalProvider {
	return &CLIApprovalProvider{}
}

func (p *CLIApprovalProvider) RequestApproval(ctx context.Context, prompt ApprovalPrompt) (ApprovalDecision, error) {
	if ctx.Err() != nil {
		return ApprovalDecision{Type: ApprovalDecisionReject, Reason: ctx.Err().Error()}, ctx.Err()
	}
	return promptApprovalDecisionCLI(prompt)
}

func promptApprovalDecisionCLI(prompt ApprovalPrompt) (ApprovalDecision, error) {
	reader := bufio.NewReader(os.Stdin)
	command := strings.TrimSpace(prompt.Command)
	if command == "" {
		return ApprovalDecision{Type: ApprovalDecisionReject, Reason: "command is empty"}, nil
	}
	for {
		fmt.Printf("\nProposed command:\n  %s\n", command)
		if strings.TrimSpace(prompt.RiskNote) != "" {
			fmt.Printf("Risk/Note:\n  %s\n", strings.TrimSpace(prompt.RiskNote))
		}
		fmt.Printf("\n[Enter] Approve  [E] Edit  [R] Reject  [N] Natural language\n> ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return ApprovalDecision{Type: ApprovalDecisionReject, Reason: err.Error()}, err
		}
		choice := strings.ToLower(strings.TrimSpace(input))
		switch choice {
		case "", "y", "yes", "approve", "a":
			return ApprovalDecision{Type: ApprovalDecisionApprove}, nil
		case "r", "reject", "no":
			return ApprovalDecision{Type: ApprovalDecisionReject}, nil
		case "e", "edit":
			decision, err := promptEditDecisionCLI(command, reader)
			if err != nil {
				return ApprovalDecision{Type: ApprovalDecisionReject, Reason: err.Error()}, err
			}
			if decision.Type != ApprovalDecisionReject {
				return decision, nil
			}
		case "n", "nl", "natural":
			decision, err := promptRephraseDecisionCLI(reader)
			if err != nil {
				return ApprovalDecision{Type: ApprovalDecisionReject, Reason: err.Error()}, err
			}
			if decision.Type != ApprovalDecisionReject {
				return decision, nil
			}
		default:
			fmt.Printf("Unknown choice. Use Enter/E/R/N.\n")
		}
	}
}

func promptEditDecisionCLI(original string, reader *bufio.Reader) (ApprovalDecision, error) {
	fmt.Printf("Edit command (leave empty to cancel):\n> ")
	input, err := reader.ReadString('\n')
	if err != nil {
		return ApprovalDecision{Type: ApprovalDecisionReject, Reason: err.Error()}, err
	}
	edited := strings.TrimSpace(input)
	if edited == "" {
		return ApprovalDecision{Type: ApprovalDecisionReject}, nil
	}
	fmt.Printf("\nConfirm edited command:\n  %s\nApprove? [y/N]: ", edited)
	confirmInput, err := reader.ReadString('\n')
	if err != nil {
		return ApprovalDecision{Type: ApprovalDecisionReject, Reason: err.Error()}, err
	}
	confirm := strings.ToLower(strings.TrimSpace(confirmInput))
	if confirm == "y" || confirm == "yes" {
		return ApprovalDecision{Type: ApprovalDecisionEdit, Command: edited}, nil
	}
	return ApprovalDecision{Type: ApprovalDecisionReject}, nil
}

func promptRephraseDecisionCLI(reader *bufio.Reader) (ApprovalDecision, error) {
	fmt.Printf("Describe the desired action:\n> ")
	input, err := reader.ReadString('\n')
	if err != nil {
		return ApprovalDecision{Type: ApprovalDecisionReject, Reason: err.Error()}, err
	}
	rephrase := strings.TrimSpace(input)
	if rephrase == "" {
		return ApprovalDecision{Type: ApprovalDecisionReject}, nil
	}
	return ApprovalDecision{Type: ApprovalDecisionRephrase, Rephrase: rephrase}, nil
}
