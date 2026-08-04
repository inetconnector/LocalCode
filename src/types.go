// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"sync"
	"time"
)

type MCPServerConfig struct {
	Name       string            `json:"name"`
	Enabled    bool              `json:"enabled"`
	Transport  string            `json:"transport"` // stdio | http
	Command    string            `json:"command,omitempty"`
	Args       []string          `json:"args,omitempty"`
	URL        string            `json:"url,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	TimeoutSec int               `json:"timeout_sec,omitempty"`
}

type Config struct {
	SchemaVersion  int    `json:"schema_version"`
	RootProjectDir string `json:"root_project_dir"`
	LastProject    string `json:"last_project"`
	LastModel      string `json:"last_model"`
	Port           int    `json:"port"`

	OllamaURL                         string `json:"ollama_url"`
	ContextLength                     int    `json:"context_length"`
	ContextCompactionEnabled          bool   `json:"context_compaction_enabled"`
	ContextCompactionThresholdPercent int    `json:"context_compaction_threshold_percent"`
	ContextCompactionKeepRecent       int    `json:"context_compaction_keep_recent"`
	MaxAgentSteps                     int    `json:"max_agent_steps"`
	CommandTimeout                    int    `json:"command_timeout_seconds"`
	ModelTimeout                      int    `json:"model_timeout_seconds"`
	ApprovalMode                      string `json:"approval_mode"` // strict | balanced | auto | dangerous
	SandboxMode                       string `json:"sandbox_mode"`  // project | workspace | unrestricted
	NetworkEnabled                    bool   `json:"network_enabled"`
	WebSearchProvider                 string `json:"web_search_provider"` // disabled | duckduckgo | ollama
	WebSearchAPIKeyEnv                string `json:"web_search_api_key_env"`
	WebSearchMaxResults               int    `json:"web_search_max_results"`
	WebFetchMaxBytes                  int64  `json:"web_fetch_max_bytes"`

	GitEnabled        bool   `json:"git_enabled"`
	AutoStateUpdate   bool   `json:"auto_state_update"`
	StateFile         string `json:"state_file"`
	CreateProjectDocs bool   `json:"create_project_docs"`

	AllowedRoots           []string          `json:"allowed_roots"`
	BlockedCommandPatterns []string          `json:"blocked_command_patterns"`
	MCPServers             []MCPServerConfig `json:"mcp_servers"`

	// Desktop UI and workflow preferences. These values are persisted and
	// immediately applied by the embedded client; none of them are placeholders.
	UITheme              string            `json:"ui_theme"` // dark | light | system
	UIAccent             string            `json:"ui_accent"`
	UIBackground         string            `json:"ui_background"`
	UIForeground         string            `json:"ui_foreground"`
	UIFont               string            `json:"ui_font"`
	CodeFont             string            `json:"code_font"`
	UILeftWidth          int               `json:"ui_left_width"`
	UIRightWidth         int               `json:"ui_right_width"`
	UITerminalHeight     int               `json:"ui_terminal_height"`
	ShowBottomBar        bool              `json:"show_bottom_bar"`
	TerminalDock         string            `json:"terminal_dock"`       // bottom | right
	TerminalShell        string            `json:"terminal_shell"`      // powershell | cmd | wsl
	AgentEnvironment     string            `json:"agent_environment"`   // windows-native | wsl
	DefaultOpenTarget    string            `json:"default_open_target"` // explorer | vscode | visualstudio
	Language             string            `json:"language"`
	ResponseSpeed        string            `json:"response_speed"` // fast | balanced | thorough
	ProfileName          string            `json:"profile_name"`
	AvatarInitials       string            `json:"avatar_initials"`
	UserInstructions     string            `json:"user_instructions"`
	PreferredLanguage    string            `json:"preferred_language"`
	VoiceEnabled         bool              `json:"voice_enabled"`
	PetEnabled           bool              `json:"pet_enabled"`
	PetName              string            `json:"pet_name"`
	Shortcuts            map[string]string `json:"shortcuts"`
	HookBeforeTask       string            `json:"hook_before_task"`
	HookAfterTask        string            `json:"hook_after_task"`
	HookBeforeTool       string            `json:"hook_before_tool"`
	HookAfterTool        string            `json:"hook_after_tool"`
	EnvironmentVars      map[string]string `json:"environment_vars"`
	AutoDiscoverTools    bool              `json:"auto_discover_tools"`
	AutoResearchToolHelp bool              `json:"auto_research_tool_help"`
	ToolOverrides        map[string]string `json:"tool_overrides"`
}

type Attachment struct {
	Name string `json:"name"`
	MIME string `json:"mime"`
	Data string `json:"data"`
	Size int64  `json:"size,omitempty"`
}

type AttachmentSummary struct {
	Name string `json:"name"`
	MIME string `json:"mime,omitempty"`
	Size int64  `json:"size,omitempty"`
}

type ModelInfo struct {
	Name       string `json:"name"`
	Size       int64  `json:"size,omitempty"`
	ModifiedAt string `json:"modified_at,omitempty"`
}

type Status struct {
	Version        string      `json:"version"`
	OllamaOnline   bool        `json:"ollama_online"`
	OllamaURL      string      `json:"ollama_url,omitempty"`
	OllamaError    string      `json:"ollama_error,omitempty"`
	Models         []ModelInfo `json:"models"`
	SelectedModel  string      `json:"selected_model"`
	GPU            string      `json:"gpu,omitempty"`
	RootDir        string      `json:"root_dir"`
	Project        string      `json:"project,omitempty"`
	Running        bool        `json:"running"`
	GitAvailable   bool        `json:"git_available"`
	MCPCount       int         `json:"mcp_count"`
	RunID          string      `json:"run_id,omitempty"`
	RunPhase       string      `json:"run_phase,omitempty"`
	RunStartedAt   time.Time   `json:"run_started_at,omitempty"`
	LastProgressAt time.Time   `json:"last_progress_at,omitempty"`
}

type UIEvent struct {
	ID          string              `json:"id"`
	Type        string              `json:"type"`
	Message     string              `json:"message,omitempty"`
	Detail      string              `json:"detail,omitempty"`
	Action      string              `json:"action,omitempty"`
	Path        string              `json:"path,omitempty"`
	Command     string              `json:"command,omitempty"`
	Preview     string              `json:"preview,omitempty"`
	Attachments []AttachmentSummary `json:"attachments,omitempty"`
	Timestamp   time.Time           `json:"timestamp"`
}

type PendingAction struct {
	ID      string
	Action  AgentAction
	Preview string
	Result  chan bool
}

type AgentContinuation struct {
	Project         string
	ThreadID        string
	Model           string
	Question        string
	Messages        []OllamaMessage
	SuggestedAction *AgentAction
	OriginalTask    string
	CompactionCount int
}

type AppState struct {
	mu sync.RWMutex

	Config Config
	Ollama *OllamaClient

	Project        string
	Model          string
	Running        bool
	Cancel         context.CancelFunc
	RunID          string
	RunPhase       string
	RunStartedAt   time.Time
	LastProgressAt time.Time

	Events        []UIEvent
	Pending       *PendingAction
	Continuation  *AgentContinuation
	Threads       map[string]*ChatThread
	CurrentThread string

	LastTask    string
	LastSummary string
	ActionLog   []string

	subscribers map[chan UIEvent]struct{}
}

func NewAppState(cfg Config, ollama *OllamaClient) *AppState {
	threads := loadThreads()
	state := &AppState{
		Config:      cfg,
		Ollama:      ollama,
		Project:     cfg.LastProject,
		Model:       cfg.LastModel,
		Threads:     threads,
		subscribers: make(map[chan UIEvent]struct{}),
	}
	if cfg.LastProject != "" {
		var latest *ChatThread
		for _, t := range threads {
			if t.Project == cfg.LastProject && !t.Archived && (latest == nil || t.UpdatedAt.After(latest.UpdatedAt)) {
				latest = t
			}
		}
		if latest != nil {
			state.CurrentThread = latest.ID
			state.Events = append([]UIEvent(nil), latest.Events...)
			if latest.Model != "" {
				state.Model = latest.Model
			}
		}
	}
	return state
}

func (s *AppState) AddEvent(ev UIEvent) {
	s.mu.Lock()
	if s.Running {
		s.LastProgressAt = time.Now()
	}
	if ev.ID == "" {
		ev.ID = newID()
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	s.Events = append(s.Events, ev)
	if len(s.Events) > 500 {
		s.Events = append([]UIEvent(nil), s.Events[len(s.Events)-500:]...)
	}
	if s.CurrentThread != "" {
		if t := s.Threads[s.CurrentThread]; t != nil {
			t.Events = append([]UIEvent(nil), s.Events...)
			t.UpdatedAt = ev.Timestamp
			if s.Model != "" {
				t.Model = s.Model
			}
		}
	}
	threadSnapshot := make(map[string]*ChatThread, len(s.Threads))
	for id, t := range s.Threads {
		if t == nil {
			continue
		}
		copy := *t
		copy.Events = append([]UIEvent(nil), t.Events...)
		threadSnapshot[id] = &copy
	}
	subs := make([]chan UIEvent, 0, len(s.subscribers))
	for ch := range s.subscribers {
		subs = append(subs, ch)
	}
	s.mu.Unlock()
	_ = saveThreads(threadSnapshot)

	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (s *AppState) Subscribe() chan UIEvent {
	ch := make(chan UIEvent, 32)
	s.mu.Lock()
	s.subscribers[ch] = struct{}{}
	s.mu.Unlock()
	return ch
}

func (s *AppState) Unsubscribe(ch chan UIEvent) {
	s.mu.Lock()
	delete(s.subscribers, ch)
	close(ch)
	s.mu.Unlock()
}
