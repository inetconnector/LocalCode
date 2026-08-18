// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLSPPoolReusesClientAndSynchronizesChangedDocument(t *testing.T) {
	if os.Getenv("LOCALCODE_LSP_POOL_HELPER") == "1" {
		return
	}
	pool := newLSPClientPool(4)
	defer pool.Close()
	cfg := lspPoolTestConfig()
	project := t.TempDir()
	path := filepath.Join(project, "sample.go")
	spec := lspPoolTestSpec()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	var first *lspClient
	firstResult, err := pool.withDocumentClient(ctx, project, cfg, spec, path, "package sample\n", func(client *lspClient) (string, error) {
		first = client
		return lspPoolTestQuery(ctx, client)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains([]byte(firstResult), []byte(`"opens":1`)) || !bytes.Contains([]byte(firstResult), []byte(`"changes":0`)) {
		t.Fatalf("unexpected first pool result: %s", firstResult)
	}

	secondResult, err := pool.withDocumentClient(ctx, project, cfg, spec, path, "package sample\n", func(client *lspClient) (string, error) {
		if client != first {
			t.Fatal("expected persistent LSP client reuse")
		}
		return lspPoolTestQuery(ctx, client)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains([]byte(secondResult), []byte(`"opens":1`)) || !bytes.Contains([]byte(secondResult), []byte(`"changes":0`)) {
		t.Fatalf("unchanged document must not be reopened or changed: %s", secondResult)
	}

	thirdResult, err := pool.withDocumentClient(ctx, project, cfg, spec, path, "package sample\nvar Changed = true\n", func(client *lspClient) (string, error) {
		if client != first {
			t.Fatal("document change must not restart a healthy server")
		}
		return lspPoolTestQuery(ctx, client)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains([]byte(thirdResult), []byte(`"opens":1`)) || !bytes.Contains([]byte(thirdResult), []byte(`"changes":1`)) || !bytes.Contains([]byte(thirdResult), []byte(`"version":2`)) {
		t.Fatalf("changed document was not synchronized via didChange version 2: %s", thirdResult)
	}
}

func TestLSPPoolRestartsOnceAfterTransportFailure(t *testing.T) {
	if os.Getenv("LOCALCODE_LSP_POOL_HELPER") == "1" {
		return
	}
	pool := newLSPClientPool(4)
	defer pool.Close()
	cfg := lspPoolTestConfig()
	marker := filepath.Join(t.TempDir(), "failed-once.marker")
	cfg.EnvironmentVars["LOCALCODE_LSP_POOL_FAIL_ONCE"] = marker
	project := t.TempDir()
	path := filepath.Join(project, "sample.go")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	starts := 0
	originalStart := pool.start
	pool.start = func(ctx context.Context, project string, cfg Config, spec lspServerSpec) (*lspClient, error) {
		starts++
		return originalStart(ctx, project, cfg, spec)
	}

	result, err := pool.withDocumentClient(ctx, project, cfg, lspPoolTestSpec(), path, "package sample\n", func(client *lspClient) (string, error) {
		return lspPoolTestQuery(ctx, client)
	})
	if err != nil {
		t.Fatal(err)
	}
	if starts != 2 {
		t.Fatalf("expected exactly one restart after transport failure, starts=%d", starts)
	}
	if !bytes.Contains([]byte(result), []byte(`"queries":1`)) {
		t.Fatalf("fresh server did not complete retried request: %s", result)
	}
}

func TestLSPPoolIsBoundedAndSeparatesProjects(t *testing.T) {
	if os.Getenv("LOCALCODE_LSP_POOL_HELPER") == "1" {
		return
	}
	pool := newLSPClientPool(2)
	defer pool.Close()
	cfg := lspPoolTestConfig()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	clients := map[*lspClient]bool{}
	for i := 0; i < 3; i++ {
		project := t.TempDir()
		path := filepath.Join(project, "sample.go")
		if _, err := pool.withDocumentClient(ctx, project, cfg, lspPoolTestSpec(), path, "package sample\n", func(client *lspClient) (string, error) {
			clients[client] = true
			return lspPoolTestQuery(ctx, client)
		}); err != nil {
			t.Fatal(err)
		}
	}
	if len(clients) != 3 {
		t.Fatalf("distinct projects must not share one language-server session, clients=%d", len(clients))
	}
	if pool.size() > 2 {
		t.Fatalf("bounded LSP pool grew beyond limit: %d", pool.size())
	}
}

func lspPoolTestConfig() Config {
	cfg := defaultConfig()
	if cfg.EnvironmentVars == nil {
		cfg.EnvironmentVars = map[string]string{}
	}
	cfg.EnvironmentVars["LOCALCODE_LSP_POOL_HELPER"] = "1"
	return cfg
}

func lspPoolTestSpec() lspServerSpec {
	return lspServerSpec{
		Tool:       "pool-test-lsp",
		Executable: os.Args[0],
		Args:       []string{"-test.run=TestLocalCodeLSPPoolHelperProcess", "--"},
		LanguageID: "go",
	}
}

func lspPoolTestQuery(ctx context.Context, client *lspClient) (string, error) {
	result, err := client.request(ctx, "textDocument/documentSymbol", map[string]any{
		"textDocument": map[string]any{"uri": "file:///sample.go"},
	})
	return string(result), err
}

func TestLocalCodeLSPPoolHelperProcess(t *testing.T) {
	if os.Getenv("LOCALCODE_LSP_POOL_HELPER") != "1" {
		return
	}
	reader := bufio.NewReader(os.Stdin)
	opens := 0
	changes := 0
	queries := 0
	version := 0
	for {
		envelope, err := readLSPEnvelope(reader)
		if err != nil {
			if err == io.EOF {
				os.Exit(0)
			}
			os.Exit(2)
		}
		switch envelope.Method {
		case "initialize":
			writeLSPTestResponse(envelope.ID, map[string]any{"capabilities": map[string]any{
				"textDocumentSync":       1,
				"documentSymbolProvider": true,
			}})
		case "initialized":
		case "textDocument/didOpen":
			opens++
			version = 1
		case "textDocument/didChange":
			changes++
			var params struct {
				TextDocument struct {
					Version int `json:"version"`
				} `json:"textDocument"`
			}
			_ = json.Unmarshal(envelope.Params, &params)
			version = params.TextDocument.Version
		case "textDocument/documentSymbol":
			if marker := os.Getenv("LOCALCODE_LSP_POOL_FAIL_ONCE"); marker != "" {
				if _, statErr := os.Stat(marker); os.IsNotExist(statErr) {
					_ = os.WriteFile(marker, []byte("failed"), 0o600)
					os.Exit(9)
				}
			}
			queries++
			writeLSPTestResponse(envelope.ID, map[string]any{
				"opens":   opens,
				"changes": changes,
				"queries": queries,
				"version": version,
			})
		case "shutdown":
			writeLSPTestResponse(envelope.ID, nil)
		case "exit":
			os.Exit(0)
		default:
			if len(envelope.ID) > 0 {
				var id any
				_ = json.Unmarshal(envelope.ID, &id)
				data, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": nil})
				_, _ = fmt.Fprintf(os.Stdout, "Content-Length: %d\r\n\r\n%s", len(data), data)
			}
		}
	}
}

func TestLSPPoolKeyIncludesProjectExecutableArgumentsAndEnvironment(t *testing.T) {
	base := lspServerSpec{Tool: "gopls", Executable: `C:\\tools\\gopls.exe`, Args: []string{"serve"}, LanguageID: "go"}
	cfg := defaultConfig()
	if cfg.EnvironmentVars == nil {
		cfg.EnvironmentVars = map[string]string{}
	}
	key := lspPoolKey(`C:\\repo-a`, cfg, base)
	if key == lspPoolKey(`C:\\repo-b`, cfg, base) {
		t.Fatal("project must participate in LSP pool key")
	}
	changedExecutable := base
	changedExecutable.Executable = `C:\\other\\gopls.exe`
	if key == lspPoolKey(`C:\\repo-a`, cfg, changedExecutable) {
		t.Fatal("executable must participate in LSP pool key")
	}
	changedArgs := base
	changedArgs.Args = []string{"serve", "-rpc.trace"}
	if key == lspPoolKey(`C:\\repo-a`, cfg, changedArgs) {
		t.Fatal("arguments must participate in LSP pool key")
	}
	changedEnv := cfg
	changedEnv.EnvironmentVars = map[string]string{"GOWORK": "off"}
	if key == lspPoolKey(`C:\\repo-a`, changedEnv, base) {
		t.Fatal("LocalCode environment must participate in LSP pool key")
	}
}
