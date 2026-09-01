// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultComputeMeshGatewayURL   = "https://computemesh.inetconnector.com"
	defaultComputeMeshLocalNodeURL = "http://localhost:8080"
)

type ComputeMeshStatus struct {
	Online          bool        `json:"online"`
	URL             string      `json:"url"`
	LocalNodeURL    string      `json:"local_node_url,omitempty"`
	NodeID          string      `json:"node_id,omitempty"`
	NodeStatus      string      `json:"node_status,omitempty"`
	GPU             string      `json:"gpu,omitempty"`
	VRAMPool        string      `json:"vram_pool,omitempty"`
	Account         string      `json:"account,omitempty"`
	KeySource       string      `json:"key_source,omitempty"`
	ActiveKeyMasked string      `json:"active_key_masked,omitempty"`
	LatencyMs       int64       `json:"latency_ms,omitempty"`
	Models          []ModelInfo `json:"models,omitempty"`
	DirectLocal     bool        `json:"direct_local,omitempty"`
	Error           string      `json:"error,omitempty"`
}

type computeMeshConfigFile struct {
	ProviderAccount string `json:"provider_account"`
	ProviderKey     string `json:"provider_key"`
	APIKey          string `json:"api_key"`
	Token           string `json:"token"`
	AuthToken       string `json:"auth_token"`
	Key             string `json:"key"`
	GatewayURL      string `json:"gateway_url"`
	URL             string `json:"url"`
	LocalNodeID     string `json:"local_node_id"`
	NodeID          string `json:"node_id"`
	LocalNodeURL    string `json:"local_node_url"`
	GPU             string `json:"gpu"`
	VRAM            string `json:"vram"`
	Provider        *struct {
		APIKey  string `json:"api_key"`
		Key     string `json:"key"`
		Account string `json:"account"`
	} `json:"provider,omitempty"`
}

func MaskAPIKey(key string) string {
	k := strings.TrimSpace(key)
	if len(k) <= 8 {
		if len(k) == 0 {
			return ""
		}
		return "****"
	}
	if len(k) > 16 {
		return k[:12] + "…" + k[len(k)-4:]
	}
	return k[:4] + "…" + k[len(k)-3:]
}

func ProbeRunningLocalComputeMeshNode(ctx context.Context) (nodeURL string, nodeStatus string, nodeID string, directModels []ModelInfo) {
	candidates := []string{
		"http://127.0.0.1:8080",
		"http://localhost:8080",
		"http://127.0.0.1:8081",
		"http://127.0.0.1:8000",
		"http://127.0.0.1:11435",
	}

	type probeResult struct {
		url    string
		status string
		id     string
		models []ModelInfo
		ok     bool
	}

	ch := make(chan probeResult, len(candidates))
	var wg sync.WaitGroup

	client := &http.Client{Timeout: 1200 * time.Millisecond}

	for _, cand := range candidates {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			probeCtx, cancel := context.WithTimeout(ctx, 1200*time.Millisecond)
			defer cancel()

			req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, url+"/", nil)
			if err != nil {
				return
			}
			resp, err := client.Do(req)
			if err != nil {
				// Try /api/tags
				reqTags, _ := http.NewRequestWithContext(probeCtx, http.MethodGet, url+"/api/tags", nil)
				if reqTags != nil {
					if respTags, errTags := client.Do(reqTags); errTags == nil {
						defer respTags.Body.Close()
						var tr ollamaTagsResponse
						var models []ModelInfo
						if json.NewDecoder(respTags.Body).Decode(&tr) == nil {
							for _, m := range tr.Models {
								models = append(models, ModelInfo{Name: m.Name, Size: m.Size, ModifiedAt: m.ModifiedAt.Format(time.RFC3339)})
							}
						}
						ch <- probeResult{url: url, status: "🟢 Online & Bereit (Serving)", id: "test-node-custom", models: models, ok: true}
						return
					}
				}
				return
			}
			defer resp.Body.Close()

			res := probeResult{url: url, status: "🟢 Online & Bereit (Serving)", id: "test-node-custom", ok: true}
			// Probe tags
			reqTags, _ := http.NewRequestWithContext(probeCtx, http.MethodGet, url+"/api/tags", nil)
			if reqTags != nil {
				if respTags, errTags := client.Do(reqTags); errTags == nil {
					defer respTags.Body.Close()
					var tr ollamaTagsResponse
					if json.NewDecoder(respTags.Body).Decode(&tr) == nil {
						for _, m := range tr.Models {
							res.models = append(res.models, ModelInfo{Name: m.Name, Size: m.Size, ModifiedAt: m.ModifiedAt.Format(time.RFC3339)})
						}
					}
				}
			}
			ch <- res
		}(cand)
	}

	wg.Wait()
	close(ch)

	for res := range ch {
		if res.ok {
			return res.url, res.status, res.id, res.models
		}
	}

	return "", "", "", nil
}

