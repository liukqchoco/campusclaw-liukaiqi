package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

type Store struct {
	db *sql.DB
}

type User struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	DisplayName  string `json:"display_name"`
	Role         string `json:"role"`
	ClassID      int64  `json:"class_id"`
	ClassName    string `json:"class_name"`
	PasswordHash string `json:"-"`
}

type Material struct {
	ID             int64     `json:"id"`
	ClassID        int64     `json:"class_id"`
	ClassName      string    `json:"class_name"`
	UploaderName   string    `json:"uploader_name"`
	Title          string    `json:"title"`
	OriginalName   string    `json:"original_name"`
	StoredName     string    `json:"-"`
	MediaType      string    `json:"media_type"`
	SizeBytes      int64     `json:"size_bytes"`
	Content        string    `json:"content,omitempty"`
	ContentPreview string    `json:"content_preview,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type NewMaterial struct {
	ClassID      int64
	UploaderID   int64
	Title        string
	OriginalName string
	StoredName   string
	MediaType    string
	SizeBytes    int64
	Content      string
}

type KnowledgeSource struct {
	ClassID          int64
	KnowledgeEntryID int64
	MaterialID       int64
	MaterialTitle    string
	Content          string
}

func Open(host, port, name, user, password string) (*Store, error) {
	cfg := mysqlDriver.NewConfig()
	cfg.User = user
	cfg.Passwd = password
	cfg.Net = "tcp"
	cfg.Addr = host + ":" + port
	cfg.DBName = name
	cfg.ParseTime = true
	cfg.Collation = "utf8mb4_unicode_ci"
	cfg.Params = map[string]string{"charset": "utf8mb4"}

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) WaitForDatabase(ctx context.Context, attempts int, interval time.Duration) error {
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := s.db.PingContext(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
	return fmt.Errorf("数据库在 %d 次重试后仍不可用: %w", attempts, lastErr)
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) Migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS classes (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			name VARCHAR(100) NOT NULL UNIQUE,
			created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS users (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			username VARCHAR(80) NOT NULL UNIQUE,
			display_name VARCHAR(100) NOT NULL,
			role VARCHAR(16) NOT NULL,
			class_id BIGINT NOT NULL,
			password_hash VARCHAR(255) NOT NULL,
			created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			CONSTRAINT fk_users_class FOREIGN KEY (class_id) REFERENCES classes(id),
			CONSTRAINT chk_users_role CHECK (role IN ('teacher', 'student'))
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
			user_id BIGINT NOT NULL,
			expires_at DATETIME(6) NOT NULL,
			created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			INDEX idx_sessions_user (user_id),
			INDEX idx_sessions_expiry (expires_at),
			CONSTRAINT fk_sessions_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS login_attempts (
			username VARCHAR(80) NOT NULL,
			ip_address VARCHAR(64) NOT NULL,
			failed_count INT NOT NULL DEFAULT 0,
			locked_until DATETIME(6) NULL,
			updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
			PRIMARY KEY (username, ip_address)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS lessons (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			class_id BIGINT NOT NULL,
			title VARCHAR(255) NOT NULL,
			created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			CONSTRAINT fk_lessons_class FOREIGN KEY (class_id) REFERENCES classes(id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS assignments (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			class_id BIGINT NOT NULL,
			title VARCHAR(255) NOT NULL,
			created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			CONSTRAINT fk_assignments_class FOREIGN KEY (class_id) REFERENCES classes(id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS assistants (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			class_id BIGINT NOT NULL,
			name VARCHAR(255) NOT NULL,
			created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			CONSTRAINT fk_assistants_class FOREIGN KEY (class_id) REFERENCES classes(id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS skills (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			class_id BIGINT NOT NULL,
			name VARCHAR(255) NOT NULL,
			created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			CONSTRAINT fk_skills_class FOREIGN KEY (class_id) REFERENCES classes(id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS materials (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			class_id BIGINT NOT NULL,
			uploader_id BIGINT NOT NULL,
			title VARCHAR(255) NOT NULL,
			original_name VARCHAR(255) NOT NULL,
			stored_name VARCHAR(255) NOT NULL,
			media_type VARCHAR(100) NOT NULL,
			size_bytes BIGINT NOT NULL,
			created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			INDEX idx_materials_class_created (class_id, created_at),
			CONSTRAINT fk_materials_class FOREIGN KEY (class_id) REFERENCES classes(id),
			CONSTRAINT fk_materials_uploader FOREIGN KEY (uploader_id) REFERENCES users(id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS knowledge_entries (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			material_id BIGINT NOT NULL UNIQUE,
			class_id BIGINT NOT NULL,
			content MEDIUMTEXT NOT NULL,
			created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
			INDEX idx_knowledge_class (class_id),
			CONSTRAINT fk_knowledge_material FOREIGN KEY (material_id) REFERENCES materials(id) ON DELETE CASCADE,
			CONSTRAINT fk_knowledge_class FOREIGN KEY (class_id) REFERENCES classes(id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	}
	statements = append(statements, aiMigrationStatements()...)

	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("执行数据库迁移失败: %w", err)
		}
	}
	return nil
}

func (s *Store) Seed(ctx context.Context, teacherPassword, studentPassword, uploadDir string) error {
	teacherHash, err := bcrypt.GenerateFromPassword([]byte(teacherPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	studentHash, err := bcrypt.GenerateFromPassword([]byte(studentPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	classA, err := s.ensureClass(ctx, "高一（1）班")
	if err != nil {
		return err
	}
	classB, err := s.ensureClass(ctx, "高一（2）班")
	if err != nil {
		return err
	}
	teacherA, err := s.ensureUser(ctx, "teacher_a", "张老师", "teacher", classA, string(teacherHash))
	if err != nil {
		return err
	}
	if _, err = s.ensureUser(ctx, "student_a1", "李同学", "student", classA, string(studentHash)); err != nil {
		return err
	}
	studentB, err := s.ensureUser(ctx, "student_b1", "王同学", "student", classB, string(studentHash))
	if err != nil {
		return err
	}

	if err = s.ensureSeedMaterial(ctx, classA, teacherA, "A 班开学说明", "a-class-welcome.md", "seed-a-class.md", "# A 班开学说明\n\n欢迎进入高一（1）班知识库。\n", uploadDir); err != nil {
		return err
	}
	scienceContent := `# 小学科学课程资料

## 磁铁

磁铁能吸引铁钉，不能吸引木块。磁铁只会吸引铁、钴、镍等铁磁性材料，不会吸引木头、塑料和玻璃。

## 水的三态

水在零摄氏度附近结成冰，加热到一百摄氏度附近沸腾并变成水蒸气。
`
	if err = s.ensureSeedMaterial(ctx, classA, teacherA, "A 班小学科学课程资料", "science-notes.md", "seed-a-science.md", scienceContent, uploadDir); err != nil {
		return err
	}
	mathContent := `# 数学单位换算资料

## 长度单位

1 米等于 100 厘米。厘米换算成米时，数值除以 100；米换算成厘米时，数值乘以 100。

1 千米等于 1000 米，1 米等于 1000 毫米。换算前应先确认原单位、目标单位和量纲。

## 质量单位

1 千克等于 1000 克。长度单位与质量单位不能跨量纲换算。
`
	if err = s.ensureSeedMaterial(ctx, classA, teacherA, "A 班数学单位换算资料", "math-unit-notes.md", "seed-a-math.md", mathContent, uploadDir); err != nil {
		return err
	}
	if err = s.ensureSeedMaterial(ctx, classB, studentB, "B 班学习安排", "b-class-plan.md", "seed-b-class.md", "# B 班学习安排\n\n本材料仅供高一（2）班查看。\n", uploadDir); err != nil {
		return err
	}
	return nil
}

func (s *Store) ensureClass(ctx context.Context, name string) (int64, error) {
	if _, err := s.db.ExecContext(ctx, `INSERT INTO classes (name) VALUES (?) ON DUPLICATE KEY UPDATE name = VALUES(name)`, name); err != nil {
		return 0, err
	}
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM classes WHERE name = ?`, name).Scan(&id)
	return id, err
}

