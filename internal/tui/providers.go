package tui

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/llm"
	"github.com/termia/termia/internal/providerpolicy"
)

func loadProviderModelsCmd(
	listModelsFn func(context.Context, config.ProviderMeta) ([]llm.ModelDescriptor, error),
	meta config.ProviderMeta,
) tea.Cmd {
	return func() tea.Msg {
		models, err := listModelsFn(context.Background(), meta)
		if err != nil {
			return providerModelsErrorMsg{provider: meta.ID, err: err}
		}
		return providerModelsLoadedMsg{provider: meta.ID, models: models}
	}
}

func (a *App) beginProviderModelsLoad(provider string) tea.Cmd {
	meta, ok := a.providerSvc.ProviderMeta(provider)
	if !ok {
		return nil
	}
	delete(a.providerModelErrors, meta.ID)
	return loadProviderModelsCmd(a.listModelsFn, meta)
}

func (a *App) beginModelsPaletteLoad() tea.Cmd {
	providers := a.modelPaletteProviders()
	if len(providers) == 0 {
		return nil
	}
	cmds := make([]tea.Cmd, 0, len(providers))
	for _, provider := range providers {
		cmds = append(cmds, a.beginProviderModelsLoad(provider.ID))
	}
	return tea.Batch(cmds...)
}

func (a App) providerPaletteItems() []paletteItem {
	providers := a.providerSvc.ManageableProviders()
	items := make([]paletteItem, 0, len(providers))
	for _, provider := range providers {
		items = append(items, paletteItem{
			Label:    provider.DisplayName,
			Desc:     a.providerPaletteDescription(provider),
			Action:   paletteActionOpenProvider,
			Value:    provider.ID,
			Provider: provider.ID,
		})
	}
	return items
}

func (a App) providerPaletteSections() []paletteSection {
	sections := []paletteSection{
		{Label: "Providers", Items: a.providerPaletteItems()},
		{
			Label: "Create",
			Items: []paletteItem{
				{
					Label:  "OpenAI Compatible",
					Desc:   "Create a custom provider",
					Action: paletteActionCreateProvider,
					Value:  config.ProviderOpenAICompatible,
				},
			},
		},
	}
	return sections
}

func (a App) providerPaletteDescription(provider config.ProviderMeta) string {
	parts := make([]string, 0, 3)
	if providerpolicy.NormalizeProviderName(a.cfg.LLM.DefaultProvider) == providerpolicy.NormalizeProviderName(provider.ID) {
		parts = append(parts, "current")
	}
	if provider.Custom {
		parts = append(parts, providerpolicy.ProviderDisplayName(provider.Kind))
	}
	if err := a.providerSvc.ValidateProvider(provider); err != nil {
		parts = append(parts, providerConfigErrorSummary(err))
	} else {
		parts = append(parts, "configured")
	}
	return strings.Join(parts, " • ")
}

func providerConfigErrorSummary(err error) string {
	if err == nil {
		return ""
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(text, "api key is required"):
		return "API key missing"
	case strings.Contains(text, "base url is required"):
		return "base URL missing"
	case strings.Contains(text, "invalid base url"):
		return "base URL invalid"
	default:
		return strings.TrimSpace(err.Error())
	}
}

func providerActionError(err error) string {
	if err == nil {
		return ""
	}
	text := strings.TrimSpace(err.Error())
	switch strings.ToLower(text) {
	case "unsupported provider":
		return "Unsupported provider."
	case "unsupported field":
		return "Unsupported field."
	case "provider not found":
		return "Provider not found."
	case "only custom providers can be deleted":
		return "Only custom providers can be deleted."
	default:
		return fmt.Sprintf("Error: %s", text)
	}
}

func (a App) providerDetailTitle() string {
	if strings.TrimSpace(a.activePaletteProvider) == "" {
		return "Provider"
	}
	return a.providerSvc.ProviderDisplayName(a.activePaletteProvider)
}

func (a App) providerDetailPaletteItems() []paletteItem {
	return flattenPaletteSections(a.providerDetailSections())
}

