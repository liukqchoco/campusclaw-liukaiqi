package httpapi

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"campusclaw/backend/internal/store"
)

var (
	skillSeparatorPattern = regexp.MustCompile(`[，,、；;|]+`)
	serverNamePattern     = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
)

func parseSkillFile(value string) (store.Skill, error) {
	if !utf8.ValidString(value) {
		return store.Skill{}, errors.New("Skill 文件必须是 UTF-8 文本")
	}
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	if len([]rune(value)) > 8000 {
		return store.Skill{}, errors.New("Skill 文件不能超过 8000 字")
	}
	lines := strings.Split(value, "\n")
	separator := -1
	for index, line := range lines {
		if strings.TrimSpace(line) == "" {
			separator = index
			break
		}
	}
	if separator < 0 {
		return store.Skill{}, errors.New("元数据与说明正文之间必须有一个空行")
	}

	fields := make(map[string]string)
	allowed := map[string]struct{}{
		"名称": {}, "适用条件": {}, "资料": {}, "工具": {}, "启用": {},
	}
	for _, raw := range lines[:separator] {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, content, ok := cutMetadataLine(line)
		if !ok {
			return store.Skill{}, fmt.Errorf("无法解析元数据行：%s", line)
		}
		if _, exists := allowed[key]; !exists {
			return store.Skill{}, fmt.Errorf("未知字段：%s", key)
		}
		if _, duplicate := fields[key]; duplicate {
			return store.Skill{}, fmt.Errorf("字段重复：%s", key)
		}
		fields[key] = strings.TrimSpace(content)
	}
	for _, required := range []string{"名称", "适用条件", "启用"} {
		if strings.TrimSpace(fields[required]) == "" {
			return store.Skill{}, fmt.Errorf("缺少必填字段：%s", required)
		}
	}
	body := strings.TrimSpace(strings.Join(lines[separator+1:], "\n"))
	if body == "" {
		return store.Skill{}, errors.New("Skill 说明正文不能为空")
	}
	if len([]rune(fields["名称"])) > 32 {
		return store.Skill{}, errors.New("名称不能超过 32 字")
	}
	if len([]rune(fields["适用条件"])) > 120 {
		return store.Skill{}, errors.New("适用条件不能超过 120 字")
	}
	if len([]rune(fields["资料"])) > 32 {
		return store.Skill{}, errors.New("资料范围不能超过 32 字")
	}
	if len([]rune(body)) > 2000 {
		return store.Skill{}, errors.New("Skill 说明正文不能超过 2000 字")
	}

	var enabled bool
	switch strings.ToLower(fields["启用"]) {
	case "是", "true", "yes", "1":
		enabled = true
	case "否", "false", "no", "0":
		enabled = false
	default:
		return store.Skill{}, errors.New("启用字段只能填写“是”或“否”")
	}
	tools, err := parseSkillTools(fields["工具"])
	if err != nil {
		return store.Skill{}, err
	}
	return store.Skill{
		Name:        fields["名称"],
		TriggerText: fields["适用条件"],
		Description: body,
		Tools:       tools,
		SourceText:  value,
		SubjectText: fields["资料"],
		Enabled:     enabled,
		BuiltIn:     false,
	}, nil
}

func cutMetadataLine(line string) (string, string, bool) {
	if key, value, ok := strings.Cut(line, "："); ok {
		return strings.TrimSpace(key), value, true
	}
	if key, value, ok := strings.Cut(line, ":"); ok {
		return strings.TrimSpace(key), value, true
	}
	return "", "", false
}

func parseSkillTools(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return []string{}, nil
	}
	rawItems := skillSeparatorPattern.Split(value, -1)
	seen := make(map[string]struct{}, len(rawItems))
	tools := make([]string, 0, len(rawItems))
	for _, raw := range rawItems {
		item := strings.TrimSpace(raw)
		if item == "" {
			continue
		}
		if item != "knowledge_search" {
			toolName, serverName, ok := strings.Cut(item, "@")
			if !ok || !validToolName(toolName) || !serverNamePattern.MatchString(serverName) {
				return nil, fmt.Errorf("非法工具引用：%s", item)
			}
		}
		if _, duplicate := seen[item]; duplicate {
			continue
		}
		seen[item] = struct{}{}
		tools = append(tools, item)
	}
	return tools, nil
}

type skillRouteCandidate struct {
	ID      int64    `json:"id"`
	Name    string   `json:"name"`
	Score   int      `json:"score"`
	Matched []string `json:"matched"`
}

type skillRouteResult struct {
	Selected   *store.Skill          `json:"-"`
	SelectedID *int64                `json:"selected_id,omitempty"`
	Name       string                `json:"selected_name,omitempty"`
	Candidates []skillRouteCandidate `json:"candidates"`
	Rule       string                `json:"rule"`
}

func routeSkill(question string, skills []store.Skill) skillRouteResult {
	normalized := strings.ToLower(question)
	result := skillRouteResult{
		Candidates: make([]skillRouteCandidate, 0, len(skills)),
		Rule:       "仅在当前应用绑定且启用的 Skill 中按适用条件词法计分；零分不选，同分取编号较小者，一次至多一个",
	}
	sorted := append([]store.Skill(nil), skills...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	bestScore := 0
	for index := range sorted {
		terms := skillSeparatorPattern.Split(sorted[index].TriggerText, -1)
		candidate := skillRouteCandidate{ID: sorted[index].ID, Name: sorted[index].Name}
		for _, raw := range terms {
			term := strings.ToLower(strings.TrimSpace(raw))
			if term != "" && strings.Contains(normalized, term) {
				candidate.Score++
				candidate.Matched = append(candidate.Matched, strings.TrimSpace(raw))
			}
		}
		result.Candidates = append(result.Candidates, candidate)
		if candidate.Score > bestScore {
			bestScore = candidate.Score
			selected := sorted[index]
			result.Selected = &selected
			id := selected.ID
			result.SelectedID = &id
			result.Name = selected.Name
		}
	}
	if bestScore == 0 {
		result.Selected = nil
		result.SelectedID = nil
		result.Name = ""
	}
	return result
}

func renderPromptTemplate(template string, variables map[string]string) string {
	rendered := template
	for key, value := range variables {
		rendered = strings.ReplaceAll(rendered, "{{"+key+"}}", value)
	}
	return strings.TrimSpace(rendered)
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}
