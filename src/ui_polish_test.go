// SPDX-License-Identifier: Apache-2.0

package main

import (
	"io/fs"
	"strings"
	"testing"
)

func TestUIPolishLayerContainsSafeProjectManagementAndApprovalLayout(t *testing.T) {
	data, err := fs.ReadFile(staticFS, "static/ui_polish.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, fragment := range []string{
		"project-create-project",
		"project-create-folder",
		"project-rename-folder",
		"/api/project-delete-preview",
		"/api/project-quarantine",
		"/api/project-quarantine-action",
		"create_project",
		"create_folder",
		"rename_folder",
		"delete_empty",
		"delete_recursive",
		"PURGE ${entry.name}",
		"--rightW:280px",
		"max-height:46vh",
		"overflow-wrap:anywhere",
		"i18n.dictionaries.en",
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("UI polish layer is missing %q", fragment)
		}
	}
}

func TestLocalizationBootstrapLoadsBaseAndPolishSynchronously(t *testing.T) {
	data, err := fs.ReadFile(staticFS, "static/i18n.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, fragment := range []string{"/i18n_base.js", "/ui_polish.js", "document.write"} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("localization bootstrap is missing %q", fragment)
		}
	}
}

func TestDesktopExtrasMenuAndTopQRButton(t *testing.T) {
	data, err := fs.ReadFile(staticFS, "static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)

	if !strings.Contains(text, `data-menu="extras"`) || !strings.Contains(text, `id="menu-extras"`) {
		t.Fatalf("expected Extras menu in index.html")
	}

	helpIndex := strings.Index(text, `id="menu-help"`)
	if helpIndex == -1 {
		t.Fatalf("expected Help menu in index.html")
	}
	helpEnd := strings.Index(text[helpIndex:], `</div></div>`)
	if helpEnd == -1 {
		t.Fatalf("could not find end of Help menu")
	}
	helpBlock := text[helpIndex : helpIndex+helpEnd]
	if strings.Contains(helpBlock, `data-action="remote-pairing"`) {
		t.Fatalf("remote-pairing must not be inside Hilfe menu")
	}

	extrasIndex := strings.Index(text, `id="menu-extras"`)
	if extrasIndex == -1 {
		t.Fatalf("expected Extras menu in index.html")
	}
	extrasEnd := strings.Index(text[extrasIndex:], `</div></div>`)
	if extrasEnd == -1 {
		t.Fatalf("could not find end of Extras menu")
	}
	extrasBlock := text[extrasIndex : extrasIndex+extrasEnd]
	if !strings.Contains(extrasBlock, `data-action="remote-pairing"`) {
		t.Fatalf("expected remote-pairing inside Extras menu")
	}

	if !strings.Contains(text, `id="topRemotePairBtn"`) {
		t.Fatalf("expected topRemotePairBtn in index.html")
	}
	if !strings.Contains(text, `showRemotePairing`) {
		t.Fatalf("expected showRemotePairing handler in index.html")
	}
}