func (s *Store) ensureUser(ctx context.Context, username, displayName, role string, classID int64, passwordHash string) (int64, error) {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (username, display_name, role, class_id, password_hash)
		VALUES (?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			display_name = VALUES(display_name),
			role = VALUES(role),
			class_id = VALUES(class_id),
			password_hash = VALUES(password_hash)`,
		username, displayName, role, classID, passwordHash,
	)
	if err != nil {
		return 0, err
	}

	var id int64
	err = s.db.QueryRowContext(ctx, `SELECT id FROM users WHERE username = ?`, username).Scan(&id)
	return id, err
}

func (s *Store) ensureSeedMaterial(ctx context.Context, classID, uploaderID int64, title, originalName, storedName, content, uploadDir string) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM materials WHERE class_id = ? AND title = ?`, classID, title).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	if err := os.MkdirAll(uploadDir, 0o750); err != nil {
		return err
	}
	path := filepath.Join(uploadDir, storedName)
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		return err
	}

	_, err := s.CreateMaterial(ctx, NewMaterial{
		ClassID:      classID,
		UploaderID:   uploaderID,
		Title:        title,
		OriginalName: originalName,
		StoredName:   storedName,
		MediaType:    "text/markdown; charset=utf-8",
		SizeBytes:    int64(len([]byte(content))),
		Content:      content,
	})
	if err != nil {
		_ = os.Remove(path)
	}
	return err
}

