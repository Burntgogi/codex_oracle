package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Metadata struct {
	ID              string         `json:"id"`
	CreatedAt       string         `json:"createdAt"`
	LastHeartbeatAt string         `json:"lastHeartbeatAt,omitempty"`
	Status          string         `json:"status"`
	Model           string         `json:"model,omitempty"`
	Mode            string         `json:"mode,omitempty"`
	CWD             string         `json:"cwd,omitempty"`
	Runtime         map[string]any `json:"runtime,omitempty"`
}

type Detail struct {
	Metadata Metadata       `json:"metadata"`
	Request  map[string]any `json:"request,omitempty"`
	Log      string         `json:"log"`
}

type ListOptions struct {
	Hours      float64
	Limit      int
	IncludeAll bool
}

type Store struct {
	home string
	// ponytail: per-store lock; use interprocess locking if multiple writers share a home.
	mu sync.Mutex
}

type CreateOptions struct {
	Prompt     string
	Model      string
	Mode       string
	CWD        string
	Status     string
	Slug       string
	Request    map[string]any
	Output     string
	Transcript string
	Runtime    map[string]any
}

type UpdateOptions struct {
	Status     string
	Output     string
	Transcript string
	Runtime    map[string]any
}

func NewStore(home string) *Store {
	if home == "" {
		home = DefaultHome()
	}
	return &Store{home: home}
}

func DefaultHome() string {
	if configured := strings.TrimSpace(os.Getenv("ORACLE_HOME_DIR")); configured != "" {
		return configured
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".codex_oracle"
	}
	return filepath.Join(home, ".codex_oracle")
}

func (s *Store) SessionsDir() string {
	return filepath.Join(s.home, "sessions")
}

func (s *Store) HomeDir() string {
	return s.home
}

func (s *Store) Create(options CreateOptions) (Metadata, error) {
	id := sessionID(options.Slug)
	sessionDir := filepath.Join(s.SessionsDir(), id)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return Metadata{}, err
	}
	status := options.Status
	if status == "" {
		status = "completed"
	}
	meta := Metadata{
		ID:        id,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Status:    status,
		Model:     options.Model,
		Mode:      options.Mode,
		CWD:       options.CWD,
		Runtime:   options.Runtime,
	}
	if err := writeJSON(filepath.Join(sessionDir, "meta.json"), meta); err != nil {
		return Metadata{}, err
	}
	request := sanitizeRequest(options.Request)
	if request == nil {
		request = map[string]any{}
	}
	if persistSessionContent() && options.Prompt != "" {
		request["prompt"] = options.Prompt
	}
	if err := writeJSON(filepath.Join(sessionDir, "request.json"), request); err != nil {
		return Metadata{}, err
	}
	output := ""
	if persistSessionContent() {
		output = options.Output
	}
	if err := writeFile(filepath.Join(sessionDir, "output.log"), []byte(output)); err != nil {
		return Metadata{}, err
	}
	if persistSessionContent() && options.Transcript != "" {
		artifactsDir := filepath.Join(sessionDir, "artifacts")
		if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
			return Metadata{}, err
		}
		if err := writeFile(filepath.Join(artifactsDir, "transcript.md"), []byte(options.Transcript)); err != nil {
			return Metadata{}, err
		}
	}
	return meta, nil
}

func (s *Store) UpdateResult(id string, options UpdateOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cleanID := filepath.Base(id)
	meta, err := s.Read(cleanID)
	if err != nil {
		return err
	}
	if options.Status != "" {
		meta.Status = options.Status
	}
	if len(options.Runtime) > 0 {
		if meta.Runtime == nil {
			meta.Runtime = map[string]any{}
		}
		for key, value := range options.Runtime {
			meta.Runtime[key] = value
		}
	}
	sessionDir := filepath.Join(s.SessionsDir(), cleanID)
	output := ""
	if persistSessionContent() {
		output = options.Output
	}
	if err := writeFile(filepath.Join(sessionDir, "output.log"), []byte(output)); err != nil {
		return err
	}
	if persistSessionContent() && options.Transcript != "" {
		artifactsDir := filepath.Join(sessionDir, "artifacts")
		if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
			return err
		}
		if err := writeFile(filepath.Join(artifactsDir, "transcript.md"), []byte(options.Transcript)); err != nil {
			return err
		}
	}
	// Commit completion only after result files are written, so failures can be retried.
	return writeJSON(filepath.Join(sessionDir, "meta.json"), meta)
}

func (s *Store) Delete(id string) error {
	cleanID := filepath.Base(strings.TrimSpace(id))
	if cleanID == "" || cleanID == "." || cleanID == ".." {
		return errors.New("invalid session id")
	}
	sessionDir := filepath.Join(s.SessionsDir(), cleanID)
	relative, err := filepath.Rel(s.SessionsDir(), sessionDir)
	if err != nil {
		return err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return errors.New("invalid session id")
	}
	return os.RemoveAll(sessionDir)
}

func ShouldPersistContent() bool {
	return persistSessionContent()
}