func (a App) providerDetailSections() []paletteSection {
	meta, ok := a.providerSvc.ProviderMeta(a.activePaletteProvider)
	if !ok {
		return nil
	}

	sections := []paletteSection{
		{
			Label: "Navigation",
			Items: []paletteItem{{Label: "Back to Providers", Action: paletteActionBackToProviders}},
		},
	}

	fields := a.providerSvc.ConfigFields(meta.Kind)
	if len(fields) > 0 {
		items := make([]paletteItem, 0, len(fields))
		for _, field := range fields {
			items = append(items, paletteItem{
				Label:    field.Label,
				Desc:     a.providerFieldDisplayValue(meta.ID, field.Field),
				Action:   paletteActionEditProviderField,
				Provider: meta.ID,
				Field:    field.Field,
			})
		}
		sections = append(sections, paletteSection{Label: "Credentials", Items: items})
	}

	actionItems := make([]paletteItem, 0, len(fields)+1)
	for _, field := range fields {
		if strings.TrimSpace(a.providerFieldRawValue(meta.ID, field.Field)) == "" {
			continue
		}
		actionItems = append(actionItems, paletteItem{
			Label:    "Clear " + providerFieldLabel(field.Field),
			Action:   paletteActionClearProviderField,
			Provider: meta.ID,
			Field:    field.Field,
		})
	}
	if meta.Custom {
		actionItems = append(actionItems, paletteItem{
			Label:    "Delete Provider",
			Desc:     "Remove this custom provider",
			Action:   paletteActionDeleteProvider,
			Provider: meta.ID,
			Value:    meta.ID,
		})
	}
	if len(actionItems) > 0 {
		sections = append(sections, paletteSection{Label: "Actions", Items: actionItems})
	}

	return sections
}

func (a App) modelPaletteItems() []paletteItem {
	return flattenPaletteSections(a.modelPaletteSections())
}

func (a App) modelPaletteSections() []paletteSection {
	providers := a.modelPaletteProviders()
	if len(providers) == 0 {
		return []paletteSection{
			{Label: "Models", Items: []paletteItem{{Label: "No configured providers", Desc: "Configure Providers first", Action: paletteActionNoop}}},
		}
	}

	sections := make([]paletteSection, 0, len(providers))
	for _, provider := range providers {
		sections = append(sections, paletteSection{
			Label: provider.DisplayName,
			Items: a.providerModelItems(provider),
		})
	}
	return sections
}

func (a App) modelPaletteProviders() []config.ProviderMeta {
	providers := a.providerSvc.ModelProviders()
	configured := make([]config.ProviderMeta, 0, len(providers))
	for _, provider := range providers {
		if err := a.providerSvc.ValidateModelCatalog(provider); err != nil {
			continue
		}
		configured = append(configured, provider)
	}
	return configured
}

func (a App) providerModelItems(provider config.ProviderMeta) []paletteItem {
	providerID := providerpolicy.NormalizeProviderName(provider.ID)
	if providerID == "" {
		return nil
	}
	if a.providerModelLoading[providerID] {
		return []paletteItem{{Label: "Loading models...", Action: paletteActionNoop}}
	}
	if errText := strings.TrimSpace(a.providerModelErrors[providerID]); errText != "" {
		return []paletteItem{{Label: providerConfigErrorSummary(fmt.Errorf("%s", errText)), Desc: errText, Action: paletteActionNoop}}
	}

	models := a.providerModels[providerID]
	if len(models) == 0 {
		return []paletteItem{{Label: "No models available", Action: paletteActionNoop}}
	}

	providerCfg, _ := a.providerSvc.ProviderConfig(providerID)
	isCurrentProvider := providerpolicy.NormalizeProviderName(a.cfg.LLM.DefaultProvider) == providerID
	items := make([]paletteItem, 0, len(models))
	for _, model := range models {
		modelID := strings.TrimSpace(model.ID)
		modelLabel := strings.TrimSpace(model.DisplayName)
		if modelLabel == "" {
			modelLabel = modelID
		}
		if modelID == "" && modelLabel == "" {
			continue
		}
		parts := make([]string, 0, 3)
		if strings.TrimSpace(providerCfg.Model) == modelID {
			parts = append(parts, "configured")
			if isCurrentProvider {
				parts = append(parts, "current")
			}
		}
		if capability := formatThinkingCapability(model); capability != "" {
			parts = append(parts, capability)
		}
		items = append(items, paletteItem{
			Label:    modelLabel,
			Desc:     strings.Join(parts, " • "),
			Action:   paletteActionSelectModel,
			Value:    modelID,
			Provider: providerID,
		})
	}
	return items
}

