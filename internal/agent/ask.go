package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
	adktool "google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type InputToolReq struct {
	Questions []AskQuestion `json:"questions"`
}

type InputToolRsp struct {
	Answers   []AskAnswer `json:"answers,omitempty"`
	Cancelled bool        `json:"cancelled,omitempty"`
	Pending   bool        `json:"pending,omitempty"`
}

type inputFormPayload struct {
	Questions []AskQuestion `json:"questions"`
}

func NewInputTool() (adktool.Tool, error) {
	return functiontool.New(
		functiontool.Config{
			Name:        "request_input",
			Description: "Ask the user to confirm, choose, or provide structured input.",
		},
		func(tc adktool.Context, req *InputToolReq) (*InputToolRsp, error) {
			if len(req.Questions) == 0 {
				return nil, fmt.Errorf("at least one question is required")
			}

			normalized := make([]AskQuestion, 0, len(req.Questions))
			for _, question := range req.Questions {
				norm, err := NormalizeAskQuestion(question)
				if err != nil {
					return nil, err
				}
				normalized = append(normalized, norm)
			}

			if confirmation := tc.ToolConfirmation(); confirmation != nil {
				if !confirmation.Confirmed {
					return &InputToolRsp{Cancelled: true}, nil
				}
				answers, err := parseInputAnswersPayload(confirmation.Payload)
				if err != nil {
					return nil, err
				}
				return &InputToolRsp{Answers: answers}, nil
			}

			if err := tc.RequestConfirmation("User input required.", inputFormPayload{Questions: normalized}); err != nil {
				return nil, err
			}
			return &InputToolRsp{Pending: true}, nil
		},
	)
}

