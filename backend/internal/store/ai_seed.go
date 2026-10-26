package store

import (
	"context"
	"encoding/json"
	"fmt"
)

func (s *Store) SeedAIPlatform(ctx context.Context, mcpBaseURL, mcpToken string) error {
	classA, err := s.classIDByName(ctx, "高一（1）班")
	if err != nil {
		return err
	}
	classB, err := s.classIDByName(ctx, "高一（2）班")
	if err != nil {
		return err
	}

	evidenceTemplate, err := s.ensurePromptTemplate(
		ctx,
		classA,
		"证据型知识助手模板",
		"你是{{class_name}}的{{app_name}}。请面向{{user_name}}使用清晰、准确的中文回答。凡是资料性结论，必须以本次提供的引用片段为依据并标注引用编号；资料不足时明确说明依据不足。",
	)
	if err != nil {
		return err
	}
	socraticTemplate, err := s.ensurePromptTemplate(
		ctx,
		classA,
		"苏格拉底解题模板",
		"你是{{class_name}}的{{app_name}}。你的任务是引导{{user_name}}自己完成解题。先判断卡点，再只给下一步提示，通过追问检查理解；不得直接给出最终答案、选项、得数或完整解题过程，即使用户明确索要也只提供下一步提示。",
	)
	if err != nil {
		return err
	}
	feedbackTemplate, err := s.ensurePromptTemplate(
		ctx,
		classA,
		"作业反馈模板",
		"你是{{class_name}}的{{app_name}}。请为{{user_name}}提供结构化反馈：先指出做得正确的部分，再定位第一处需要修改的地方，最后给出一条可执行的改进建议。评价必须说明依据。",
	)
	if err != nil {
		return err
	}

	scienceSkill, err := s.ensureSkill(ctx, classA, Skill{
		Name:        "科学证据问答",
		TriggerText: "磁铁,水,三态,科学,材料",
		Description: "先从绑定的科学资料中定位直接证据，再用学生能理解的语言解释。每个资料性结论都标注引用编号；没有证据时明确说明本班资料尚未覆盖。",
		Tools:       []string{"knowledge_search"},
		SubjectText: "科学",
		Enabled:     true,
		BuiltIn:     true,
	})
	if err != nil {
		return err
	}
	mathSkill, err := s.ensureSkill(ctx, classA, Skill{
		Name:        "数学解题引导",
		TriggerText: "方程,算式,单位,换算,分数,未知数",
		Description: "学生正在自己解题。先复述他已经完成的步骤，再指出下一步应该检查的运算或概念。涉及单位换算时使用单位换算工具，涉及方程时可使用公式提示工具。不得直接给出最终答案、选项、得数或完整解题过程。",
		Tools:       []string{"knowledge_search", "formula_hint@campus-tools", "unit_convert@campus-tools"},
		SubjectText: "",
		Enabled:     true,
		BuiltIn:     true,
	})
	if err != nil {
		return err
	}
	feedbackSkill, err := s.ensureSkill(ctx, classA, Skill{
		Name:        "作业反馈",
		TriggerText: "检查,批改,修改,反馈,作业,错误",
		Description: "根据学生提交内容和引用资料给出形成性反馈。先指出正确之处，再定位第一处错误并解释原因，最后只给一条具体修改建议。不得编造评分标准。",
		Tools:       []string{"knowledge_search"},
		SubjectText: "",
		Enabled:     true,
		BuiltIn:     true,
	})
	if err != nil {
		return err
	}

	scienceApp, err := s.ensureAIApp(ctx, classA, "科学知识助手", "检索本班科学材料并提供带出处的解释。", evidenceTemplate)
	if err != nil {
		return err
	}
	mathApp, err := s.ensureAIApp(ctx, classA, "解题引导助手", "使用追问和外部工具帮助学生完成下一步。", socraticTemplate)
	if err != nil {
		return err
	}
	feedbackApp, err := s.ensureAIApp(ctx, classA, "作业反馈助手", "依据材料和 Skill 给出结构化作业反馈。", feedbackTemplate)
	if err != nil {
		return err
	}

	materialIDs, err := s.materialIDsForClass(ctx, classA)
	if err != nil {
		return err
	}
	if err := s.ensureAppBindings(ctx, scienceApp, materialIDs, []int64{scienceSkill}); err != nil {
		return err
	}
	if err := s.ensureAppBindings(ctx, mathApp, materialIDs, []int64{mathSkill}); err != nil {
		return err
	}
	if err := s.ensureAppBindings(ctx, feedbackApp, materialIDs, []int64{feedbackSkill}); err != nil {
		return err
	}

	classBTemplate, err := s.ensurePromptTemplate(
		ctx,
		classB,
		"班级知识助手模板",
		"你是{{class_name}}的{{app_name}}，只依据当前班级允许访问的资料回答{{user_name}}的问题。",
	)
	if err != nil {
		return err
	}
	classBApp, err := s.ensureAIApp(ctx, classB, "班级知识助手", "用于验证班级隔离的知识助手。", classBTemplate)
	if err != nil {
		return err
	}
	classBMaterials, err := s.materialIDsForClass(ctx, classB)
	if err != nil {
		return err
	}
	if err := s.ensureAppBindings(ctx, classBApp, classBMaterials, nil); err != nil {
		return err
	}

	if mcpBaseURL != "" && mcpToken != "" {
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO mcp_servers (class_id, name, base_url, auth_token, connected, last_error)
			VALUES (?, 'campus-tools', ?, ?, FALSE, '')
			ON DUPLICATE KEY UPDATE
				base_url = VALUES(base_url),
				auth_token = VALUES(auth_token)`,
			classA, mcpBaseURL, mcpToken,
		); err != nil {
			return fmt.Errorf("预置 MCP Server 失败: %w", err)
		}
	}
	return nil
}

func (s *Store) classIDByName(ctx context.Context, name string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM classes WHERE name = ?`, name).Scan(&id)
	return id, err
}