func formatThinkingCapability(model llm.ModelDescriptor) string {
	switch model.ThinkingSupport {
	case llm.ThinkingSupportUnsupported:
		return "no thinking"
	case llm.ThinkingSupportSupported:
		if len(model.ThinkingLevels) == 0 {
			return "thinking"
		}
		return "thinking: " + strings.Join(model.ThinkingLevels, "/")
	default:
		return ""
	}
}

func (a *App) openProviderConfigPrompt(provider string, field llm.ProviderConfigField) {
	a.providerConfigOpen = true
	a.providerConfigProvider = providerpolicy.NormalizeProviderName(provider)
	a.providerConfigField = field
	a.providerConfigError = ""
	a.providerConfigInput.SetValue(a.providerFieldRawValue(provider, field))
	a.providerConfigInput.Placeholder = a.providerFieldPlaceholder(provider, field)
	a.providerConfigInput.Prompt = "> "
	a.providerConfigInput.EchoMode = textinput.EchoNormal
	if field == llm.ProviderFieldAPIKey {
		a.providerConfigInput.EchoMode = textinput.EchoPassword
		a.providerConfigInput.EchoCharacter = '•'
	}
	a.providerConfigInput.Focus()
	a.providerConfigInput.CursorEnd()
}

func (a *App) closeProviderConfigPrompt() {
	a.providerConfigOpen = false
	a.providerConfigProvider = ""
	a.providerConfigField = ""
	a.providerConfigError = ""
	a.providerConfigInput.SetValue("")
	a.providerConfigInput.Blur()
	a.providerConfigInput.EchoMode = textinput.EchoNormal
}

func (a App) handleProviderConfigKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		provider := a.providerConfigProvider
		a.closeProviderConfigPrompt()
		a.openProviderPalette(provider)
		return a, nil
	case tea.KeyEnter:
		return a.submitProviderConfigPrompt()
	}

	var cmd tea.Cmd
	a.providerConfigInput, cmd = a.providerConfigInput.Update(msg)
	return a, cmd
}

func (a App) submitProviderConfigPrompt() (tea.Model, tea.Cmd) {
	provider := providerpolicy.NormalizeProviderName(a.providerConfigProvider)
	field := a.providerConfigField
	value := strings.TrimSpace(a.providerConfigInput.Value())

	if field == llm.ProviderFieldBaseURL && value != "" {
		if _, err := url.ParseRequestURI(value); err != nil {
			a.providerConfigError = fmt.Sprintf("Invalid base URL: %v", err)
			return a, nil
		}
	}

	if err := a.providerSvc.UpdateProviderField(provider, field, value); err != nil {
		a.providerConfigError = providerActionError(err)
		return a, nil
	}

	delete(a.providerModels, provider)
	delete(a.providerModelErrors, provider)
	delete(a.providerModelLoading, provider)
	a.closeProviderConfigPrompt()
	a.openProviderPalette(provider)
	return a, a.setTransientStatus(fmt.Sprintf("%s updated.", a.providerFieldTitle(provider, field)))
}

func (a App) renderProviderConfigPrompt(totalWidth int) string {
	if totalWidth <= 0 {
		return ""
	}
	promptWidth := totalWidth - 6
	if promptWidth > 72 {
		promptWidth = 72
	}
	if promptWidth < 36 {
		promptWidth = totalWidth
	}

	frameW, _ := commandPaletteStyle.GetFrameSize()
	contentWidth := promptWidth - frameW
	if contentWidth < 10 {
		contentWidth = 10
	}

	input := a.providerConfigInput
	promptCells := lipgloss.Width(input.Prompt)
	input.Width = maxInt(contentWidth-promptCells, suggestedMinWidth)
	inputLine := input.View()

	lines := []string{
		a.renderProviderConfigHeader(contentWidth),
		metaStyle.Render("Current: " + a.providerFieldDisplayValue(a.providerConfigProvider, a.providerConfigField)),
		inputLine,
		metaStyle.Render("Enter to save, Esc to cancel"),
	}
	if strings.TrimSpace(a.providerConfigError) != "" {
		lines = append(lines, dirPromptErrorStyle.Render(a.providerConfigError))
	}

	content := strings.Join(lines, "\n")
	return commandPaletteStyle.Width(promptWidth).Render(content)
}

