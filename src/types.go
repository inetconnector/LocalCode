// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"log"
	"sync"
	"time"
)

type MCPServerConfig struct {
	Name          string            `json:"name"`
	DisplayName   string            `json:"display_name,omitempty"`
	Description   string            `json:"description,omitempty"`
	Enabled       bool              `json:"enabled"`
	Transport     string            `json:"transport"` // builtin | stdio | streamable-http
	Preset        string            `json:"preset,omitempty"`
	Command       string            `json:"command,omitempty"`
	Args          []string          `json:"args,omitempty"`
	URL           string            `json:"url,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	TimeoutSec    int               `json:"timeout_sec,omitempty"`
	AutoInstall   bool              `json:"auto_install,omitempty"`
	ProjectScoped bool              `json:"project_scoped,omitempty"`
	AuthEnv       string            `json:"auth_env,omitempty"`
	ReadOnly      bool              `json:"read_only,omitempty"`
}

type MCPServerStatus struct {
	Name         string   `json:"name"`
	DisplayName  string   `json:"display_name"`
	Preset       string   `json:"preset,omitempty"`
	Enabled      bool     `json:"enabled"`
	Installed    bool     `json:"installed"`
	Connected    bool     `json:"connected"`
	AuthRequired bool     `json:"auth_required,omitempty"`
	ToolCount    int      `json:"tool_count,omitempty"`
	Tools        []string `json:"tools,omitempty"`
	Version      string   `json:"version,omitempty"`
	Source       string   `json:"source,omitempty"`
	Error        string   `json:"error,omitempty"`
}

type Config struct {
	SchemaVersion  int               `json:"schema_version"`
	RootProjectDir string            `json:"root_project_dir"`
	LastProject    string            `json:"last_project"`
	LastModel      string            `json:"last_model"`
	ProjectAliases map[string]string `json:"project_aliases,omitempty"`
	PinnedProjects []string          `json:"pinned_projects,omitempty"`
	HiddenProjects []string          `json:"hidden_projects,omitempty"`
	Memories       []MemoryEntry     `json:"memories,omitempty"`
	Port           int               `json:"port"`

	OllamaURL                         string  `json:"ollama_url"`
	SetupDownloadsEnabled             bool    `json:"setup_downloads_enabled"`
	OllamaAutoInstall                 bool    `json:"ollama_auto_install"`
	OllamaAutoPull                    bool    `json:"ollama_auto_pull"`
	OllamaDefaultModel                string  `json:"ollama_default_model"`
	EditingEngine                     string  `json:"editing_engine"` // aider | claude | opencode | native
	AiderEnabled                      bool    `json:"aider_enabled"`
	AiderAutoInstall                  bool    `json:"aider_auto_install"`
	AiderVersion                      string  `json:"aider_version"`
	AiderExecutable                   string  `json:"aider_executable,omitempty"`
	AiderMainModel                    string  `json:"aider_main_model,omitempty"`
	AiderArchitectMode                bool    `json:"aider_architect_mode"`
	AiderArchitectModel               string  `json:"aider_architect_model,omitempty"`
	AiderEditorModel                  string  `json:"aider_editor_model,omitempty"`
	AiderEditFormat                   string  `json:"aider_edit_format"`
	AiderEditorEditFormat             string  `json:"aider_editor_edit_format"`
	AiderMapTokens                    int     `json:"aider_map_tokens"`
	AiderMaxChatHistoryTokens         int     `json:"aider_max_chat_history_tokens"`
	AiderAutoLint                     bool    `json:"aider_auto_lint"`
	AiderAutoTest                     bool    `json:"aider_auto_test"`
	AiderLintCommand                  string  `json:"aider_lint_command,omitempty"`
	AiderTestCommand                  string  `json:"aider_test_command,omitempty"`
	AiderUseGit                       bool    `json:"aider_use_git"`
	AiderAutoCommits                  bool    `json:"aider_auto_commits"`
	ClaudeCodeEnabled                 bool    `json:"claude_code_enabled"`
	ClaudeCodeAutoInstall             bool    `json:"claude_code_auto_install"`
	ClaudeCodeChannel                 string  `json:"claude_code_channel"`
	ClaudeCodeExecutable              string  `json:"claude_code_executable,omitempty"`
	ClaudeCodeModel                   string  `json:"claude_code_model,omitempty"`
	ClaudeCodePermissionMode          string  `json:"claude_code_permission_mode"`
	ClaudeCodeMaxTurns                int     `json:"claude_code_max_turns"`
	OpenCodeEnabled                   bool    `json:"opencode_enabled"`
	OpenCodeAutoInstall               bool    `json:"opencode_auto_install"`
	OpenCodeVersion                   string  `json:"opencode_version"`
	OpenCodeExecutable                string  `json:"opencode_executable,omitempty"`
	OpenCodeModel                     string  `json:"opencode_model,omitempty"`
	OpenCodeAgent                     string  `json:"opencode_agent,omitempty"`
	OpenCodeAutoApprove               bool    `json:"opencode_auto_approve"`
	ContextLength                     int     `json:"context_length"`
	ContextCompactionEnabled          bool    `json:"context_compaction_enabled"`
	ContextCompactionThresholdPercent int     `json:"context_compaction_threshold_percent"`
	ContextCompactionKeepRecent       int     `json:"context_compaction_keep_recent"`
	MaxAgentSteps                     int     `json:"max_agent_steps"`
	CommandTimeout                    int     `json:"command_timeout_seconds"`
	ModelTimeout                      int     `json:"model_timeout_seconds"`
	ApprovalMode                      string  `json:"approval_mode"` // strict | balanced | auto | dangerous
	SandboxMode                       string  `json:"sandbox_mode"`  // project | workspace | unrestricted
	NetworkEnabled                    bool    `json:"network_enabled"`
	WebSearchProvider                 string  `json:"web_search_provider"` // disabled | duckduckgo | ollama
	WebSearchAPIKeyEnv                string  `json:"web_search_api_key_env"`
	WebSearchMaxResults               int     `json:"web_search_max_results"`
	WebFetchMaxBytes                  int64   `json:"web_fetch_max_bytes"`
	ImageGeneratorProvider            string  `json:"image_generator_provider"` // disabled | automatic1111
	ImageGeneratorURL                 string  `json:"image_generator_url"`
	ImageGeneratorSteps               int     `json:"image_generator_steps"`
	ImageGeneratorCFGScale            float64 `json:"image_generator_cfg_scale"`

	GitEnabled        bool   `json:"git_enabled"`
	AutoStateUpdate   bool   `json:"auto_state_update"`
	StateFile         string `json:"state_file"`
	CreateProjectDocs bool   `json:"create_project_docs"`

	AllowedRoots           []string          `json:"allowed_roots"`
	BlockedCommandPatterns []string          `json:"blocked_command_patterns"`
	ApprovalRules          []ApprovalRule    `json:"approval_rules"`
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

type MemoryEntry struct {
	ID        string    `json:"id"`
	Scope     string    `json:"scope"` // project | global
	Project   string    `json:"project,omitempty"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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
	Version             string      `json:"version"`
	OllamaOnline        bool        `json:"ollama_online"`
	OllamaURL           string      `json:"ollama_url,omitempty"`
	OllamaError         string      `json:"ollama_error,omitempty"`
	EditingEngine       string      `json:"editing_engine"`
	AiderInstalled      bool        `json:"aider_installed"`
	AiderVersion        string      `json:"aider_version,omitempty"`
	EngineInstalled     bool        `json:"engine_installed"`
	EngineVersion       string      `json:"engine_version,omitempty"`
	EngineExecutable    string      `json:"engine_executable,omitempty"`
	EngineAuthenticated bool        `json:"engine_authenticated"`
	EngineError         string      `json:"engine_error,omitempty"`
	Models              []ModelInfo `json:"models"`
	SelectedModel       string      `json:"selected_model"`
	GPU                 string      `json:"gpu,omitempty"`
	RootDir             string      `json:"root_dir"`
	Project             string      `json:"project,omitempty"`
	Running             bool        `json:"running"`
	GitAvailable        bool        `json:"git_available"`
	MCPCount            int         `json:"mcp_count"`
	RunID               string      `json:"run_id,omitempty"`
	RunPhase            string      `json:"run_phase,omitempty"`
	RunStartedAt        time.Time   `json:"run_started_at,omitempty"`
	LastProgressAt      time.Time   `json:"last_progress_at,omitempty"`
	ResolvedLanguage    string      `json:"resolved_language"`
	SystemLanguage      string      `json:"system_language"`
	SupportedLanguages  []string    `json:"supported_languages"`
}