func parseInputAnswersPayload(payload any) ([]AskAnswer, error) {
	if payload == nil {
		return nil, fmt.Errorf("input payload is empty")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var body struct {
		Answers []AskAnswer `json:"answers"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, err
	}
	return body.Answers, nil
}

func NormalizeAskQuestion(question AskQuestion) (AskQuestion, error) {
	question.Question = strings.TrimSpace(question.Question)
	if question.Question == "" {
		return AskQuestion{}, fmt.Errorf("question is required")
	}
	question.Header = strings.TrimSpace(question.Header)
	if question.Header == "" {
		question.Header = truncateRunes(question.Question, AskOptionTitleMaxLen)
	}
	question.ID = strings.TrimSpace(question.ID)

	options := make([]AskOption, 0, len(question.Options)+1)
	for _, option := range question.Options {
		title := truncateRunes(strings.TrimSpace(option.Title), AskOptionTitleMaxLen)
		if title == "" {
			continue
		}
		if strings.EqualFold(title, AskTypeYourAnswerTitle) || strings.EqualFold(title, "Other") {
			continue
		}
		options = append(options, AskOption{
			Title:       title,
			Description: truncateRunes(strings.TrimSpace(option.Description), AskOptionDescMaxLen),
		})
	}
	options = append(options, AskOption{
		Title:       AskTypeYourAnswerTitle,
		Description: AskTypeYourAnswerDesc,
	})
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

type CLIResponder struct {
	reader *bufio.Reader
}

type cliKeyKind int

const (
	cliKeyUnknown cliKeyKind = iota
	cliKeyUp
	cliKeyDown
	cliKeyEnter
	cliKeyTab
	cliKeySpace
	cliKeyBackspace
	cliKeyEscape
	cliKeyRune
)

type cliAskState struct {
	question       AskQuestion
	cursor         int
	selected       map[int]bool
	customSelected bool
	customTexts    []string
	customBuffer   []rune
	customActive   bool
}

func NewCLIResponder() *CLIResponder {
	return &CLIResponder{reader: bufio.NewReader(os.Stdin)}
}

func newCLIAskState(question AskQuestion) *cliAskState {
	return &cliAskState{
		question: question,
		selected: make(map[int]bool),
	}
}

func (r *CLIResponder) Handle(ctx context.Context, request HITLRequest) (HITLResponse, error) {
	switch request.Kind {
	case HITLKindConfirm:
		return r.handleConfirm(ctx, request)
	case HITLKindInputForm:
		return r.handleInputForm(ctx, request)
	default:
		return HITLResponse{}, fmt.Errorf("unsupported hitl request kind %q", request.Kind)
	}
}

func (r *CLIResponder) handleConfirm(ctx context.Context, request HITLRequest) (HITLResponse, error) {
	if ctx.Err() != nil {
		return HITLResponse{}, ctx.Err()
	}
	LockConsoleOutput()
	defer UnlockConsoleOutput()
	confirmText := renderCLIConfirm(request)
	fmt.Print(confirmText)
	if !cliConfirmInline(request) {
		fmt.Print("\n")
		fmt.Print(renderCLIApprovalPrompt())
	}
	line, err := r.reader.ReadString('\n')
	if err != nil {
		return HITLResponse{}, err
	}
	line = strings.ToLower(strings.TrimSpace(line))
	if line == "" || line == "y" || line == "yes" {
		return HITLResponse{Confirmed: true}, nil
	}
	return HITLResponse{
		Confirmed: false,
		Payload: map[string]any{
			"status":  "rejected",
			"reason":  "User rejected this tool call.",
			"message": "Do not execute the tool. Ask the user for an alternative if needed.",
		},
	}, nil
}

func (r *CLIResponder) handleInputForm(ctx context.Context, request HITLRequest) (HITLResponse, error) {
	if ctx.Err() != nil {
		return HITLResponse{}, ctx.Err()
	}

	LockConsoleOutput()
	defer UnlockConsoleOutput()

	if term.IsTerminal(int(os.Stdin.Fd())) {
		response, err := r.handleInteractiveInputForm(ctx, request)
		if err == nil {
			return response, nil
		}
	}

	return r.handleLineInputForm(ctx, request)
}

func (r *CLIResponder) handleLineInputForm(ctx context.Context, request HITLRequest) (HITLResponse, error) {
	if ctx.Err() != nil {
		return HITLResponse{}, ctx.Err()
	}

	answers := make([]AskAnswer, 0, len(request.Questions))
	for _, question := range request.Questions {
		renderCLIQuestion(question)
		for i, option := range question.Options {
			fmt.Println(renderCLIOption(i, option))
		}

		if question.Multiple {
			fmt.Print("\nSelect options (comma separated): ")
		} else {
			fmt.Print("\nSelect an option: ")
		}
		line, err := r.reader.ReadString('\n')
		if err != nil {
			return HITLResponse{}, err
		}
		line = strings.TrimSpace(line)
		answer := AskAnswer{ID: question.ID, Question: question.Question}
		selectedIndexes := parseSelectionIndexes(line, question.Multiple)
		for _, idx := range selectedIndexes {
			if idx < 0 || idx >= len(question.Options) {
				continue
			}
			option := question.Options[idx]
			if strings.EqualFold(option.Title, AskTypeYourAnswerTitle) {
				fmt.Print("Type your answer: ")
				custom, err := r.reader.ReadString('\n')
				if err != nil {
					return HITLResponse{}, err
				}
				custom = strings.TrimSpace(custom)
				if custom != "" {
					answer.CustomTexts = append(answer.CustomTexts, custom)
				}
				continue
			}
			answer.SelectedOptions = append(answer.SelectedOptions, option.Title)
		}
		answers = append(answers, answer)
	}

	return HITLResponse{
		Confirmed: true,
		Answers:   answers,
		Payload: map[string]any{
			"answers": answers,
		},
	}, nil
}

func (r *CLIResponder) handleInteractiveInputForm(ctx context.Context, request HITLRequest) (HITLResponse, error) {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return HITLResponse{}, err
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	answers := make([]AskAnswer, 0, len(request.Questions))
	for _, question := range request.Questions {
		answer, err := r.handleInteractiveQuestion(ctx, question)
		if err != nil {
			return HITLResponse{}, err
		}
		answers = append(answers, answer)
		fmt.Print("\r\n")
	}

	return HITLResponse{
		Confirmed: true,
		Answers:   answers,
		Payload: map[string]any{
			"answers": answers,
		},
	}, nil
}

func (r *CLIResponder) handleInteractiveQuestion(ctx context.Context, question AskQuestion) (AskAnswer, error) {
	state := newCLIAskState(question)
	renderedLines := 0
	render := func() {
		renderedLines = rewriteCLIBlock(renderCLIInteractiveQuestion(state), renderedLines)
	}

	render()
	for {
		if ctx.Err() != nil {
			return AskAnswer{}, ctx.Err()
		}
		key, value, err := readCLIKey(r.reader)
		if err != nil {
			return AskAnswer{}, err
		}
		switch key {
		case cliKeyUp:
			if !state.customActive {
				state.move(-1)
			}
		case cliKeyDown:
			if !state.customActive {
				state.move(1)
			}
		case cliKeyTab:
			if !state.customActive && state.currentIsCustom() {
				state.enterCustom()
			}
		case cliKeySpace:
			if state.customActive {
				state.appendCustomRune(' ')
				break
			}
			if question.Multiple {
				if state.currentIsCustom() {
					if state.hasCustom() {
						state.toggleCurrentSelection()
					}
				} else {
					state.toggleCurrentSelection()
				}
			}
		case cliKeyEnter:
			if state.customActive {
				state.commitCustom()
				render()
				continue
			}
			if question.Multiple {
				render()
				return state.buildAnswer(), nil
			}
			if state.currentIsCustom() {
				if state.hasCustom() {
					state.selectCurrentSingle()
					render()
					return state.buildAnswer(), nil
				}
				break
			}
			state.selectCurrentSingle()
			render()
			return state.buildAnswer(), nil
		case cliKeyBackspace:
			if state.customActive {
				state.backspaceCustom()
			}
		case cliKeyEscape:
			if state.customActive {
				state.cancelCustom()
			}
		case cliKeyRune:
			if state.customActive {
				state.appendCustomRune(value)
			}
		}
		render()
	}
}

func parseSelectionIndexes(input string, multiple bool) []int {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}

	parts := []string{input}
	if multiple {
		parts = strings.Split(input, ",")
	}

	result := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var idx int
		if _, err := fmt.Sscanf(part, "%d", &idx); err == nil && idx > 0 {
			result = append(result, idx-1)
		}
	}
	return result
}

func (s *cliAskState) move(delta int) {
	total := len(s.question.Options)
	if total == 0 {
		s.cursor = 0
		return
	}
	s.cursor = (s.cursor + delta + total) % total
}

func (s *cliAskState) currentIsCustom() bool {
	if s.cursor < 0 || s.cursor >= len(s.question.Options) {
		return false
	}
	return strings.EqualFold(s.question.Options[s.cursor].Title, AskTypeYourAnswerTitle)
}

func (s *cliAskState) hasCustom() bool {
	return len(s.customTexts) > 0
}

func (s *cliAskState) enterCustom() {
	s.customActive = true
	s.customBuffer = []rune(strings.Join(s.customTexts, ", "))
}

func (s *cliAskState) cancelCustom() {
	s.customActive = false
	s.customBuffer = nil
}

func (s *cliAskState) appendCustomRune(value rune) {
	if value == 0 {
		return
	}
	s.customBuffer = append(s.customBuffer, value)
}

func (s *cliAskState) backspaceCustom() {
	if len(s.customBuffer) == 0 {
		return
	}
	s.customBuffer = s.customBuffer[:len(s.customBuffer)-1]
}

func (s *cliAskState) commitCustom() {
	value := strings.TrimSpace(string(s.customBuffer))
	s.customActive = false
	s.customBuffer = nil
	if value == "" {
		s.customTexts = nil
		s.customSelected = false
		return
	}
	s.customTexts = []string{value}
	s.customSelected = true
}

func (s *cliAskState) selectCurrentSingle() {
	clear(s.selected)
	s.customSelected = s.currentIsCustom()
	if !s.currentIsCustom() {
		s.selected[s.cursor] = true
	}
}

func (s *cliAskState) toggleCurrentSelection() {
	if s.currentIsCustom() {
		s.customSelected = !s.customSelected
		return
	}
	if s.selected[s.cursor] {
		delete(s.selected, s.cursor)
		return
	}
	s.selected[s.cursor] = true
}

func (s *cliAskState) buildAnswer() AskAnswer {
	answer := AskAnswer{
		ID:       s.question.ID,
		Question: s.question.Question,
	}
	for idx, option := range s.question.Options {
		if strings.EqualFold(option.Title, AskTypeYourAnswerTitle) {
			continue
		}
		if s.selected[idx] {
			answer.SelectedOptions = append(answer.SelectedOptions, option.Title)
		}
	}
	if s.customSelected {
		answer.CustomTexts = append(answer.CustomTexts, s.customTexts...)
	}
	return answer
}

func readCLIKey(reader *bufio.Reader) (cliKeyKind, rune, error) {
	b, err := reader.ReadByte()
	if err != nil {
		return cliKeyUnknown, 0, err
	}
	switch b {
	case '\r', '\n':
		return cliKeyEnter, 0, nil
	case '\t':
		return cliKeyTab, 0, nil
	case ' ':
		return cliKeySpace, 0, nil
	case 0x7f, 0x08:
		return cliKeyBackspace, 0, nil
	case 0x1b:
		next, err := reader.Peek(2)
		if err == nil && len(next) == 2 && next[0] == '[' {
			_, _ = reader.Discard(2)
			switch next[1] {
			case 'A':
				return cliKeyUp, 0, nil
			case 'B':
				return cliKeyDown, 0, nil
			}
		}
		return cliKeyEscape, 0, nil
	default:
		if err := reader.UnreadByte(); err == nil {
			value, _, readErr := reader.ReadRune()
			if readErr != nil {
				return cliKeyUnknown, 0, readErr
			}
			return cliKeyRune, value, nil
		}
		return cliKeyRune, rune(b), nil
	}
}

func rewriteCLIBlock(content string, previousLines int) int {
	if previousLines > 0 {
		fmt.Print("\r")
		if previousLines > 1 {
			fmt.Printf("\x1b[%dA", previousLines-1)
		}
		fmt.Print("\x1b[J")
	}
	fmt.Print(renderCLIBlockForRawTerminal(content))
	return len(strings.Split(content, "\n"))
}

func renderCLIBlockForRawTerminal(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	return strings.ReplaceAll(content, "\n", "\r\n")
}

func renderCLIInteractiveQuestion(state *cliAskState) string {
	lines := []string{
		"",
		cliTitleStyle.Render(strings.TrimSpace(state.question.Header)),
		strings.TrimSpace(state.question.Question),
	}
	for idx, option := range state.question.Options {
		lines = append(lines, renderCLIInteractiveOption(state, idx, option))
	}
	lines = append(lines, "")
	if state.customActive {
		lines = append(lines, cliSubtitleStyle.Render("Type your answer ")+cliDescStyle.Render("(Enter save, Esc cancel)")+cliSubtitleStyle.Render(": ")+string(state.customBuffer))
	} else {
		hint := "↑/↓ move  Enter confirm"
		if state.question.Multiple {
			hint = "↑/↓ move  Space toggle  Enter submit"
		}
		if state.currentIsCustom() {
			if state.question.Multiple {
				if state.hasCustom() {
					hint = "↑/↓ move  Space toggle custom  Tab edit custom  Enter submit"
				} else {
					hint = "↑/↓ move  Tab edit custom  Enter submit"
				}
			} else {
				if state.hasCustom() {
					hint = "↑/↓ move  Tab edit custom  Enter confirm"
				} else {
					hint = "↑/↓ move  Tab edit custom"
				}
			}
		}
		lines = append(lines, cliSubtitleStyle.Render(hint))
	}
	return strings.Join(lines, "\n")
}

func renderCLIInteractiveOption(state *cliAskState, index int, option AskOption) string {
	titleStyle := lipgloss.NewStyle()
	if index == state.cursor {
		titleStyle = cliTitleStyle
	}
	parts := []string{"  "}
	if state.question.Multiple {
		selected := state.selected[index]
		if strings.EqualFold(option.Title, AskTypeYourAnswerTitle) {
			selected = state.customSelected
		}
		if selected {
			parts = append(parts, "[x] ")
		} else {
			parts = append(parts, "[ ] ")
		}
	}
	title := titleStyle.Render(option.Title)
	if strings.EqualFold(option.Title, AskTypeYourAnswerTitle) && len(state.customTexts) > 0 {
		title += " " + cliDescStyle.Render("("+strings.Join(state.customTexts, ", ")+")")
	}
	parts = append(parts, title)
	if desc := strings.TrimSpace(option.Description); desc != "" {
		parts = append(parts, " "+cliDescStyle.Render(desc))
	}
	return strings.Join(parts, "")
}

var (
	cliTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#5B44E8", Dark: "#7C6BFF"})
	cliSubtitleStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#656D76", Dark: "#8B949E"})
	cliCommandStyle  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#1A8CFF", Dark: "#58A6FF"})
	cliDescStyle     = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#656D76", Dark: "#8B949E"})
)

func renderCLIConfirm(request HITLRequest) string {
	return buildCLIConfirmText(request)
}

func renderCLIQuestion(question AskQuestion) {
	title := strings.TrimSpace(question.Header)
	if title == "" {
		title = "Input Required"
	}
	fmt.Printf("\n%s\n", cliTitleStyle.Render(title))
	if prompt := strings.TrimSpace(question.Question); prompt != "" {
		fmt.Printf("%s\n", prompt)
	}
}

func renderCLIOption(index int, option AskOption) string {
	title := fmt.Sprintf("  [%d] %s", index+1, option.Title)
	if desc := strings.TrimSpace(option.Description); desc != "" {
		return title + " " + cliDescStyle.Render(desc)
	}
	return title
}

func buildCLIConfirmText(request HITLRequest) string {
	if cliConfirmInline(request) {
		return buildCLIInlineConfirmText(request)
	}
	lines := []string{""}
	lines = append(lines, cliTitleStyle.Render(cliConfirmTitle(request)))
	if command := strings.TrimSpace(request.Command); command != "" {
		lines = append(lines, cliCommandStyle.Render("  "+command))
	}
	if meta := cliConfirmMeta(request); meta != "" {
		lines = append(lines, cliSubtitleStyle.Render(meta))
	}
	if prompt := cliConfirmPrompt(request); prompt != "" {
		lines = append(lines, cliDescStyle.Render(prompt))
	}
	lines = append(lines, cliSubtitleStyle.Render("Allow [Enter/Y]  Reject [N/Esc]"))
	return strings.Join(lines, "\n")
}

func buildCLIInlineConfirmText(request HITLRequest) string {
	command := strings.TrimSpace(request.Command)
	if command == "" {
		return cliTitleStyle.Render(cliConfirmTitle(request)) + " " + cliSubtitleStyle.Render("(y/n): ")
	}
	return "\n" + cliTitleStyle.Render("Allow command") + " " + cliCommandStyle.Render(command) + " " + cliSubtitleStyle.Render("(y/n): ")
}

func cliConfirmInline(request HITLRequest) bool {
	return strings.TrimSpace(request.Command) != ""
}

func cliConfirmTitle(request HITLRequest) string {
	if strings.TrimSpace(request.Command) != "" {
		return "Allow command"
	}
	if title := strings.TrimSpace(request.Title); title != "" && !strings.EqualFold(title, "Confirmation Required") {
		return title
	}
	return "Allow action"
}

func cliConfirmMeta(request HITLRequest) string {
	parts := make([]string, 0, 2)
	if cwd := strings.TrimSpace(request.Cwd); cwd != "" {
		parts = append(parts, cwd)
	}
	if tool := strings.TrimSpace(request.OriginalTool); tool != "" && !strings.EqualFold(tool, "command") {
		parts = append(parts, "tool "+tool)
	}
	return strings.Join(parts, "  ")
}

func cliConfirmPrompt(request HITLRequest) string {
	if note := strings.TrimSpace(request.RiskNote); note != "" {
		return note
	}
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return ""
	}
	if strings.EqualFold(prompt, "Approval required.") {
		return ""
	}
	lower := strings.ToLower(prompt)
	if strings.Contains(lower, "approve or reject the tool call") {
		return ""
	}
	return prompt
}

func renderCLIApprovalPrompt() string {
	return cliSubtitleStyle.Render("Choice: ")
}