func (a App) renderProviderConfigHeader(width int) string {
	left := paletteHeaderTitleStyle.Render(a.providerFieldTitle(a.providerConfigProvider, a.providerConfigField))
	right := metaStyle.Render("Esc")
	return buildStatusLine(left, right, width)
}

func (a *App) openCustomProviderPrompt() {
	a.customProviderOpen = true
	a.customProviderError = ""
	a.customProviderNameInput.SetValue("")
	a.customProviderAPIKeyInput.SetValue("")
	a.customProviderBaseURLInput.SetValue("")
	a.customProviderNameInput.Placeholder = "My Provider"
	a.customProviderAPIKeyInput.Placeholder = ""
	a.customProviderBaseURLInput.Placeholder = "https://api.example.com/v1"
	a.customProviderNameInput.Prompt = "Provider Name: "
	a.customProviderAPIKeyInput.Prompt = "API Key: "
	a.customProviderBaseURLInput.Prompt = "API Base URL: "
	a.customProviderAPIKeyInput.EchoMode = textinput.EchoPassword
	a.customProviderAPIKeyInput.EchoCharacter = '•'
	a.setCustomProviderFocus(customProviderFieldName)
}

func (a *App) closeCustomProviderPrompt() {
	a.customProviderOpen = false
	a.customProviderError = ""
	a.customProviderNameInput.SetValue("")
	a.customProviderAPIKeyInput.SetValue("")
	a.customProviderBaseURLInput.SetValue("")
	a.customProviderNameInput.Blur()
	a.customProviderAPIKeyInput.Blur()
	a.customProviderBaseURLInput.Blur()
}

func (a *App) setCustomProviderFocus(field customProviderField) {
	a.customProviderFocus = field
	a.customProviderNameInput.Blur()
	a.customProviderAPIKeyInput.Blur()
	a.customProviderBaseURLInput.Blur()

	switch field {
	case customProviderFieldName:
		a.customProviderNameInput.Focus()
		a.customProviderNameInput.CursorEnd()
	case customProviderFieldAPIKey:
		a.customProviderAPIKeyInput.Focus()
		a.customProviderAPIKeyInput.CursorEnd()
	default:
		a.customProviderBaseURLInput.Focus()
		a.customProviderBaseURLInput.CursorEnd()
	}
}

func (a *App) moveCustomProviderFocus(delta int) {
	count := 3
	index := int(a.customProviderFocus) + delta
	for index < 0 {
		index += count
	}
	a.setCustomProviderFocus(customProviderField(index % count))
}

func (a *App) activeCustomProviderInput() *textinput.Model {
	switch a.customProviderFocus {
	case customProviderFieldName:
		return &a.customProviderNameInput
	case customProviderFieldAPIKey:
		return &a.customProviderAPIKeyInput
	default:
		return &a.customProviderBaseURLInput
	}
}

func (a App) handleCustomProviderKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		a.closeCustomProviderPrompt()
		a.openPaletteStage(paletteStageProviders)
		return a, nil
	case tea.KeyUp:
		a.moveCustomProviderFocus(-1)
		return a, nil
	case tea.KeyDown, tea.KeyTab:
		a.moveCustomProviderFocus(1)
		return a, nil
	case tea.KeyEnter:
		return a.submitCustomProviderPrompt()
	}

	input := a.activeCustomProviderInput()
	if input == nil {
		return a, nil
	}
	var cmd tea.Cmd
	*input, cmd = input.Update(msg)
	return a, cmd
}