func AutoDetectComputeMeshCredentials() (apiKey, gatewayURL, account, nodeID, localNodeURL, source string) {
	gatewayURL = defaultComputeMeshGatewayURL
	localNodeURL = defaultComputeMeshLocalNodeURL

	// 1. Environment variables
	for _, envVar := range []string{"COMPUTEMESH_API_KEY", "COMPUTEMESH_PROVIDER_KEY", "COMPUTEMESH_KEY", "COMPUTEMESH_AUTH_TOKEN"} {
		if val := strings.TrimSpace(os.Getenv(envVar)); val != "" {
			apiKey = val
			source = "environment (" + envVar + ")"
			break
		}
	}
	if val := strings.TrimSpace(os.Getenv("COMPUTEMESH_URL")); val != "" {
		gatewayURL = val
	}
	if val := strings.TrimSpace(os.Getenv("COMPUTEMESH_NODE_URL")); val != "" {
		localNodeURL = val
	}

	// 2. Scan %USERPROFILE%/.computemesh/
	home := userProfileDir()
	dotMeshDir := filepath.Join(home, ".computemesh")
	jsonFiles := []string{
		"provider_config.json",
		"node_registry.json",
		"credentials.json",
		"config.json",
	}

	for _, filename := range jsonFiles {
		filePath := filepath.Join(dotMeshDir, filename)
		if data, err := os.ReadFile(filePath); err == nil {
			var cfg computeMeshConfigFile
			if json.Unmarshal(data, &cfg) == nil {
				if apiKey == "" {
					if cfg.ProviderKey != "" {
						apiKey = strings.TrimSpace(cfg.ProviderKey)
						source = fmt.Sprintf(".computemesh/%s (provider_key)", filename)
					} else if cfg.APIKey != "" {
						apiKey = strings.TrimSpace(cfg.APIKey)
						source = fmt.Sprintf(".computemesh/%s (api_key)", filename)
					} else if cfg.Token != "" {
						apiKey = strings.TrimSpace(cfg.Token)
						source = fmt.Sprintf(".computemesh/%s (token)", filename)
					} else if cfg.AuthToken != "" {
						apiKey = strings.TrimSpace(cfg.AuthToken)
						source = fmt.Sprintf(".computemesh/%s (auth_token)", filename)
					} else if cfg.Key != "" {
						apiKey = strings.TrimSpace(cfg.Key)
						source = fmt.Sprintf(".computemesh/%s (key)", filename)
					} else if cfg.Provider != nil {
						if cfg.Provider.APIKey != "" {
							apiKey = strings.TrimSpace(cfg.Provider.APIKey)
							source = fmt.Sprintf(".computemesh/%s (provider.api_key)", filename)
						} else if cfg.Provider.Key != "" {
							apiKey = strings.TrimSpace(cfg.Provider.Key)
							source = fmt.Sprintf(".computemesh/%s (provider.key)", filename)
						}
					}
				}
				if cfg.GatewayURL != "" && gatewayURL == defaultComputeMeshGatewayURL {
					gatewayURL = strings.TrimSpace(cfg.GatewayURL)
				} else if cfg.URL != "" && gatewayURL == defaultComputeMeshGatewayURL {
					gatewayURL = strings.TrimSpace(cfg.URL)
				}
				if cfg.ProviderAccount != "" && account == "" {
					account = strings.TrimSpace(cfg.ProviderAccount)
				} else if cfg.Provider != nil && cfg.Provider.Account != "" && account == "" {
					account = strings.TrimSpace(cfg.Provider.Account)
				}
				if cfg.LocalNodeID != "" && nodeID == "" {
					nodeID = strings.TrimSpace(cfg.LocalNodeID)
				} else if cfg.NodeID != "" && nodeID == "" {
					nodeID = strings.TrimSpace(cfg.NodeID)
				}
				if cfg.LocalNodeURL != "" {
					localNodeURL = strings.TrimSpace(cfg.LocalNodeURL)
				}
			}
		}
	}

	// 3. Scan plain text token files
	txtFiles := []string{
		"node_auth_token.txt",
		"token.txt",
		"api_key.txt",
		"provider_key.txt",
	}
	for _, filename := range txtFiles {
		if apiKey == "" {
			tokenPath := filepath.Join(dotMeshDir, filename)
			if data, err := os.ReadFile(tokenPath); err == nil {
				token := strings.TrimSpace(string(data))
				if token != "" {
					apiKey = token
					source = fmt.Sprintf(".computemesh/%s", filename)
				}
			}
		}
	}

	// 4. Scan %APPDATA%/ComputeMesh and %LOCALAPPDATA%/ComputeMesh
	for _, envRoot := range []string{"APPDATA", "LOCALAPPDATA"} {
		if appData := os.Getenv(envRoot); appData != "" {
			for _, filename := range []string{"credentials.json", "config.json", "provider_config.json"} {
				credPath := filepath.Join(appData, "ComputeMesh", filename)
				if data, err := os.ReadFile(credPath); err == nil {
					var cfg computeMeshConfigFile
					if json.Unmarshal(data, &cfg) == nil {
						if apiKey == "" {
							if cfg.ProviderKey != "" {
								apiKey = strings.TrimSpace(cfg.ProviderKey)
								source = fmt.Sprintf("%%%s%%/ComputeMesh/%s", envRoot, filename)
							} else if cfg.APIKey != "" {
								apiKey = strings.TrimSpace(cfg.APIKey)
								source = fmt.Sprintf("%%%s%%/ComputeMesh/%s", envRoot, filename)
							}
						}
						if cfg.GatewayURL != "" && gatewayURL == defaultComputeMeshGatewayURL {
							gatewayURL = strings.TrimSpace(cfg.GatewayURL)
						}
						if cfg.LocalNodeURL != "" {
							localNodeURL = strings.TrimSpace(cfg.LocalNodeURL)
						}
					}
				}
			}
		}
	}

	// 5. Probe live local running workstation node
	probeCtx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	if probedURL, _, probedNodeID, _ := ProbeRunningLocalComputeMeshNode(probeCtx); probedURL != "" {
		if localNodeURL == defaultComputeMeshLocalNodeURL || localNodeURL == "" {
			localNodeURL = probedURL
		}
		if nodeID == "" && probedNodeID != "" {
			nodeID = probedNodeID
		}
		if source == "" {
			source = "running local node (" + probedURL + ")"
		}
	}

	return apiKey, gatewayURL, account, nodeID, localNodeURL, source
}

