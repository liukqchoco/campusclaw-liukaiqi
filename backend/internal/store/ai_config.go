package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

type AIAppInput struct {
	Name             string
	Description      string
	PromptTemplateID int64
	Enabled          bool
	MaterialIDs      []int64
	SkillIDs         []int64
}

func (s *Store) ListPromptTemplates(ctx context.Context, classID int64) ([]PromptTemplate, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, class_id, name, content, built_in, updated_at
		FROM prompt_templates
		WHERE class_id = ?
		ORDER BY id`,
		classID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	templates := make([]PromptTemplate, 0)
	for rows.Next() {
		var template PromptTemplate
		if err := rows.Scan(
			&template.ID,
			&template.ClassID,
			&template.Name,
			&template.Content,
			&template.BuiltIn,
			&template.UpdatedAt,
		); err != nil {
			return nil, err
		}
		templates = append(templates, template)
	}
	return templates, rows.Err()
}

func (s *Store) UpdatePromptTemplate(ctx context.Context, id, classID int64, name, content string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE prompt_templates
		SET name = ?, content = ?
		WHERE id = ? AND class_id = ?`,
		name, content, id, classID,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ListAIApps(ctx context.Context, classID int64, includeDisabled bool) ([]AIApp, error) {
	query := `
		SELECT a.id, a.class_id, a.name, a.description, a.prompt_template_id,
		       p.name, p.content, a.enabled, a.created_at, a.updated_at
		FROM ai_apps a
		JOIN prompt_templates p ON p.id = a.prompt_template_id AND p.class_id = a.class_id
		WHERE a.class_id = ?`
	if !includeDisabled {
		query += ` AND a.enabled = TRUE`
	}
	query += ` ORDER BY a.id`
	rows, err := s.db.QueryContext(ctx, query, classID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	apps := make([]AIApp, 0)
	for rows.Next() {
		var app AIApp
		if err := rows.Scan(
			&app.ID,
			&app.ClassID,
			&app.Name,
			&app.Description,
			&app.PromptTemplateID,
			&app.PromptTemplateName,
			&app.PromptTemplate,
			&app.Enabled,
			&app.CreatedAt,
			&app.UpdatedAt,
		); err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for index := range apps {
		if err := s.loadAppBindings(ctx, &apps[index]); err != nil {
			return nil, err
		}
	}
	return apps, nil
}

func (s *Store) GetAIApp(ctx context.Context, id, classID int64, enabledOnly bool) (AIApp, error) {
	query := `
		SELECT a.id, a.class_id, a.name, a.description, a.prompt_template_id,
		       p.name, p.content, a.enabled, a.created_at, a.updated_at
		FROM ai_apps a
		JOIN prompt_templates p ON p.id = a.prompt_template_id AND p.class_id = a.class_id
		WHERE a.id = ? AND a.class_id = ?`
	if enabledOnly {
		query += ` AND a.enabled = TRUE`
	}
	var app AIApp
	err := s.db.QueryRowContext(ctx, query, id, classID).Scan(
		&app.ID,
		&app.ClassID,
		&app.Name,
		&app.Description,
		&app.PromptTemplateID,
		&app.PromptTemplateName,
		&app.PromptTemplate,
		&app.Enabled,
		&app.CreatedAt,
		&app.UpdatedAt,
	)
	if err != nil {
		return AIApp{}, err
	}
	if err := s.loadAppBindings(ctx, &app); err != nil {
		return AIApp{}, err
	}
	return app, nil
}

func (s *Store) CreateAIApp(ctx context.Context, classID int64, input AIAppInput) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if err := validateTemplateTx(ctx, tx, input.PromptTemplateID, classID); err != nil {
		return 0, err
	}
	if err := validateBindingsTx(ctx, tx, classID, input.MaterialIDs, input.SkillIDs); err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO ai_apps (class_id, name, description, prompt_template_id, enabled)
		VALUES (?, ?, ?, ?, ?)`,
		classID, input.Name, input.Description, input.PromptTemplateID, input.Enabled,
	)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := replaceBindingsTx(ctx, tx, id, input.MaterialIDs, input.SkillIDs); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) UpdateAIApp(ctx context.Context, id, classID int64, input AIAppInput) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := validateTemplateTx(ctx, tx, input.PromptTemplateID, classID); err != nil {
		return err
	}
	if err := validateBindingsTx(ctx, tx, classID, input.MaterialIDs, input.SkillIDs); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE ai_apps
		SET name = ?, description = ?, prompt_template_id = ?, enabled = ?
		WHERE id = ? AND class_id = ?`,
		input.Name, input.Description, input.PromptTemplateID, input.Enabled, id, classID,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM ai_apps WHERE id = ? AND class_id = ?`, id, classID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return sql.ErrNoRows
		}
	}
	if err := replaceBindingsTx(ctx, tx, id, input.MaterialIDs, input.SkillIDs); err != nil {
		return err
	}
	return tx.Commit()
}

func validateTemplateTx(ctx context.Context, tx *sql.Tx, templateID, classID int64) error {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM prompt_templates WHERE id = ? AND class_id = ?`, templateID, classID).Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func validateBindingsTx(ctx context.Context, tx *sql.Tx, classID int64, materialIDs, skillIDs []int64) error {
	for _, id := range uniquePositiveIDs(materialIDs) {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM materials WHERE id = ? AND class_id = ?`, id, classID).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return sql.ErrNoRows
		}
	}
	for _, id := range uniquePositiveIDs(skillIDs) {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM ai_skills WHERE id = ? AND class_id = ?`, id, classID).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return sql.ErrNoRows
		}
	}
	return nil
}