func (a App) submitCustomProviderPrompt() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(a.customProviderNameInput.Value())
	apiKey := strings.TrimSpace(a.customProviderAPIKeyInput.Value())
	baseURL := strings.TrimSpace(a.customProviderBaseURLInput.Value())

	if name == "" {
		a.customProviderError = "Provider name is required."
		return a, nil
	}
	if apiKey == "" {
		a.customProviderError = "API key is required."
		return a, nil
	}
	if baseURL == "" {
		a.customProviderError = "API base URL is required."
		return a, nil
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		a.customProviderError = fmt.Sprintf("Invalid base URL: %v", err)
		return a, nil
	}

	custom, err := a.providerSvc.CreateCustomProvider(name, apiKey, baseURL)
	if err != nil {
		a.customProviderError = err.Error()
		return a, nil
	}

	a.closeCustomProviderPrompt()
	a.openProviderPalette(custom.ID)
	return a, a.setTransientStatus(fmt.Sprintf("%s created.", strings.TrimSpace(custom.Name)))
}

func (a App) renderCustomProviderPrompt(totalWidth int) string {
	if totalWidth <= 0 {
		return ""
	}
	promptWidth := totalWidth - 6
	if promptWidth > 84 {
		promptWidth = 84
	}
	if promptWidth < 42 {
		promptWidth = totalWidth
	}

	frameW, _ := commandPaletteStyle.GetFrameSize()
	contentWidth := promptWidth - frameW
	if contentWidth < 16 {
		contentWidth = 16
	}

	nameInput := a.customProviderNameInput
	nameInput.Width = maxInt(contentWidth-lipgloss.Width(nameInput.Prompt), 8)
	apiKeyInput := a.customProviderAPIKeyInput
	apiKeyInput.Width = maxInt(contentWidth-lipgloss.Width(apiKeyInput.Prompt), 8)
	baseURLInput := a.customProviderBaseURLInput
	baseURLInput.Width = maxInt(contentWidth-lipgloss.Width(baseURLInput.Prompt), 8)

	lines := []string{
		a.renderCustomProviderHeader(contentWidth),
		nameInput.View(),
		apiKeyInput.View(),
		baseURLInput.View(),
		metaStyle.Render("Tab or Up/Down to switch fields, Enter to save, Esc to cancel"),
	}
	if strings.TrimSpace(a.customProviderError) != "" {
		lines = append(lines, dirPromptErrorStyle.Render(a.customProviderError))
	}

	content := strings.Join(lines, "\n")
	return commandPaletteStyle.Width(promptWidth).Render(content)
}

func (a App) renderCustomProviderHeader(width int) string {
	left := paletteHeaderTitleStyle.Render("New OpenAI Compatible Provider")
	right := metaStyle.Render("Esc")
	return buildStatusLine(left, right, width)
}

func (a App) providerFieldTitle(provider string, field llm.ProviderConfigField) string {
	return fmt.Sprintf("%s %s", a.providerSvc.ProviderDisplayName(provider), providerFieldLabel(field))
}

func providerFieldLabel(field llm.ProviderConfigField) string {
	switch field {
	case llm.ProviderFieldAPIKey:
		return "API Key"
	case llm.ProviderFieldBaseURL:
		return "Base URL"
	default:
		return strings.TrimSpace(string(field))
	}
}

func (a App) providerFieldPlaceholder(provider string, field llm.ProviderConfigField) string {
	value := a.providerFieldDisplayValue(provider, field)
	if strings.TrimSpace(value) == "" || value == "(not set)" {
		return ""
	}
	return value
}

func (a App) providerFieldRawValue(provider string, field llm.ProviderConfigField) string {
	return a.providerSvc.ProviderFieldRawValue(provider, field)
}

func (a App) providerFieldDisplayValue(provider string, field llm.ProviderConfigField) string {
	return a.providerSvc.ProviderFieldDisplayValue(provider, field)
}

func maskSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "(not set)"
	}
	runes := []rune(value)
	if len(runes) <= 8 {
		return strings.Repeat("•", len(runes))
	}
	return string(runes[:4]) + strings.Repeat("•", len(runes)-8) + string(runes[len(runes)-4:])
}

func (a App) clearProviderField(provider string, field llm.ProviderConfigField) (tea.Model, tea.Cmd) {
	provider = providerpolicy.NormalizeProviderName(provider)
	if err := a.providerSvc.ClearProviderField(provider, field); err != nil {
		return a, a.setTransientStatus(providerActionError(err))
	}
	delete(a.providerModels, provider)
	delete(a.providerModelErrors, provider)
	delete(a.providerModelLoading, provider)
	return a, a.setTransientStatus(fmt.Sprintf("%s cleared.", a.providerFieldTitle(provider, field)))
}

