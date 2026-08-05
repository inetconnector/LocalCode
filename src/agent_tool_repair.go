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
		Message: localizeConfigText(cfg, fmt.Sprintf("%s wurde auf diesem Computer nicht gefunden. Soll LocalCode das Werkzeug jetzt installieren und den ursprünglichen Vorgang danach automatisch fortsetzen?", missing.Info.DisplayName), fmt.Sprintf("%s was not found on this computer. Should LocalCode install the tool now and automatically continue the original operation afterwards?", missing.Info.DisplayName)),
	}
	preview := toolInstallPreview(missing.Info.Name, cfg)
	if strings.TrimSpace(missing.Detail) != "" {
		preview += localizeConfigText(cfg, "\n\nBisherige Suche:\n", "\n\nPrevious search:\n") + truncateText(missing.Detail, 18000)
	}
	approved, approvalErr := s.requestApprovalWithPreview(ctx, project, installAction, preview)
	if approvalErr != nil {
		return cfg, "", false, approvalErr
	}
	if !approved {
		return cfg, localizeConfigText(cfg, "Installation wurde vom Nutzer abgelehnt.", "Installation was declined by the user."), false, nil
	}
	s.AddEvent(UIEvent{Type: "action_running", Message: localizeConfigText(cfg, "Installiere ", "Installing ") + missing.Info.DisplayName, Action: "install_tool", Path: missing.Info.Name, Preview: preview})
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
		detail += localizeConfigText(cfg, "FEHLER: ", "ERROR: ") + installErr.Error()
		s.AddEvent(UIEvent{Type: "tool_error", Message: localizeConfigText(cfg, missing.Info.DisplayName+" konnte nicht installiert werden", "Could not install "+missing.Info.DisplayName), Detail: detail, Action: "install_tool", Path: missing.Info.Name})
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
		detail := installOutput + localizeConfigText(newCfg, "\n\nInstallation meldete Erfolg, aber das Werkzeug wurde bei der anschließenden Prüfung nicht gefunden.", "\n\nThe installer reported success, but the tool was not found during the follow-up verification.")
		s.AddEvent(UIEvent{Type: "tool_error", Message: localizeConfigText(newCfg, "Installation konnte nicht verifiziert werden", "Installation could not be verified"), Detail: detail, Action: "install_tool", Path: missing.Info.Name})
		return cfg, detail, false, errors.New("installed tool could not be rediscovered")
	}
	installDetail := installOutput + localizeConfigText(newCfg, "\n\nVerifiziert: ", "\n\nVerified: ") + verified.Path
	if verified.Version != "" {
		installDetail += "\nVersion: " + verified.Version
	}
	s.AddEvent(UIEvent{Type: "action_done", Message: localizeConfigText(newCfg, missing.Info.DisplayName+" installiert", missing.Info.DisplayName+" installed"), Detail: truncateText(installDetail, 30000), Action: "install_tool", Path: verified.Path})
	s.recordAction("install_tool: " + missing.Info.Name)
	s.UpdateProjectState(localizeConfigText(newCfg, "Werkzeug "+missing.Info.Name+" installiert", "Tool "+missing.Info.Name+" installed"))
	return newCfg, installDetail, true, nil
}

func missingToolForAction(project string, cfg Config, a AgentAction) *ToolNotFoundError {
	name := ""
	switch a.Action {
	case "git", "git_commit":
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
	fmt.Fprintf(&detail, localizeConfigText(cfg, "%s wurde vor der genehmigten Aktion nicht gefunden.\n", "%s was not found before the approved action.\n"), info.DisplayName)
	if len(info.SearchedPath) > 0 {
		detail.WriteString(localizeConfigText(cfg, "Durchsuchte Pfade:\n", "Searched paths:\n"))
		for _, candidate := range info.SearchedPath {
			detail.WriteString("- " + candidate + "\n")
		}
	}
	return &ToolNotFoundError{Info: info, Detail: detail.String()}
}

func (s *AppState) executeActionWithToolRepair(ctx context.Context, project string, cfg Config, a AgentAction) (string, error) {
	if a.Action == "aider_edit" || a.Action == "aider_repo_map" || a.Action == "aider_lint" || a.Action == "aider_test" {
		currentCfg := cfg
		for attempt := 0; attempt < 2; attempt++ {
			result, err := s.executeAiderAction(ctx, project, currentCfg, a)
			if err == nil {
				return result, nil
			}
			var missing *AiderNotInstalledError
			if !errors.As(err, &missing) {
				return result, err
			}
			newCfg, installDetail, installed, installErr := s.offerInstallAider(ctx, project, currentCfg)
			if installErr != nil {
				return strings.TrimSpace(result + "\n\n" + installDetail), installErr
			}
			if !installed {
				return strings.TrimSpace(result + "\n\n" + installDetail), err
			}
			currentCfg = newCfg
		}
		return "", errors.New("Aider installation retry limit reached")
	}
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
