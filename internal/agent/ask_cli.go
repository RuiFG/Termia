package agent

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func promptAskQuestionsCLI(questions []AskQuestion) ([]AskAnswer, error) {
	reader := bufio.NewReader(os.Stdin)
	answers := make([]AskAnswer, 0, len(questions))
	for _, question := range questions {
		answer, err := promptAskQuestionCLI(reader, question)
		if err != nil {
			return nil, err
		}
		answers = append(answers, answer)
	}
	return answers, nil
}

func promptAskQuestionCLI(reader *bufio.Reader, question AskQuestion) (AskAnswer, error) {
	fmt.Printf("\n%s\n", strings.TrimSpace(question.Question))
	for idx, option := range question.Options {
		label := strings.TrimSpace(option.Title)
		description := strings.TrimSpace(option.Description)
		if description != "" {
			fmt.Printf("  %d) %s - %s\n", idx+1, label, description)
			continue
		}
		fmt.Printf("  %d) %s\n", idx+1, label)
	}

	for {
		if question.Multiple {
			fmt.Printf("Select options (comma-separated): ")
		} else {
			fmt.Printf("Select option: ")
		}
		input, err := reader.ReadString('\n')
		if err != nil {
			return AskAnswer{Question: question.Question}, err
		}
		selection := strings.TrimSpace(input)
		if selection == "" {
			continue
		}
		selected, useCustom, err := parseAskSelection(selection, question)
		if err != nil {
			fmt.Printf("Invalid selection. Try again.\n")
			continue
		}
		if useCustom {
			custom, err := promptAskCustomAnswer(reader)
			if err != nil {
				return AskAnswer{Question: question.Question}, err
			}
			if strings.TrimSpace(custom) == "" {
				fmt.Printf("Custom answer required. Try again.\n")
				continue
			}
			return AskAnswer{
				Question:     question.Question,
				Selected:     []string{custom},
				CustomAnswer: custom,
				UsedCustom:   true,
			}, nil
		}
		return AskAnswer{
			Question: question.Question,
			Selected: selected,
		}, nil
	}
}

func parseAskSelection(selection string, question AskQuestion) ([]string, bool, error) {
	parts := strings.FieldsFunc(selection, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	if len(parts) == 0 {
		return nil, false, fmt.Errorf("empty selection")
	}
	selectedIdx := map[int]struct{}{}
	useCustom := false
	for _, part := range parts {
		trimmed := strings.ToLower(strings.TrimSpace(part))
		if trimmed == "" {
			continue
		}
		if trimmed == "t" || trimmed == "type" || trimmed == "custom" {
			useCustom = true
			continue
		}
		idx, err := strconv.Atoi(trimmed)
		if err != nil {
			return nil, false, fmt.Errorf("invalid selection")
		}
		if idx < 1 || idx > len(question.Options) {
			return nil, false, fmt.Errorf("invalid selection")
		}
		selectedIdx[idx-1] = struct{}{}
	}
	if !question.Multiple && len(selectedIdx) > 1 {
		return nil, false, fmt.Errorf("multiple selections not allowed")
	}
	selected := make([]string, 0, len(selectedIdx))
	for idx, option := range question.Options {
		if _, ok := selectedIdx[idx]; !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(option.Title), AskTypeYourAnswerTitle) {
			useCustom = true
			continue
		}
		selected = append(selected, option.Title)
	}
	if len(selected) == 0 && !useCustom {
		return nil, false, fmt.Errorf("invalid selection")
	}
	return selected, useCustom, nil
}

func promptAskCustomAnswer(reader *bufio.Reader) (string, error) {
	fmt.Printf("Enter custom answer (finish with empty line):\n")
	var lines []string
	for {
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		trimmed := strings.TrimRight(input, "\r\n")
		if trimmed == "" {
			break
		}
		lines = append(lines, trimmed)
	}
	return strings.Join(lines, "\n"), nil
}