func (s *Store) FindUserByUsername(ctx context.Context, username string) (User, error) {
	var user User
	err := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.username, u.display_name, u.role, u.class_id, c.name, u.password_hash
		FROM users u
		JOIN classes c ON c.id = u.class_id
		WHERE u.username = ?`,
		username,
	).Scan(&user.ID, &user.Username, &user.DisplayName, &user.Role, &user.ClassID, &user.ClassName, &user.PasswordHash)
	return user, err
}

func (s *Store) FindUserByID(ctx context.Context, id int64) (User, error) {
	var user User
	err := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.username, u.display_name, u.role, u.class_id, c.name
		FROM users u
		JOIN classes c ON c.id = u.class_id
		WHERE u.id = ?`,
		id,
	).Scan(&user.ID, &user.Username, &user.DisplayName, &user.Role, &user.ClassID, &user.ClassName)
	return user, err
}

func (s *Store) CreateSession(ctx context.Context, idHash string, userID int64, expiresAt time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err = tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? OR expires_at <= NOW(6)`, userID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO sessions (id_hash, user_id, expires_at) VALUES (?, ?, ?)`, idHash, userID, expiresAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteSession(ctx context.Context, idHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id_hash = ?`, idHash)
	return err
}

func (s *Store) UserForSession(ctx context.Context, idHash string) (User, error) {
	var user User
	err := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.username, u.display_name, u.role, u.class_id, c.name
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		JOIN classes c ON c.id = u.class_id
		WHERE s.id_hash = ? AND s.expires_at > NOW(6)`,
		idHash,
	).Scan(&user.ID, &user.Username, &user.DisplayName, &user.Role, &user.ClassID, &user.ClassName)
	return user, err
}

func (s *Store) LoginAllowed(ctx context.Context, username, ip string, now time.Time) (bool, error) {
	var lockedUntil sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT locked_until FROM login_attempts WHERE username = ? AND ip_address = ?`, username, ip).Scan(&lockedUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return !lockedUntil.Valid || !lockedUntil.Time.After(now), nil
}

func (s *Store) RecordLoginFailure(ctx context.Context, username, ip string, maxFailures int, lockDuration time.Duration, now time.Time) error {
	var previous int
	var previousLock sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT failed_count, locked_until
		FROM login_attempts
		WHERE username = ? AND ip_address = ?`,
		username, ip,
	).Scan(&previous, &previousLock)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	if previousLock.Valid && previousLock.Time.Before(now) {
		previous = 0
	}
	next := previous + 1
	var lockedUntil any
	if next >= maxFailures {
		lockedUntil = now.Add(lockDuration)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO login_attempts (username, ip_address, failed_count, locked_until)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			failed_count = VALUES(failed_count),
			locked_until = VALUES(locked_until)`,
		username, ip, next, lockedUntil,
	)
	return err
}

func (s *Store) ClearLoginFailures(ctx context.Context, username, ip string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM login_attempts WHERE username = ? AND ip_address = ?`, username, ip)
	return err
}