func replaceBindingsTx(ctx context.Context, tx *sql.Tx, appID int64, materialIDs, skillIDs []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM ai_app_materials WHERE app_id = ?`, appID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM ai_app_skills WHERE app_id = ?`, appID); err != nil {
		return err
	}
	for _, id := range uniquePositiveIDs(materialIDs) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO ai_app_materials (app_id, material_id) VALUES (?, ?)`, appID, id); err != nil {
			return err
		}
	}
	for _, id := range uniquePositiveIDs(skillIDs) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO ai_app_skills (app_id, skill_id) VALUES (?, ?)`, appID, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) loadAppBindings(ctx context.Context, app *AIApp) error {
	materialIDs, err := queryIDs(ctx, s.db, `SELECT material_id FROM ai_app_materials WHERE app_id = ? ORDER BY material_id`, app.ID)
	if err != nil {
		return err
	}
	skillIDs, err := queryIDs(ctx, s.db, `SELECT skill_id FROM ai_app_skills WHERE app_id = ? ORDER BY skill_id`, app.ID)
	if err != nil {
		return err
	}
	app.MaterialIDs = materialIDs
	app.SkillIDs = skillIDs
	return nil
}

type idQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func queryIDs(ctx context.Context, queryer idQuerier, query string, args ...any) ([]int64, error) {
	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func uniquePositiveIDs(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func (s *Store) ListSkills(ctx context.Context, classID int64) ([]Skill, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, class_id, name, trigger_text, description, tools_json, source_text,
		       subject_text, enabled, built_in, created_at, updated_at
		FROM ai_skills
		WHERE class_id = ?
		ORDER BY id`,
		classID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	skills := make([]Skill, 0)
	for rows.Next() {
		skill, err := scanSkill(rows)
		if err != nil {
			return nil, err
		}
		skills = append(skills, skill)
	}
	return skills, rows.Err()
}

func (s *Store) ListAppSkills(ctx context.Context, appID, classID int64) ([]Skill, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, s.class_id, s.name, s.trigger_text, s.description, s.tools_json,
		       s.source_text, s.subject_text, s.enabled, s.built_in, s.created_at, s.updated_at
		FROM ai_app_skills b
		JOIN ai_apps a ON a.id = b.app_id
		JOIN ai_skills s ON s.id = b.skill_id AND s.class_id = a.class_id
		WHERE b.app_id = ? AND a.class_id = ? AND s.enabled = TRUE
		ORDER BY s.id`,
		appID, classID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	skills := make([]Skill, 0)
	for rows.Next() {
		skill, err := scanSkill(rows)
		if err != nil {
			return nil, err
		}
		skills = append(skills, skill)
	}
	return skills, rows.Err()
}

type rowScanner interface {
	Scan(...any) error
}

func scanSkill(scanner rowScanner) (Skill, error) {
	var skill Skill
	var toolsJSON string
	err := scanner.Scan(
		&skill.ID,
		&skill.ClassID,
		&skill.Name,
		&skill.TriggerText,
		&skill.Description,
		&toolsJSON,
		&skill.SourceText,
		&skill.SubjectText,
		&skill.Enabled,
		&skill.BuiltIn,
		&skill.CreatedAt,
		&skill.UpdatedAt,
	)
	if err != nil {
		return Skill{}, err
	}
	if err := json.Unmarshal([]byte(toolsJSON), &skill.Tools); err != nil {
		return Skill{}, fmt.Errorf("解析技能工具失败: %w", err)
	}
	return skill, nil
}

func (s *Store) CreateSkill(ctx context.Context, classID int64, skill Skill) (int64, error) {
	toolsJSON, err := json.Marshal(skill.Tools)
	if err != nil {
		return 0, err
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO ai_skills
			(class_id, name, trigger_text, description, tools_json, source_text, subject_text, enabled, built_in)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, FALSE)`,
		classID,
		skill.Name,
		skill.TriggerText,
		skill.Description,
		string(toolsJSON),
		skill.SourceText,
		skill.SubjectText,
		skill.Enabled,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) SetSkillEnabled(ctx context.Context, id, classID int64, enabled bool) error {
	result, err := s.db.ExecContext(ctx, `UPDATE ai_skills SET enabled = ? WHERE id = ? AND class_id = ?`, enabled, id, classID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		var count int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ai_skills WHERE id = ? AND class_id = ?`, id, classID).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			return sql.ErrNoRows
		}
	}
	return nil
}

func (s *Store) ListKnowledgeSourcesForApp(ctx context.Context, appID, classID int64, subject string) ([]KnowledgeSource, error) {
	query := `
		SELECT k.class_id, k.id, m.id, m.title, k.content
		FROM ai_app_materials b
		JOIN ai_apps a ON a.id = b.app_id
		JOIN materials m ON m.id = b.material_id AND m.class_id = a.class_id
		JOIN knowledge_entries k ON k.material_id = m.id AND k.class_id = a.class_id
		WHERE b.app_id = ? AND a.class_id = ?`
	args := []any{appID, classID}
	if trimmed := strings.TrimSpace(subject); trimmed != "" {
		query += ` AND m.title LIKE ?`
		args = append(args, "%"+trimmed+"%")
	}
	query += ` ORDER BY m.id`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sources := make([]KnowledgeSource, 0)
	for rows.Next() {
		var source KnowledgeSource
		if err := rows.Scan(&source.ClassID, &source.KnowledgeEntryID, &source.MaterialID, &source.MaterialTitle, &source.Content); err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, rows.Err()
}
