// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	missionKnowledgeSchemaVersion = 1
	missionKnowledgeMaxEntries    = 128
	missionKnowledgeMaxTitle      = 160
	missionKnowledgeMaxSummary    = 4096
	missionKnowledgeMaxComponent  = 160
	missionKnowledgeMaxIdentity   = 160
	missionKnowledgeMaxFileBytes  = 512 << 10

	MissionKnowledgeArchitectureDecision = "architecture_decision"
	MissionKnowledgeSubsystemContract    = "subsystem_contract"
	MissionKnowledgeKnownFailure         = "known_failure"
	MissionKnowledgeTestEvidence         = "test_evidence"
)

var (
	missionKnowledgeFileMu       sync.Mutex
	errMissionKnowledgeInvalid   = errors.New("invalid mission knowledge")
	errMissionKnowledgeConflict  = errors.New("mission knowledge changed concurrently")
	errMissionKnowledgeCorrupt   = errors.New("corrupt mission knowledge")
)

type MissionKnowledgeEntry struct {
	ID             string    `json:"id"`
	Kind           string    `json:"kind"`
	Title          string    `json:"title"`
	Summary        string    `json:"summary"`
	Component      string    `json:"component,omitempty"`
	MissionID      string    `json:"mission_id,omitempty"`
	TaskID         string    `json:"task_id,omitempty"`
	EvidenceSHA256 string    `json:"evidence_sha256,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	DigestSHA256   string    `json:"digest_sha256"`
}

type MissionKnowledgeStore struct {
	SchemaVersion         int                     `json:"schema_version"`
	ProjectIdentitySHA256 string                  `json:"project_identity_sha256"`
	UpdatedAt             time.Time               `json:"updated_at"`
	Entries               []MissionKnowledgeEntry `json:"entries"`
}

type missionKnowledgeDigestInput struct {
	Kind           string `json:"kind"`
	Title          string `json:"title"`
	Summary        string `json:"summary"`
	Component      string `json:"component,omitempty"`
	MissionID      string `json:"mission_id,omitempty"`
	TaskID         string `json:"task_id,omitempty"`
	EvidenceSHA256 string `json:"evidence_sha256,omitempty"`
}

func missionKnowledgeKindValid(kind string) bool {
	switch kind {
	case MissionKnowledgeArchitectureDecision, MissionKnowledgeSubsystemContract, MissionKnowledgeKnownFailure, MissionKnowledgeTestEvidence:
		return true
	default:
		return false
	}
}

func boundedMissionKnowledgeText(value string, limit int, required bool) (string, error) {
	value = strings.TrimSpace(value)
	if required && value == "" {
		return "", errMissionKnowledgeInvalid
	}
	if len(value) > limit || strings.ContainsRune(value, '\x00') {
		return "", errMissionKnowledgeInvalid
	}
	return value, nil
}

func missionKnowledgeEntryDigest(entry MissionKnowledgeEntry) (string, error) {
	data, err := json.Marshal(missionKnowledgeDigestInput{
		Kind: entry.Kind, Title: entry.Title, Summary: entry.Summary, Component: entry.Component,
		MissionID: entry.MissionID, TaskID: entry.TaskID, EvidenceSHA256: entry.EvidenceSHA256,
	})
	if err != nil {
		return "", err
	}
	return missionSHA256Bytes(data), nil
}

func normalizeMissionKnowledgeEntry(entry MissionKnowledgeEntry, now time.Time) (MissionKnowledgeEntry, error) {
	entry.Kind = strings.TrimSpace(entry.Kind)
	if !missionKnowledgeKindValid(entry.Kind) {
		return MissionKnowledgeEntry{}, errMissionKnowledgeInvalid
	}
	var err error
	if entry.Title, err = boundedMissionKnowledgeText(entry.Title, missionKnowledgeMaxTitle, true); err != nil {
		return MissionKnowledgeEntry{}, err
	}
	if entry.Summary, err = boundedMissionKnowledgeText(entry.Summary, missionKnowledgeMaxSummary, true); err != nil {
		return MissionKnowledgeEntry{}, err
	}
	if entry.Component, err = boundedMissionKnowledgeText(entry.Component, missionKnowledgeMaxComponent, false); err != nil {
		return MissionKnowledgeEntry{}, err
	}
	if entry.MissionID, err = boundedMissionKnowledgeText(entry.MissionID, missionKnowledgeMaxIdentity, false); err != nil {
		return MissionKnowledgeEntry{}, err
	}
	if entry.TaskID, err = boundedMissionKnowledgeText(entry.TaskID, missionKnowledgeMaxIdentity, false); err != nil {
		return MissionKnowledgeEntry{}, err
	}
	entry.EvidenceSHA256 = strings.ToLower(strings.TrimSpace(entry.EvidenceSHA256))
	if entry.EvidenceSHA256 != "" && !validMissionVerificationDigest(entry.EvidenceSHA256) {
		return MissionKnowledgeEntry{}, errMissionKnowledgeInvalid
	}
	key := entry.Kind + "\x00" + strings.ToLower(entry.Title) + "\x00" + strings.ToLower(entry.Component)
	entry.ID = missionSHA256String(key)
	if now.IsZero() {
		now = time.Now()
	}
	entry.CreatedAt = now
	entry.UpdatedAt = now
	entry.DigestSHA256, err = missionKnowledgeEntryDigest(entry)
	if err != nil || !validMissionVerificationDigest(entry.DigestSHA256) {
		return MissionKnowledgeEntry{}, errMissionKnowledgeInvalid
	}
	return entry, nil
}

func missionKnowledgeLocation(project string) (string, string, error) {
	project = strings.TrimSpace(project)
	if project == "" {
		return "", "", errMissionKnowledgeInvalid
	}
	info, err := os.Stat(project)
	if err != nil || !info.IsDir() {
		return "", "", errMissionKnowledgeInvalid
	}
	identity := missionProjectIdentity(project)
	if !validMissionVerificationDigest(identity) {
		return "", "", errMissionKnowledgeInvalid
	}
	return filepath.Join(appDataDir(), "mission-knowledge", identity+".json"), identity, nil
}

func validateMissionKnowledgeStore(store MissionKnowledgeStore, identity string) error {
	if store.SchemaVersion != missionKnowledgeSchemaVersion || store.ProjectIdentitySHA256 != identity || !validMissionVerificationDigest(store.ProjectIdentitySHA256) || len(store.Entries) > missionKnowledgeMaxEntries {
		return errMissionKnowledgeCorrupt
	}
	seen := make(map[string]struct{}, len(store.Entries))
	for _, entry := range store.Entries {
		normalized, err := normalizeMissionKnowledgeEntry(entry, entry.UpdatedAt)
		if err != nil || normalized.ID != entry.ID || normalized.DigestSHA256 != entry.DigestSHA256 || entry.CreatedAt.IsZero() || entry.UpdatedAt.IsZero() || entry.UpdatedAt.Before(entry.CreatedAt) {
			return errMissionKnowledgeCorrupt
		}
		if _, exists := seen[entry.ID]; exists {
			return errMissionKnowledgeCorrupt
		}
		seen[entry.ID] = struct{}{}
	}
	return nil
}

func loadMissionKnowledgeFile(path, identity string) (MissionKnowledgeStore, error) {
	store := MissionKnowledgeStore{SchemaVersion: missionKnowledgeSchemaVersion, ProjectIdentitySHA256: identity, Entries: []MissionKnowledgeEntry{}}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return store, nil
	}
	if err != nil {
		return MissionKnowledgeStore{}, err
	}
	if len(data) == 0 || len(data) > missionKnowledgeMaxFileBytes {
		return MissionKnowledgeStore{}, errMissionKnowledgeCorrupt
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&store); err != nil {
		return MissionKnowledgeStore{}, fmt.Errorf("%w: %v", errMissionKnowledgeCorrupt, err)
	}
	if err := validateMissionKnowledgeStore(store, identity); err != nil {
		return MissionKnowledgeStore{}, err
	}
	return store, nil
}

func LoadMissionKnowledge(project string) (MissionKnowledgeStore, error) {
	path, identity, err := missionKnowledgeLocation(project)
	if err != nil {
		return MissionKnowledgeStore{}, err
	}
	missionKnowledgeFileMu.Lock()
	defer missionKnowledgeFileMu.Unlock()
	return loadMissionKnowledgeFile(path, identity)
}

func encodeMissionKnowledgeStore(store MissionKnowledgeStore) ([]byte, error) {
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	if len(data) > missionKnowledgeMaxFileBytes {
		return nil, errMissionKnowledgeInvalid
	}
	return data, nil
}

func writeNewMissionKnowledgeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if os.IsExist(err) {
		return errMissionKnowledgeConflict
	}
	if err != nil {
		return err
	}
	written, writeErr := file.Write(data)
	if writeErr == nil && written != len(data) {
		writeErr = fmt.Errorf("short mission knowledge write")
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(path)
		return writeErr
	}
	return closeErr
}

func UpsertMissionKnowledge(project string, incoming MissionKnowledgeEntry) (MissionKnowledgeStore, error) {
	path, identity, err := missionKnowledgeLocation(project)
	if err != nil {
		return MissionKnowledgeStore{}, err
	}
	now := time.Now()
	entry, err := normalizeMissionKnowledgeEntry(incoming, now)
	if err != nil {
		return MissionKnowledgeStore{}, err
	}

	missionKnowledgeFileMu.Lock()
	defer missionKnowledgeFileMu.Unlock()
	store, err := loadMissionKnowledgeFile(path, identity)
	if err != nil {
		return MissionKnowledgeStore{}, err
	}
	_, statErr := os.Stat(path)
	exists := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return MissionKnowledgeStore{}, statErr
	}
	var expected any
	if exists {
		version, versionErr := readFileVersion(path)
		if versionErr != nil {
			return MissionKnowledgeStore{}, versionErr
		}
		expected = version
	}

	updated := false
	for index := range store.Entries {
		if store.Entries[index].ID != entry.ID {
			continue
		}
		entry.CreatedAt = store.Entries[index].CreatedAt
		store.Entries[index] = entry
		updated = true
		break
	}
	if !updated {
		store.Entries = append(store.Entries, entry)
	}
	sort.SliceStable(store.Entries, func(i, j int) bool {
		return store.Entries[i].UpdatedAt.After(store.Entries[j].UpdatedAt)
	})
	if len(store.Entries) > missionKnowledgeMaxEntries {
		store.Entries = append([]MissionKnowledgeEntry(nil), store.Entries[:missionKnowledgeMaxEntries]...)
	}
	store.SchemaVersion = missionKnowledgeSchemaVersion
	store.ProjectIdentitySHA256 = identity
	store.UpdatedAt = now
	data, err := encodeMissionKnowledgeStore(store)
	if err != nil {
		return MissionKnowledgeStore{}, err
	}
	if !exists {
		if err := writeNewMissionKnowledgeFile(path, data); err != nil {
			return MissionKnowledgeStore{}, err
		}
	} else {
		version := expected.(fileVersion)
		if err := atomicWriteFileIfVersion(path, data, 0o600, version); err != nil {
			return MissionKnowledgeStore{}, fmt.Errorf("%w: %v", errMissionKnowledgeConflict, err)
		}
	}
	return store, nil
}