func (a App) deleteCustomProvider(provider string) (tea.Model, tea.Cmd) {
	provider = providerpolicy.NormalizeProviderName(provider)
	meta, err := a.providerSvc.DeleteCustomProvider(provider)
	if err != nil {
		return a, a.setTransientStatus(providerActionError(err))
	}
	delete(a.providerModels, provider)
	delete(a.providerModelErrors, provider)
	delete(a.providerModelLoading, provider)
	a.syncThinkLevelForCurrentModel()
	a.openPaletteStage(paletteStageProviders)
	return a, a.setTransientStatus(fmt.Sprintf("%s deleted.", meta.DisplayName))
}

func (a App) selectProviderModel(provider string, modelID string) (tea.Model, tea.Cmd) {
	provider = providerpolicy.NormalizeProviderName(provider)
	if err := a.providerSvc.SelectModel(provider, modelID); err != nil {
		return a, a.setTransientStatus(providerActionError(err))
	}

	a.syncThinkLevelForCurrentModel()
	a.closePalette()
	return a, a.setTransientStatus(fmt.Sprintf("Model switched to %s (%s).", strings.TrimSpace(modelID), a.providerSvc.ProviderDisplayName(provider)))
}

func (a App) currentModelDescriptor() (llm.ModelDescriptor, bool) {
	return a.providerSvc.CurrentModelDescriptor(a.providerModels)
}

func (a App) currentThinkingLevels() []ThinkLevel {
	model, ok := a.currentModelDescriptor()
	if !ok || model.ThinkingSupport != llm.ThinkingSupportSupported || len(model.ThinkingLevels) == 0 {
		return nil
	}

	levels := make([]ThinkLevel, 0, len(model.ThinkingLevels))
	seen := make(map[ThinkLevel]struct{}, len(model.ThinkingLevels))
	for _, level := range model.ThinkingLevels {
		thinkLevel, ok := parseThinkLevel(level)
		if !ok {
			continue
		}
		if _, exists := seen[thinkLevel]; exists {
			continue
		}
		seen[thinkLevel] = struct{}{}
		levels = append(levels, thinkLevel)
	}
	return levels
}

func parseThinkLevel(value string) (ThinkLevel, bool) {
	switch llm.NormalizeThinkingLevel(value) {
	case "low":
		return ThinkLow, true
	case "medium":
		return ThinkMedium, true
	case "high":
		return ThinkHigh, true
	default:
		return ThinkLow, false
	}
}

func thinkLevelConfigValue(level ThinkLevel) string {
	switch level {
	case ThinkLow:
		return "low"
	case ThinkHigh:
		return "high"
	default:
		return "medium"
	}
}

func (a App) currentConfiguredThinkLevel() (ThinkLevel, bool) {
	return parseThinkLevel(a.providerSvc.CurrentConfiguredThinkingLevel())
}

func (a *App) persistCurrentThinkLevel() bool {
	if err := a.providerSvc.PersistCurrentThinkingLevel(thinkLevelConfigValue(a.thinkLevel)); err != nil {
		a.setStatusMsg(providerActionError(err))
		return false
	}
	return true
}

func (a *App) syncThinkLevelForCurrentModel() {
	levels := a.currentThinkingLevels()
	if len(levels) == 0 {
		return
	}
	if configured, ok := a.currentConfiguredThinkLevel(); ok {
		for _, level := range levels {
			if level == configured {
				a.thinkLevel = level
				return
			}
		}
	}
	for _, level := range levels {
		if level == a.thinkLevel {
			return
		}
	}
	if preferred, ok := parseThinkLevel(llm.DefaultThinkingLevel(providerpolicy.NormalizeProviderName(a.cfg.LLM.DefaultProvider), a.currentModelLabel())); ok {
		for _, level := range levels {
			if level == preferred {
				a.thinkLevel = level
				return
			}
		}
	}
	a.thinkLevel = levels[0]
}

func flattenPaletteSections(sections []paletteSection) []paletteItem {
	items := make([]paletteItem, 0, len(sections)*4)
	for _, section := range sections {
		items = append(items, paletteItem{Label: section.Label, Header: true})
		items = append(items, section.Items...)
	}
	return items
}
