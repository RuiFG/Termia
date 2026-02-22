package agent

import (
	"fmt"
	"strings"
)

const (
	AskOptionTitleMaxLen = 20
	AskOptionDescMaxLen  = 60
	AskOptionMinCount    = 3
	AskOptionMaxCount    = 4
)

const AskTypeYourAnswerTitle = "Type Your Answer"

type ApprovalPrompt struct {
	PromptID  string `json:"prompt_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Command   string `json:"command" jsonschema:"description=Command to approve,required"`
	Cwd       string `json:"cwd,omitempty" jsonschema:"description=Working directory (optional)"`
	RiskNote  string `json:"risk_note,omitempty" jsonschema:"description=Risk or explanation provided by the agent"`
}

type ApprovalDecisionType string

const (
	ApprovalDecisionApprove  ApprovalDecisionType = "approve"
	ApprovalDecisionEdit     ApprovalDecisionType = "edit"
	ApprovalDecisionReject   ApprovalDecisionType = "reject"
	ApprovalDecisionRephrase ApprovalDecisionType = "rephrase"
)

type ApprovalDecision struct {
	Type     ApprovalDecisionType `json:"type"`
	Command  string               `json:"command,omitempty"`
	Rephrase string               `json:"rephrase,omitempty"`
	Reason   string               `json:"reason,omitempty"`
}

type AskOption struct {
	Title       string `json:"title" jsonschema:"description=Short option title,required"`
	Description string `json:"description,omitempty" jsonschema:"description=Short description"`
}

type AskQuestion struct {
	Question string      `json:"question" jsonschema:"description=Full question text,required"`
	Header   string      `json:"header" jsonschema:"description=Short label for UI,required"`
	Options  []AskOption `json:"options" jsonschema:"description=Available options,required"`
	Multiple bool        `json:"multiple,omitempty" jsonschema:"description=Allow multiple selections"`
}

type AskAnswer struct {
	Question      string   `json:"question"`
	Selected      []string `json:"selected"`
	CustomAnswer  string   `json:"custom_answer,omitempty"`
	UsedCustom    bool     `json:"used_custom,omitempty"`
	SelectionNote string   `json:"selection_note,omitempty"`
}

func ValidateAskOption(option AskOption) error {
	if strings.TrimSpace(option.Title) == "" {
		return fmt.Errorf("ask option title is required")
	}
	if len([]rune(option.Title)) > AskOptionTitleMaxLen {
		return fmt.Errorf("ask option title exceeds %d characters", AskOptionTitleMaxLen)
	}
	if len([]rune(option.Description)) > AskOptionDescMaxLen {
		return fmt.Errorf("ask option description exceeds %d characters", AskOptionDescMaxLen)
	}
	return nil
}

func ValidateAskQuestion(question AskQuestion) error {
	if strings.TrimSpace(question.Question) == "" {
		return fmt.Errorf("ask question text is required")
	}
	if strings.TrimSpace(question.Header) == "" {
		return fmt.Errorf("ask question header is required")
	}
	if len(question.Options) < AskOptionMinCount || len(question.Options) > AskOptionMaxCount {
		return fmt.Errorf("ask question must include %d-%d options", AskOptionMinCount, AskOptionMaxCount)
	}
	for _, option := range question.Options {
		if err := ValidateAskOption(option); err != nil {
			return err
		}
	}
	return nil
}

func NormalizeAskQuestion(question AskQuestion) (AskQuestion, error) {
	questionText := strings.TrimSpace(question.Question)
	if questionText == "" {
		return AskQuestion{}, fmt.Errorf("ask question text is required")
	}
	question.Question = questionText
	header := strings.TrimSpace(question.Header)
	if header == "" {
		header = truncateRunes(questionText, AskOptionTitleMaxLen)
	}
	question.Header = header

	options := make([]AskOption, 0, len(question.Options))
	hasTypeYourAnswer := false
	for _, option := range question.Options {
		title := strings.TrimSpace(option.Title)
		desc := strings.TrimSpace(option.Description)
		if title == "" {
			continue
		}
		if strings.EqualFold(title, AskTypeYourAnswerTitle) {
			title = AskTypeYourAnswerTitle
			hasTypeYourAnswer = true
		}
		title = truncateRunes(title, AskOptionTitleMaxLen)
		desc = truncateRunes(desc, AskOptionDescMaxLen)
		options = append(options, AskOption{Title: title, Description: desc})
	}
	if !hasTypeYourAnswer {
		options = append(options, AskOption{Title: AskTypeYourAnswerTitle})
	}
	if len(options) == 0 {
		options = []AskOption{{Title: AskTypeYourAnswerTitle}}
	}
	if len(options) > AskOptionMaxCount {
		typedIndex := -1
		for idx, option := range options {
			if strings.EqualFold(strings.TrimSpace(option.Title), AskTypeYourAnswerTitle) {
				typedIndex = idx
				break
			}
		}
		trimmed := append([]AskOption{}, options[:AskOptionMaxCount]...)
		if typedIndex == -1 {
			trimmed[len(trimmed)-1] = AskOption{Title: AskTypeYourAnswerTitle}
		} else if typedIndex >= AskOptionMaxCount {
			trimmed[len(trimmed)-1] = AskOption{Title: AskTypeYourAnswerTitle}
		}
		options = trimmed
	}
	question.Options = options
	return question, nil
}

func truncateRunes(value string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}