func (s *Store) WriteArtifact(id string, relativePath string, content []byte) error {
	cleanID := filepath.Base(id)
	cleanPath := filepath.Clean(relativePath)
	if cleanPath == "." || filepath.IsAbs(cleanPath) || cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(os.PathSeparator)) {
		return errors.New("invalid artifact path")
	}
	sessionDir := filepath.Join(s.SessionsDir(), cleanID)
	fullPath := filepath.Join(sessionDir, cleanPath)
	relative, err := filepath.Rel(sessionDir, fullPath)
	if err != nil {
		return err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return errors.New("invalid artifact path")
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return err
	}
	return writeFile(fullPath, content)
}

func (s *Store) UpdateHeartbeat(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cleanID := filepath.Base(id)
	meta, err := s.Read(cleanID)
	if err != nil {
		return err
	}
	meta.LastHeartbeatAt = time.Now().UTC().Format(time.RFC3339)
	return writeJSON(filepath.Join(s.SessionsDir(), cleanID, "meta.json"), meta)
}

// Record the browser location without replacing output or transcript artifacts.
func (s *Store) UpdateConversation(id string, conversationURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, err := s.Read(id)
	if err != nil {
		return err
	}
	if meta.Status != "running" {
		return errors.New("only a running session can be checkpointed")
	}
	if previous, _ := meta.Runtime["conversationUrl"].(string); previous != "" && previous != conversationURL {
		return errors.New("session already belongs to a different conversation")
	}
	if meta.Runtime == nil {
		meta.Runtime = map[string]any{}
	}
	meta.Runtime["conversationUrl"] = conversationURL
	meta.LastHeartbeatAt = time.Now().UTC().Format(time.RFC3339)
	return writeJSON(filepath.Join(s.SessionsDir(), filepath.Base(id), "meta.json"), meta)
}

func (s *Store) List(options ListOptions) ([]Metadata, error) {
	entries, err := os.ReadDir(s.SessionsDir())
	if errors.Is(err, os.ErrNotExist) {
		return []Metadata{}, nil
	}
	if err != nil {
		return nil, err
	}
	metas := []Metadata{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		meta, err := s.Read(entry.Name())
		if err == nil && meta != nil {
			metas = append(metas, *meta)
		}
	}
	sort.Slice(metas, func(i, j int) bool {
		return createdAt(metas[i]).After(createdAt(metas[j]))
	})
	if !options.IncludeAll {
		hours := options.Hours
		if hours <= 0 {
			hours = 24
		}
		cutoff := time.Now().Add(-time.Duration(hours * float64(time.Hour)))
		filtered := metas[:0]
		for _, meta := range metas {
			if createdAt(meta).After(cutoff) {
				filtered = append(filtered, meta)
			}
		}
		metas = filtered
	}
	if options.Limit > 0 && len(metas) > options.Limit {
		metas = metas[:options.Limit]
	}
	return metas, nil
}

func (s *Store) Read(id string) (*Metadata, error) {
	cleanID := filepath.Base(id)
	data, err := os.ReadFile(filepath.Join(s.SessionsDir(), cleanID, "meta.json"))
	if err != nil {
		return nil, err
	}
	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	if meta.ID == "" {
		meta.ID = cleanID
	}
	return &meta, nil
}

func (s *Store) ReadDetail(id string) (*Detail, error) {
	meta, err := s.Read(id)
	if err != nil {
		return nil, err
	}
	sessionDir := filepath.Join(s.SessionsDir(), filepath.Base(id))
	logBytes, err := os.ReadFile(filepath.Join(sessionDir, "output.log"))
	if errors.Is(err, os.ErrNotExist) {
		logBytes = nil
	} else if err != nil {
		return nil, err
	}
	request := map[string]any(nil)
	requestBytes, err := os.ReadFile(filepath.Join(sessionDir, "request.json"))
	if err == nil {
		request = map[string]any{}
		if err := json.Unmarshal(requestBytes, &request); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return &Detail{Metadata: *meta, Request: request, Log: string(logBytes)}, nil
}

func createdAt(meta Metadata) time.Time {
	parsed, err := time.Parse(time.RFC3339, meta.CreatedAt)
	if err == nil {
		return parsed
	}
	return time.Time{}
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFile(path, data)
}

func writeFile(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".oracle-write-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func persistSessionContent() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("ORACLE_SESSION_CONTENT_MODE")), "full")
}

func sanitizeRequest(request map[string]any) map[string]any {
	if request == nil {
		return nil
	}
	if persistSessionContent() {
		return request
	}
	sanitized := map[string]any{}
	for key, value := range request {
		if isContentKey(key) {
			continue
		}
		sanitized[key] = value
	}
	return sanitized
}

func isContentKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	switch normalized {
	case "prompt", "bundletext", "handoff", "answer", "transcript", "output", "browserfollowups", "followups":
		return true
	default:
		return false
	}
}

func sessionID(slug string) string {
	clean := sanitizeSlug(slug)
	if clean != "" {
		return clean + "-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	}
	return "session-" + time.Now().UTC().Format("20060102T150405.000000000Z")
}

func sanitizeSlug(slug string) string {
	slug = strings.ToLower(strings.TrimSpace(slug))
	var builder strings.Builder
	lastDash := false
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && builder.Len() > 0 {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}