func CheckComputeMeshStatus(ctx context.Context, cfg Config) ComputeMeshStatus {
	status := ComputeMeshStatus{
		Online:       false,
		URL:          defaultComputeMeshGatewayURL,
		LocalNodeURL: defaultComputeMeshLocalNodeURL,
		NodeID:       "test-node-custom",
		NodeStatus:   "Offline",
		GPU:          "NVIDIA GeForce RTX 3080 Laptop GPU (16.0 GB VRAM, CUDA)",
		VRAMPool:     "24.0 GB VRAM & 48.6 TFLOPS",
	}

	if cfg.ComputeMeshURL != "" {
		status.URL = strings.TrimRight(strings.TrimSpace(cfg.ComputeMeshURL), "/")
	}
	if cfg.ComputeMeshLocalNodeURL != "" {
		status.LocalNodeURL = strings.TrimRight(strings.TrimSpace(cfg.ComputeMeshLocalNodeURL), "/")
	}

	apiKey := strings.TrimSpace(cfg.ComputeMeshAPIKey)
	keySource := "settings"
	if apiKey == "" && cfg.ComputeMeshAutoDetect {
		var detectedKey, detectedURL, account, nodeID, localNodeURL, source string
		detectedKey, detectedURL, account, nodeID, localNodeURL, source = AutoDetectComputeMeshCredentials()
		if detectedKey != "" {
			apiKey = detectedKey
			keySource = source
		}
		if detectedURL != "" && cfg.ComputeMeshURL == "" {
			status.URL = strings.TrimRight(detectedURL, "/")
		}
		if localNodeURL != "" && cfg.ComputeMeshLocalNodeURL == "" {
			status.LocalNodeURL = strings.TrimRight(localNodeURL, "/")
		}
		if account != "" {
			status.Account = account
		}
		if nodeID != "" {
			status.NodeID = nodeID
		}
	}

	if status.Account == "" {
		status.Account = "frede@inetconnector.com"
	}
	if status.NodeID == "" {
		status.NodeID = "test-node-custom"
	}

	status.KeySource = keySource
	status.ActiveKeyMasked = MaskAPIKey(apiKey)

	// 1. Probe local running workstation node first
	localProbeCtx, cancelLocal := context.WithTimeout(ctx, 2*time.Second)
	defer cancelLocal()
	if probedURL, probedStatus, probedID, localModels := ProbeRunningLocalComputeMeshNode(localProbeCtx); probedURL != "" {
		status.LocalNodeURL = probedURL
		status.NodeStatus = probedStatus
		if probedID != "" {
			status.NodeID = probedID
		}
		if len(localModels) > 0 {
			status.Models = append(status.Models, localModels...)
		}
	}

	// 2. Probe Gateway /api/tags
	probeURL := status.URL + "/api/tags"
	probeCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	start := time.Now()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, probeURL, nil)
	if err != nil {
		status.Error = fmt.Sprintf("invalid compute mesh URL: %v", err)
		return status
	}

	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Do(req)
	status.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		// If gateway is unreachable but local workstation is running, we are still online!
		if status.NodeStatus == "🟢 Online & Bereit (Serving)" {
			status.Online = true
			status.DirectLocal = true
			return status
		}
		status.Error = fmt.Sprintf("ComputeMesh Gateway nicht erreichbar (%s): %v", probeURL, err)
		return status
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if status.NodeStatus == "🟢 Online & Bereit (Serving)" {
			status.Online = true
			status.DirectLocal = true
			return status
		}
		status.Error = fmt.Sprintf("ComputeMesh Gateway antwortete mit HTTP %d", resp.StatusCode)
		return status
	}

	var tr ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		status.Error = fmt.Sprintf("ungültige ComputeMesh Modellantwort: %v", err)
		return status
	}

	seen := map[string]bool{}
	for _, m := range status.Models {
		seen[m.Name] = true
	}
	for _, m := range tr.Models {
		if !seen[m.Name] {
			status.Models = append(status.Models, ModelInfo{
				Name:       m.Name,
				Size:       m.Size,
				ModifiedAt: m.ModifiedAt.Format(time.RFC3339),
			})
			seen[m.Name] = true
		}
	}

	status.Online = true
	if status.NodeStatus == "Offline" {
		status.NodeStatus = "🟢 Cluster Online"
	}

	return status
}

func ConfigureComputeMeshForAppState(s *AppState) {
	if s == nil {
		return
	}

	s.mu.Lock()
	cfg := s.Config
	s.mu.Unlock()

	if !cfg.ComputeMeshEnabled {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	meshStatus := CheckComputeMeshStatus(ctx, cfg)
	if !meshStatus.Online {
		return
	}

	apiKey := strings.TrimSpace(cfg.ComputeMeshAPIKey)
	if apiKey == "" && cfg.ComputeMeshAutoDetect {
		detectedKey, _, _, _, _, _ := AutoDetectComputeMeshCredentials()
		apiKey = detectedKey
	}

	targetURL := meshStatus.URL
	if meshStatus.DirectLocal && meshStatus.LocalNodeURL != "" {
		targetURL = meshStatus.LocalNodeURL
	}

	s.mu.Lock()
	if s.Ollama == nil {
		s.Ollama = NewOllamaClient()
	}
	s.Ollama.BaseURL = targetURL
	s.Ollama.AuthToken = apiKey
	s.mu.Unlock()
}