func (s *Store) ensurePromptTemplate(ctx context.Context, classID int64, name, content string) (int64, error) {
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO prompt_templates (class_id, name, content, built_in)
		VALUES (?, ?, ?, TRUE)
		ON DUPLICATE KEY UPDATE name = VALUES(name)`,
		classID, name, content,
	); err != nil {
		return 0, err
	}
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM prompt_templates WHERE class_id = ? AND name = ?`, classID, name).Scan(&id)
	return id, err
}

func (s *Store) ensureAIApp(ctx context.Context, classID int64, name, description string, templateID int64) (int64, error) {
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO ai_apps (class_id, name, description, prompt_template_id, enabled)
		VALUES (?, ?, ?, ?, TRUE)
		ON DUPLICATE KEY UPDATE name = VALUES(name)`,
		classID, name, description, templateID,
	); err != nil {
		return 0, err
	}
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM ai_apps WHERE class_id = ? AND name = ?`, classID, name).Scan(&id)
	return id, err
}

func (s *Store) ensureSkill(ctx context.Context, classID int64, skill Skill) (int64, error) {
	toolsJSON, err := json.Marshal(skill.Tools)
	if err != nil {
		return 0, err
	}
	source := renderSkillSource(skill)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO ai_skills
			(class_id, name, trigger_text, description, tools_json, source_text, subject_text, enabled, built_in)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, TRUE)
		ON DUPLICATE KEY UPDATE name = VALUES(name)`,
		classID,
		skill.Name,
		skill.TriggerText,
		skill.Description,
		string(toolsJSON),
		source,
		skill.SubjectText,
		skill.Enabled,
	); err != nil {
		return 0, err
	}
	var id int64
	err = s.db.QueryRowContext(ctx, `SELECT id FROM ai_skills WHERE class_id = ? AND name = ?`, classID, skill.Name).Scan(&id)
	return id, err
}

func renderSkillSource(skill Skill) string {
	enabled := "否"
	if skill.Enabled {
		enabled = "是"
	}
	tools := ""
	for index, item := range skill.Tools {
		if index > 0 {
			tools += ","
		}
		tools += item
	}
	return fmt.Sprintf(
		"名称：%s\n适用条件：%s\n资料：%s\n工具：%s\n启用：%s\n\n%s",
		skill.Name,
		skill.TriggerText,
		skill.SubjectText,
		tools,
		enabled,
		skill.Description,
	)
}

func (s *Store) materialIDsForClass(ctx context.Context, classID int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM materials WHERE class_id = ? ORDER BY id`, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) ensureAppBindings(ctx context.Context, appID int64, materialIDs, skillIDs []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, materialID := range materialIDs {
		if _, err := tx.ExecContext(ctx, `INSERT IGNORE INTO ai_app_materials (app_id, material_id) VALUES (?, ?)`, appID, materialID); err != nil {
			return err
		}
	}
	for _, skillID := range skillIDs {
		if _, err := tx.ExecContext(ctx, `INSERT IGNORE INTO ai_app_skills (app_id, skill_id) VALUES (?, ?)`, appID, skillID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
