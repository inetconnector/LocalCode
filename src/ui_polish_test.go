// SPDX-License-Identifier: Apache-2.0

package main

import (
	"io/fs"
	"strings"
	"testing"
)

func TestUIPolishLayerContainsFolderManagementAndApprovalLayout(t *testing.T) {
	data, err := fs.ReadFile(staticFS, "static/ui_polish.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, fragment := range []string{
		"project-create-folder",
		"project-rename-folder",
		"project-delete-empty",
		"project-delete-recursive",
		"create_folder",
		"rename_folder",
		"delete_empty",
		"delete_recursive",
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