type UIEvent struct {
	ID          string              `json:"id"`
	ThreadID    string              `json:"thread_id,omitempty"`
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
	Result  chan ApprovalDecision
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

	LastTask         string
	LastSummary      string
	ActionLog        []string
	LastAiderBackup  string
	LastEngineBackup string

	subscribers         map[chan UIEvent]struct{}
	threadSaveCh        chan map[string]*ChatThread
	threadSaveStop      chan struct{}
	threadSaveDone      chan struct{}
	threadSaveCloseOnce sync.Once
}

func NewAppState(cfg Config, ollama *OllamaClient) *AppState {
	threads := loadThreads()
	state := &AppState{
		Config:         cfg,
		Ollama:         ollama,
		Project:        cfg.LastProject,
		Model:          cfg.LastModel,
		Threads:        threads,
		subscribers:    make(map[chan UIEvent]struct{}),
		threadSaveCh:   make(chan map[string]*ChatThread, 1),
		threadSaveStop: make(chan struct{}),
		threadSaveDone: make(chan struct{}),
	}
	go state.threadSaveWorker()
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
	if ev.ThreadID == "" {
		ev.ThreadID = s.CurrentThread
	}
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
	s.queueThreadSave(threadSnapshot)

	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (s *AppState) queueThreadSave(snapshot map[string]*ChatThread) {
	if s.threadSaveCh == nil || s.threadSaveStop == nil {
		if err := saveThreads(snapshot); err != nil {
			log.Printf("saving chat threads failed: %v", err)
		}
		return
	}
	select {
	case <-s.threadSaveStop:
		return
	default:
	}
	select {
	case s.threadSaveCh <- snapshot:
		return
	default:
	}
	// Replace an older pending snapshot with the newest complete state. The
	// background writer is the sole file writer, so temporary files never race.
	select {
	case <-s.threadSaveCh:
	default:
	}
	select {
	case <-s.threadSaveStop:
		return
	case s.threadSaveCh <- snapshot:
	default:
	}
}

func (s *AppState) threadSaveWorker() {
	defer close(s.threadSaveDone)
	for {
		select {
		case snapshot := <-s.threadSaveCh:
			// Briefly coalesce event bursts without delaying the agent or the UI.
			timer := time.NewTimer(120 * time.Millisecond)
		coalesce:
			for {
				select {
				case newer := <-s.threadSaveCh:
					snapshot = newer
				case <-timer.C:
					break coalesce
				case <-s.threadSaveStop:
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					for {
						select {
						case newer := <-s.threadSaveCh:
							snapshot = newer
						default:
							if err := saveThreads(snapshot); err != nil {
								log.Printf("saving chat threads failed during shutdown: %v", err)
							}
							return
						}
					}
				}
			}
			if err := saveThreads(snapshot); err != nil {
				log.Printf("saving chat threads failed: %v", err)
			}
		case <-s.threadSaveStop:
			var latest map[string]*ChatThread
			for {
				select {
				case snapshot := <-s.threadSaveCh:
					latest = snapshot
				default:
					if latest != nil {
						if err := saveThreads(latest); err != nil {
							log.Printf("saving chat threads failed during shutdown: %v", err)
						}
					}
					return
				}
			}
		}
	}
}

// Close flushes the newest queued chat snapshot and stops the persistence
// worker. It is safe to call more than once.
func (s *AppState) Close() {
	defaultMCPManager.Close()
	if s == nil || s.threadSaveStop == nil || s.threadSaveDone == nil {
		return
	}
	s.threadSaveCloseOnce.Do(func() {
		close(s.threadSaveStop)
		<-s.threadSaveDone
	})
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