func (s *Store) ListMaterials(ctx context.Context, classID int64, search string) ([]Material, error) {
	query := `
		SELECT m.id, m.class_id, c.name, u.display_name, m.title, m.original_name,
		       m.media_type, m.size_bytes, LEFT(k.content, 180), m.created_at
		FROM materials m
		JOIN classes c ON c.id = m.class_id
		JOIN users u ON u.id = m.uploader_id
		JOIN knowledge_entries k ON k.material_id = m.id
		WHERE m.class_id = ?`
	args := []any{classID}

	if trimmed := strings.TrimSpace(search); trimmed != "" {
		query += ` AND (m.title LIKE ? OR k.content LIKE ?)`
		pattern := "%" + trimmed + "%"
		args = append(args, pattern, pattern)
	}
	query += ` ORDER BY m.created_at DESC, m.id DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	materials := make([]Material, 0)
	for rows.Next() {
		var material Material
		if err := rows.Scan(
			&material.ID,
			&material.ClassID,
			&material.ClassName,
			&material.UploaderName,
			&material.Title,
			&material.OriginalName,
			&material.MediaType,
			&material.SizeBytes,
			&material.ContentPreview,
			&material.CreatedAt,
		); err != nil {
			return nil, err
		}
		materials = append(materials, material)
	}
	return materials, rows.Err()
}

func (s *Store) ListKnowledgeSources(ctx context.Context, classID int64) ([]KnowledgeSource, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT k.class_id, k.id, m.id, m.title, k.content
		FROM materials m
		JOIN knowledge_entries k ON k.material_id = m.id
		WHERE m.class_id = ? AND k.class_id = ?
		ORDER BY m.id`,
		classID, classID,
	)
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

func (s *Store) ListAllKnowledgeSources(ctx context.Context) ([]KnowledgeSource, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT k.class_id, k.id, m.id, m.title, k.content
		FROM knowledge_entries k
		JOIN materials m ON m.id = k.material_id AND m.class_id = k.class_id
		ORDER BY k.class_id, m.id`)
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

func (s *Store) GetKnowledgeSourceForMaterial(
	ctx context.Context,
	materialID, classID int64,
) (KnowledgeSource, error) {
	var source KnowledgeSource
	err := s.db.QueryRowContext(ctx, `
		SELECT k.class_id, k.id, m.id, m.title, k.content
		FROM knowledge_entries k
		JOIN materials m ON m.id = k.material_id AND m.class_id = k.class_id
		WHERE m.id = ? AND m.class_id = ?`,
		materialID, classID,
	).Scan(
		&source.ClassID,
		&source.KnowledgeEntryID,
		&source.MaterialID,
		&source.MaterialTitle,
		&source.Content,
	)
	return source, err
}

func (s *Store) GetMaterial(ctx context.Context, id, classID int64) (Material, error) {
	var material Material
	err := s.db.QueryRowContext(ctx, `
		SELECT m.id, m.class_id, c.name, u.display_name, m.title, m.original_name,
		       m.stored_name, m.media_type, m.size_bytes, k.content, m.created_at
		FROM materials m
		JOIN classes c ON c.id = m.class_id
		JOIN users u ON u.id = m.uploader_id
		JOIN knowledge_entries k ON k.material_id = m.id
		WHERE m.id = ? AND m.class_id = ?`,
		id, classID,
	).Scan(
		&material.ID,
		&material.ClassID,
		&material.ClassName,
		&material.UploaderName,
		&material.Title,
		&material.OriginalName,
		&material.StoredName,
		&material.MediaType,
		&material.SizeBytes,
		&material.Content,
		&material.CreatedAt,
	)
	return material, err
}

func (s *Store) CreateMaterial(ctx context.Context, material NewMaterial) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		INSERT INTO materials
			(class_id, uploader_id, title, original_name, stored_name, media_type, size_bytes)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		material.ClassID,
		material.UploaderID,
		material.Title,
		material.OriginalName,
		material.StoredName,
		material.MediaType,
		material.SizeBytes,
	)
	if err != nil {
		return 0, err
	}
	materialID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO knowledge_entries (material_id, class_id, content)
		VALUES (?, ?, ?)`,
		materialID, material.ClassID, material.Content,
	); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return materialID, nil
}
