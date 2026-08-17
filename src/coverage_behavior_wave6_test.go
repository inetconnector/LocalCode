// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"
	"time"
)

func TestGitReadOnlyClassifierBranchMatrix(t *testing.T) {
	branchCases := []struct {
		args []string
		want bool
	}{
		{nil, true},
		{[]string{"--show-current"}, true},
		{[]string{"--list", "--all", "-vv", "--ignore-case"}, true},
		{[]string{"--sort", "refname"}, true},
		{[]string{"--format", "%(refname)"}, true},
		{[]string{"--contains", "HEAD"}, true},
		{[]string{"--no-contains", "HEAD"}, true},
		{[]string{"--merged", "HEAD"}, true},
		{[]string{"--no-merged", "HEAD"}, true},
		{[]string{"--points-at", "HEAD"}, true},
		{[]string{"--sort=-committerdate"}, true},
		{[]string{"--format=%(refname)"}, true},
		{[]string{"--color=always"}, true},
		{[]string{"--column=always"}, true},
		{[]string{"--contains=HEAD"}, true},
		{[]string{"--no-contains=HEAD"}, true},
		{[]string{"--merged=HEAD"}, true},
		{[]string{"--no-merged=HEAD"}, true},
		{[]string{"--points-at=HEAD"}, true},
		{[]string{"new-branch"}, false},
		{[]string{"--delete", "old"}, false},
		{[]string{"--unknown"}, false},
	}
	for _, tc := range branchCases {
		if got := gitBranchArgsReadOnly(tc.args); got != tc.want {
			t.Fatalf("gitBranchArgsReadOnly(%v)=%v want %v", tc.args, got, tc.want)
		}
	}

	tagCases := []struct {
		args []string
		want bool
	}{
		{nil, true},
		{[]string{"--list"}, true},
		{[]string{"-l", "v*"}, true},
		{[]string{"-n"}, true},
		{[]string{"-n5"}, true},
		{[]string{"--contains", "HEAD"}, true},
		{[]string{"--no-contains", "HEAD"}, true},
		{[]string{"--points-at", "HEAD"}, true},
		{[]string{"--sort", "refname"}, true},
		{[]string{"--format", "%(refname)"}, true},
		{[]string{"--contains"}, false},
		{[]string{"--format"}, false},
		{[]string{"--contains=HEAD"}, true},
		{[]string{"--merged"}, true},
		{[]string{"--no-merged=HEAD"}, true},
		{[]string{"--ignore-case"}, true},
		{[]string{"--delete", "v1"}, false},
		{[]string{"v1"}, false},
		{[]string{"--list", "v1*"}, true},
	}
	for _, tc := range tagCases {
		if got := gitTagArgsReadOnly(tc.args); got != tc.want {
			t.Fatalf("gitTagArgsReadOnly(%v)=%v want %v", tc.args, got, tc.want)
		}
	}

	remoteCases := []struct {
		args []string
		want bool
	}{
		{nil, true},
		{[]string{"-v"}, true},
		{[]string{"--verbose"}, true},
		{[]string{"get-url"}, false},
		{[]string{"get-url", "origin"}, true},
		{[]string{"get-url", "--all", "origin"}, true},
		{[]string{"get-url", "set-url", "origin"}, false},
		{[]string{"get-url", "remove", "origin"}, false},
		{[]string{"add", "origin", "https://example.invalid/repo"}, false},
	}
	for _, tc := range remoteCases {
		if got := gitRemoteArgsReadOnly(tc.args); got != tc.want {
			t.Fatalf("gitRemoteArgsReadOnly(%v)=%v want %v", tc.args, got, tc.want)
		}
	}
}

func TestSmallSecurityHelperBranches(t *testing.T) {
	if !remoteDeviceExpiresAt(RemoteDevice{}).IsZero() {
		t.Fatal("zero paired-at must produce zero expiry")
	}
	paired := time.Date(2026, time.August, 1, 12, 0, 0, 0, time.UTC)
	got := remoteDeviceExpiresAt(RemoteDevice{PairedAt: paired})
	want := paired.Add(remoteDeviceTokenTTL)
	if !got.Equal(want) {
		t.Fatalf("expiry=%v want %v", got, want)
	}
	if sameQuestion("", "anything") || sameQuestion("anything", "") {
		t.Fatal("empty normalized questions must never match")
	}
	if _, ok := knownToolName("definitely-not-a-localcode-tool"); ok {
		t.Fatal("unknown tool must not be classified as known")
	}
}
