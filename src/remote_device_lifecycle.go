// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"
)

const remoteDeviceTokenTTL = 30 * 24 * time.Hour

type RemoteDeviceView struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	PairedAt   time.Time `json:"paired_at"`
	LastSeenAt time.Time `json:"last_seen_at,omitempty"`
	ExpiresAt  time.Time `json:"expires_at"`
}

func remoteDeviceExpiresAt(device RemoteDevice) time.Time {
	if device.PairedAt.IsZero() {
		return time.Time{}
	}
	return device.PairedAt.Add(remoteDeviceTokenTTL)
}

func remoteDeviceExpired(device RemoteDevice, now time.Time) bool {
	expires := remoteDeviceExpiresAt(device)
	return expires.IsZero() || !now.Before(expires)
}

func remoteDeviceView(device RemoteDevice) RemoteDeviceView {
	return RemoteDeviceView{
		ID:         device.ID,
		Name:       device.Name,
		PairedAt:   device.PairedAt,
		LastSeenAt: device.LastSeenAt,
		ExpiresAt:  remoteDeviceExpiresAt(device),
	}
}

func (s *AppState) RemoteDeviceViews() []RemoteDeviceView {
	now := time.Now()
	s.mu.RLock()
	views := make([]RemoteDeviceView, 0, len(s.Config.RemoteDevices))
	for _, device := range s.Config.RemoteDevices {
		if remoteDeviceExpired(device, now) {
			continue
		}
		views = append(views, remoteDeviceView(device))
	}
	s.mu.RUnlock()
	sort.SliceStable(views, func(i, j int) bool { return views[i].PairedAt.After(views[j].PairedAt) })
	return views
}

func (s *AppState) RevokeRemoteDevice(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("remote device id is required")
	}
	_, err := s.mutateConfig(func(cfg *Config) error {
		out := make([]RemoteDevice, 0, len(cfg.RemoteDevices))
		found := false
		for _, device := range cfg.RemoteDevices {
			if strings.EqualFold(strings.TrimSpace(device.ID), id) {
				found = true
				continue
			}
			out = append(out, device)
		}
		if !found {
			return errors.New("remote device not found")
		}
		cfg.RemoteDevices = out
		return nil
	})
	return err
}

func (s *Server) handleRemoteDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"devices": s.state.RemoteDeviceViews()})
}

func (s *Server) handleRemoteRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.state.RevokeRemoteDevice(req.ID); err != nil {
		if isConfigMutationError(err) {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true})
}
