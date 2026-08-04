// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *AppState) offerInstallMissingTool(ctx context.Context, project string, cfg Config, missing *ToolNotFoundError) (Config, string, bool, error) {
	if missing == nil || !toolInstallSupported(missing.Info.Name) {
		return cfg, "", false, missing
	}
	installAction := AgentAction{
		Action:  "install_tool",
		Tool:    missing.Info.Name,
		Message: fmt.Sprintf("%s wurde auf diesem Computer nicht gefunden. Soll LocalCode das Werkzeug jetzt installieren und den ursprünglichen Vorgang danach automatisch fortsetzen?", missing.Info.DisplayName),
	}
	preview := toolInstallPreview(missing.Info.Name)
	if strings.TrimSpace(missing.Detail) != "" {
		preview += "\n\nBisherige Suche:\n" + truncateText(missing.Detail, 18000)
	}
	approved, approvalErr := s.requestApprovalWithPreview(ctx, installAction, preview)
	if approvalErr != nil {
		return cfg, "", false, approvalErr
	}
	if !approved {
		return cfg, "Installation wurde vom Nutzer abgelehnt.", false, nil
	}
	s.AddEvent(UIEvent{Type: "action_running", Message: "Installiere " + missing.Info.DisplayName, Action: "install_tool", Path: missing.Info.Name, Preview: preview})
	installTimeout := 20 * time.Minute
	if profileForTool(missing.Info.Name).InstallKind == "vs-build-tools" {
		installTimeout = 90 * time.Minute
	}
	installCtx, cancel := context.WithTimeout(ctx, installTimeout)
	newCfg, installOutput, installErr := installKnownTool(installCtx, project, missing.Info.Name, cfg)
	cancel()
	if installErr != nil {
		detail := strings.TrimSpace(installOutput)
		if detail != "" {
			detail += "\n\n"
		}
		detail += "ERROR: " + installErr.Error()
		s.AddEvent(UIEvent{Type: "tool_error", Message: missing.Info.DisplayName + " konnte nicht installiert werden", Detail: detail, Action: "install_tool", Path: missing.Info.Name})
		return cfg, detail, false, installErr
	}
	newCfg = normalizeConfig(newCfg)
	s.mu.Lock()
	s.Config = newCfg
	s.mu.Unlock()
	if err := saveConfig(newCfg); err != nil {
		return cfg, installOutput, false, fmt.Errorf("tool installed but configuration could not be saved: %w", err)
	}
	verified := discoverTool(project, missing.Info.Name, newCfg, true)
	if !verified.Available {
		detail := installOutput + "\n\nInstallation meldete Erfolg, aber das Werkzeug wurde bei der anschließenden Prüfung nicht gefunden."
		s.AddEvent(UIEvent{Type: "tool_error", Message: "Installation konnte nicht verifiziert werden", Detail: detail, Action: "install_tool", Path: missing.Info.Name})
		return cfg, detail, false, errors.New("installed tool could not be rediscovered")
	}
	installDetail := installOutput + "\n\nVerifiziert: " + verified.Path
	if verified.Version != "" {
		installDetail += "\nVersion: " + verified.Version
	}
	s.AddEvent(UIEvent{Type: "action_done", Message: missing.Info.DisplayName + " installiert", Detail: truncateText(installDetail, 30000), Action: "install_tool", Path: verified.Path})
	s.recordAction("install_tool: " + missing.Info.Name)
	s.UpdateProjectState("Werkzeug " + missing.Info.Name + " installiert")
	return newCfg, installDetail, true, nil
}

func missingToolForAction(project string, cfg Config, a AgentAction) *ToolNotFoundError {
	name := ""
	switch a.Action {
	case "git":
		name = "git"
	case "build_project":
		name = detectProjectPlan(project).BuildTool
	case "deploy_android":
		plan := detectProjectPlan(project)
		if plan.BuildTool != "" && !discoverTool(project, plan.BuildTool, cfg, false).Available {
			name = plan.BuildTool
		} else {
			name = "adb"
		}
	case "run_tool":
		name = strings.TrimSpace(a.Tool)
	case "run_command":
		head, _, ok := splitCommandHead(a.Command)
		if !ok || strings.ContainsAny(head, `\/`) {
			return nil
		}
		if knownName, known := knownToolName(head); known {
			name = knownName
		}
	}
	if name == "" {
		return nil
	}
	info := discoverTool(project, name, cfg, false)
	if info.Available {
		return nil
	}
	var detail strings.Builder
	fmt.Fprintf(&detail, "%s wurde vor der genehmigten Aktion nicht gefunden.\n", info.DisplayName)
	if len(info.SearchedPath) > 0 {
		detail.WriteString("Durchsuchte Pfade:\n")
		for _, candidate := range info.SearchedPath {
			detail.WriteString("- " + candidate + "\n")
		}
	}
	return &ToolNotFoundError{Info: info, Detail: detail.String()}
}

func (s *AppState) executeActionWithToolRepair(ctx context.Context, project string, cfg Config, a AgentAction) (string, error) {
	currentCfg := cfg
	var installLog []string
	for attempt := 0; attempt < 4; attempt++ {
		result, err := executeAction(ctx, project, currentCfg, a)
		if len(installLog) > 0 {
			result = strings.Join(installLog, "\n\n") + "\n\nERNEUTER ORIGINALAUFRUF:\n" + result
		}
		if err == nil {
			return result, nil
		}
		var missing *ToolNotFoundError
		if !errors.As(err, &missing) || missing == nil || !toolInstallSupported(missing.Info.Name) {
			return result, err
		}
		newCfg, installDetail, installed, installErr := s.offerInstallMissingTool(ctx, project, currentCfg, missing)
		if installErr != nil {
			return strings.TrimSpace(result + "\n\n" + installDetail), installErr
		}
		if !installed {
			return strings.TrimSpace(result + "\n\n" + installDetail), err
		}
		installLog = append(installLog, installDetail)
		currentCfg = newCfg
	}
	return strings.Join(installLog, "\n\n"), errors.New("too many consecutive missing tool installations")
}
