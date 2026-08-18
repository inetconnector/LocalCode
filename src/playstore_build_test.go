// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlayStoreBuildHelperUsesProjectWrapperAndNeverPublishes(t *testing.T) {
	path := filepath.Join("..", "scripts", "build-playstore.ps1")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{
		"gradlew.bat",
		"bundleRelease",
		"assembleRelease",
		"Get-FileHash -Algorithm SHA256",
		"Test-AabSignature",
		"jarsigner",
		"jar verified",
		"signature_status",
		"PLAY_STORE_BUILD_RESULT_JSON",
		"never creates/replaces keystores",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("Play Store build safety/verification marker missing: %s", required)
		}
	}
	for _, forbidden := range []string{
		"keytool -genkey",
		"keytool.exe -genkey",
		"gradle publish",
		"publishBundle",
		"playPublisher",
		"service-account.json",
	} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("Play Store helper contains forbidden publishing/key-generation behavior: %s", forbidden)
		}
	}
}

func TestMobileRemoteContainsSafeProjectAndPlayStoreControls(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("static", "remote.html"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{
		"/remote/api/project-delete-preview",
		"action:'create_project'",
		"action:'delete_empty'",
		"action:'delete_recursive'",
		"preview.confirmation",
		"bundleRelease (.aab)",
		"Niemals automatisch zum Play Store hochladen",
		"Never upload to Google Play",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("mobile remote project/release control missing: %s", required)
		}
	}
	if strings.Contains(text, `data-decision="global"`) {
		t.Fatal("mobile UI must not expose global approval persistence")
	}
}
