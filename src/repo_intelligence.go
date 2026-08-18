// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	repoIntelMaxFiles      = 3200
	repoIntelMaxReadable   = 1400
	repoIntelMaxFileBytes  = 512 * 1024
	repoIntelMaxSymbols    = 28
	repoIntelMaxImports    = 10
	repoIntelRelevantFiles = 24
	repoIntelArchitecture  = 14
	repoIntelVerification  = 10
)

type repoIntelFile struct {
	Path       string
	Language   string
	Score      int
	Size       int64
	Symbols    []string
	Imports    []string
	Entrypoint bool
	Test       bool
	Config     bool
}

var repoIntelIgnoredDirs = map[string]bool{
	".git": true, ".hg": true, ".svn": true, ".idea": true, ".vs": true,
	"node_modules": true, "vendor": true, "dist": true, "build": true, "out": true,
	"bin": true, "obj": true, "target": true, ".gradle": true, ".next": true,
	"coverage": true, ".pytest_cache": true, "__pycache__": true, ".localcode": true,
}

var repoIntelCodeExtensions = map[string]string{
	".go": "Go", ".cs": "C#", ".fs": "F#", ".vb": "VB.NET",
	".java": "Java", ".kt": "Kotlin", ".kts": "Kotlin",
	".js": "JavaScript", ".jsx": "JavaScript", ".mjs": "JavaScript", ".cjs": "JavaScript",
	".ts": "TypeScript", ".tsx": "TypeScript", ".py": "Python", ".rs": "Rust",
	".c": "C", ".h": "C/C++", ".cc": "C++", ".cpp": "C++", ".cxx": "C++", ".hpp": "C++",
	".rb": "Ruby", ".php": "PHP", ".swift": "Swift", ".scala": "Scala",
	".html": "HTML", ".htm": "HTML", ".css": "CSS", ".scss": "SCSS", ".vue": "Vue", ".svelte": "Svelte",
	".sql": "SQL", ".proto": "Proto", ".sh": "Shell", ".ps1": "PowerShell", ".bat": "Batch", ".cmd": "Batch",
}

var repoIntelImportantNames = map[string]bool{
	"readme.md": true, "agents.md": true, "state.md": true,
	"go.mod": true, "go.work": true, "package.json": true, "pyproject.toml": true, "requirements.txt": true,
	"cargo.toml": true, "build.gradle": true, "build.gradle.kts": true, "settings.gradle": true, "settings.gradle.kts": true,
	"pom.xml": true, "cmakelists.txt": true, "makefile": true, "dockerfile": true, "compose.yml": true, "compose.yaml": true,
	"global.json": true, "directory.build.props": true, "directory.build.targets": true,
}

var repoIntelSymbolPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^\s*func\s+(?:\([^)]*\)\s*)?([A-Za-z_][A-Za-z0-9_]*)`),
	regexp.MustCompile(`^\s*type\s+([A-Za-z_][A-Za-z0-9_]*)\s+(?:struct|interface|func|map|\[)`),
	regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s+([A-Za-z_$][A-Za-z0-9_$]*)`),
	regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?class\s+([A-Za-z_$][A-Za-z0-9_$]*)`),
	regexp.MustCompile(`^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*(?:async\s*)?(?:\([^)]*\)|[A-Za-z_$][A-Za-z0-9_$]*)\s*=>`),
	regexp.MustCompile(`^\s*(?:async\s+)?def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
	regexp.MustCompile(`^\s*class\s+([A-Za-z_][A-Za-z0-9_]*)\b`),
	regexp.MustCompile(`^\s*(?:public|private|protected|internal|static|sealed|abstract|partial|final|open|data|internal|suspend|override|virtual|async|unsafe|extern|new|readonly|ref|record|\s)+\s*(?:class|interface|record|enum|struct)\s+([A-Za-z_][A-Za-z0-9_]*)\b`),
	regexp.MustCompile(`^\s*(?:public|private|protected|internal|static|final|open|suspend|override|async|virtual|abstract|extern|unsafe|\s)+\s*[A-Za-z_<>,?\[\].:]+\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
	regexp.MustCompile(`^\s*(?:pub\s+)?(?:async\s+)?fn\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
	regexp.MustCompile(`^\s*(?:pub\s+)?(?:struct|enum|trait)\s+([A-Za-z_][A-Za-z0-9_]*)\b`),
	regexp.MustCompile(`^\s*fun\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
}

func repositoryIntelligence(project, task string) (string, error) {
	project = filepath.Clean(project)
	terms := repoIntelTerms(task)
	files := make([]repoIntelFile, 0, 256)
	fileCount := 0
	readCount := 0
	walkErr := filepath.WalkDir(project, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == project {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if repoIntelIgnoredDirs[strings.ToLower(name)] || strings.HasPrefix(name, ".git") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		fileCount++
		if fileCount > repoIntelMaxFiles {
			return filepath.SkipAll
		}
		rel, relErr := filepath.Rel(project, path)
		if relErr != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(name))
		language, code := repoIntelCodeExtensions[ext]
		important := repoIntelImportantNames[strings.ToLower(name)] || repoIntelLooksBuildFile(name)
		if !code && !important {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil || info.Size() > repoIntelMaxFileBytes {
			return nil
		}
		item := repoIntelFile{
			Path:       filepath.ToSlash(rel),
			Language:   language,
			Size:       info.Size(),
			Entrypoint: repoIntelIsEntrypoint(rel),
			Test:       repoIntelIsTestFile(rel),
			Config:     important,
		}
		if item.Language == "" {
			item.Language = "Config"
		}
		item.Score = repoIntelPathScore(item, terms)
		if readCount < repoIntelMaxReadable {
			content, readErr := repoIntelReadLimited(path, repoIntelMaxFileBytes)
			if readErr == nil {
				readCount++
				item.Score += repoIntelContentScore(content, terms)
				item.Symbols = repoIntelExtractSymbols(content)
				item.Imports = repoIntelExtractImports(content)
			}
		}
		files = append(files, item)
		return nil
	})
	if walkErr != nil {
		return "", walkErr
	}
	if len(files) == 0 {
		return "REPOSITORY INTELLIGENCE MAP\nNo supported source or build files were found.\n", nil
	}

	relevant := append([]repoIntelFile(nil), files...)
	sort.SliceStable(relevant, func(i, j int) bool {
		if relevant[i].Score != relevant[j].Score {
			return relevant[i].Score > relevant[j].Score
		}
		return relevant[i].Path < relevant[j].Path
	})
	architecture := append([]repoIntelFile(nil), files...)
	sort.SliceStable(architecture, func(i, j int) bool {
		ai := repoIntelArchitectureScore(architecture[i])
		aj := repoIntelArchitectureScore(architecture[j])
		if ai != aj {
			return ai > aj
		}
		return architecture[i].Path < architecture[j].Path
	})

	var b strings.Builder
	b.WriteString("REPOSITORY INTELLIGENCE MAP\n")
	fmt.Fprintf(&b, "Scanned: %d project files; analyzed: %d relevant source/build files; task terms: %s\n", fileCount, len(files), strings.Join(terms, ", "))
	b.WriteString("Purpose: deterministic context selection before edits; paths are evidence, not permission to mutate.\n\n")

	b.WriteString("ARCHITECTURE ANCHORS\n")
	for _, item := range repoIntelLimitArchitecture(architecture) {
		b.WriteString(repoIntelFormatFile(item))
	}

	b.WriteString("\nTASK-RELEVANT FILES\n")
	for _, item := range repoIntelLimitRelevant(relevant) {
		b.WriteString(repoIntelFormatFile(item))
	}

	paired := repoIntelPairedTests(relevant, files)
	b.WriteString("\nLIKELY TEST / CROSS-CHECK TARGETS\n")
	if len(paired) == 0 {
		b.WriteString("- No obvious paired tests detected; use build-system verification below.\n")
	} else {
		for _, path := range paired {
			b.WriteString("- " + path + "\n")
		}
	}

	b.WriteString("\nVERIFICATION PLAN\n")
	commands := repoIntelVerificationCommands(project, files)
	if len(commands) == 0 {
		b.WriteString("- No deterministic build/test command identified. Verify changed files with the narrowest project-supported check and inspect the diff.\n")
	} else {
		for _, command := range commands {
			b.WriteString("- " + command + "\n")
		}
	}
	b.WriteString("\nRELIABILITY INVARIANTS\n")
	b.WriteString("- Read the highest-ranked implementation and test files before editing.\n")
	b.WriteString("- Preserve public APIs and project conventions unless the task explicitly requires a breaking change.\n")
	b.WriteString("- Prefer coherent symbol-level edits over scattered line edits.\n")
	b.WriteString("- After edits, inspect changed files and run the smallest relevant checks first, then broader tests/builds.\n")
	b.WriteString("- A successful process exit without an observable intended change is not sufficient proof of task completion.\n")
	return truncateText(b.String(), 42000), nil
}

func repoIntelTerms(task string) []string {
	seen := map[string]bool{}
	terms := make([]string, 0, 12)
	for _, word := range significantWords(task) {
		word = strings.ToLower(strings.TrimSpace(word))
		if len(word) < 4 || seen[word] {
			continue
		}
		seen[word] = true
		terms = append(terms, word)
		if len(terms) >= 12 {
			break
		}
	}
	return terms
}

func repoIntelReadLimited(path string, maxBytes int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > maxBytes {
		data = data[:maxBytes]
	}
	if strings.IndexByte(string(data), 0) >= 0 {
		return "", fmt.Errorf("binary file")
	}
	return string(data), nil
}

func repoIntelPathScore(item repoIntelFile, terms []string) int {
	path := strings.ToLower(item.Path)
	base := strings.ToLower(filepath.Base(item.Path))
	score := 0
	for _, term := range terms {
		if strings.Contains(base, term) {
			score += 14
		} else if strings.Contains(path, term) {
			score += 7
		}
	}
	if item.Entrypoint {
		score += 5
	}
	if item.Config {
		score += 3
	}
	if item.Test {
		score += 2
	}
	return score
}

func repoIntelContentScore(content string, terms []string) int {
	lower := strings.ToLower(content)
	score := 0
	for _, term := range terms {
		count := strings.Count(lower, term)
		if count > 5 {
			count = 5
		}
		score += count * 3
	}
	return score
}

func repoIntelExtractSymbols(content string) []string {
	seen := map[string]bool{}
	var symbols []string
	lines := strings.Split(content, "\n")
	if len(lines) > 2600 {
		lines = lines[:2600]
	}
	for _, line := range lines {
		if len(line) > 500 {
			continue
		}
		for _, pattern := range repoIntelSymbolPatterns {
			match := pattern.FindStringSubmatch(line)
			if len(match) < 2 {
				continue
			}
			name := strings.TrimSpace(match[1])
			if name == "" || seen[name] {
				break
			}
			seen[name] = true
			symbols = append(symbols, name)
			break
		}
		if len(symbols) >= repoIntelMaxSymbols {
			break
		}
	}
	return symbols
}

func repoIntelExtractImports(content string) []string {
	seen := map[string]bool{}
	var imports []string
	lines := strings.Split(content, "\n")
	if len(lines) > 400 {
		lines = lines[:400]
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if !(strings.HasPrefix(lower, "import ") || strings.HasPrefix(lower, "from ") || strings.HasPrefix(lower, "using ") || strings.HasPrefix(lower, "require(") || strings.HasPrefix(lower, "use ")) {
			continue
		}
		if len(trimmed) > 180 {
			trimmed = trimmed[:180] + "…"
		}
		if !seen[trimmed] {
			seen[trimmed] = true
			imports = append(imports, trimmed)
		}
		if len(imports) >= repoIntelMaxImports {
			break
		}
	}
	return imports
}

func repoIntelLooksBuildFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".sln") || strings.HasSuffix(lower, ".csproj") || strings.HasSuffix(lower, ".fsproj") || strings.HasSuffix(lower, ".vbproj")
}

func repoIntelIsEntrypoint(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case "main.go", "main.py", "app.py", "program.cs", "startup.cs", "main.rs", "main.kt", "main.java", "index.js", "index.ts", "server.js", "server.ts", "app.js", "app.ts", "package.json":
		return true
	default:
		return false
	}
}

func repoIntelIsTestFile(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(path))
	base := strings.ToLower(filepath.Base(path))
	return strings.Contains(lower, "/test/") || strings.Contains(lower, "/tests/") || strings.Contains(lower, "/__tests__/") ||
		strings.Contains(base, "_test.") || strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") ||
		strings.HasSuffix(base, "tests.cs") || strings.HasSuffix(base, "test.cs")
}

func repoIntelArchitectureScore(item repoIntelFile) int {
	score := 0
	if item.Entrypoint {
		score += 20
	}
	if item.Config {
		score += 12
	}
	if len(item.Symbols) > 0 {
		score += min(10, len(item.Symbols))
	}
	if item.Test {
		score -= 4
	}
	depth := strings.Count(filepath.ToSlash(item.Path), "/")
	score -= min(5, depth)
	return score
}

func repoIntelLimitArchitecture(items []repoIntelFile) []repoIntelFile {
	out := make([]repoIntelFile, 0, repoIntelArchitecture)
	for _, item := range items {
		if len(out) >= repoIntelArchitecture {
			break
		}
		if repoIntelArchitectureScore(item) <= 0 && len(out) > 5 {
			continue
		}
		out = append(out, item)
	}
	return out
}

func repoIntelLimitRelevant(items []repoIntelFile) []repoIntelFile {
	if len(items) <= repoIntelRelevantFiles {
		return items
	}
	return items[:repoIntelRelevantFiles]
}

func repoIntelFormatFile(item repoIntelFile) string {
	flags := make([]string, 0, 3)
	if item.Entrypoint {
		flags = append(flags, "entry")
	}
	if item.Test {
		flags = append(flags, "test")
	}
	if item.Config {
		flags = append(flags, "build/config")
	}
	flagText := ""
	if len(flags) > 0 {
		flagText = " [" + strings.Join(flags, ",") + "]"
	}
	line := fmt.Sprintf("- %s (%s, score=%d)%s", item.Path, item.Language, item.Score, flagText)
	if len(item.Symbols) > 0 {
		line += " symbols: " + strings.Join(item.Symbols, ", ")
	}
	if len(line) > 720 {
		line = line[:720] + "…"
	}
	return line + "\n"
}

func repoIntelPairedTests(relevant, all []repoIntelFile) []string {
	tests := make([]repoIntelFile, 0)
	for _, item := range all {
		if item.Test {
			tests = append(tests, item)
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, source := range relevant {
		if source.Test {
			if !seen[source.Path] {
				seen[source.Path] = true
				out = append(out, source.Path)
			}
			continue
		}
		stem := strings.ToLower(strings.TrimSuffix(filepath.Base(source.Path), filepath.Ext(source.Path)))
		if len(stem) < 3 {
			continue
		}
		for _, test := range tests {
			if strings.Contains(strings.ToLower(test.Path), stem) && !seen[test.Path] {
				seen[test.Path] = true
				out = append(out, test.Path)
				if len(out) >= repoIntelVerification {
					return out
				}
			}
		}
		if len(out) >= repoIntelVerification {
			break
		}
	}
	return out
}

func repoIntelVerificationCommands(project string, files []repoIntelFile) []string {
	has := func(name string) bool {
		_, err := os.Stat(filepath.Join(project, name))
		return err == nil
	}
	seen := map[string]bool{}
	var commands []string
	add := func(command string) {
		if command == "" || seen[command] || len(commands) >= repoIntelVerification {
			return
		}
		seen[command] = true
		commands = append(commands, command)
	}
	if has("go.mod") || has("go.work") {
		add("go test ./...")
		add("go vet ./...")
	}
	if has("package.json") {
		for _, command := range repoIntelPackageJSONCommands(filepath.Join(project, "package.json")) {
			add(command)
		}
	}
	if has("pyproject.toml") || has("pytest.ini") || has("requirements.txt") {
		add("python -m pytest")
	}
	if has("Cargo.toml") {
		add("cargo test")
		add("cargo clippy --all-targets --all-features")
	}
	if has("gradlew") {
		add("./gradlew test")
	} else if has("gradlew.bat") {
		add("gradlew.bat test")
	} else if has("pom.xml") {
		add("mvn test")
	}
	var hasDotnet bool
	for _, item := range files {
		lower := strings.ToLower(item.Path)
		if strings.HasSuffix(lower, ".sln") || strings.HasSuffix(lower, ".csproj") || strings.HasSuffix(lower, ".fsproj") || strings.HasSuffix(lower, ".vbproj") {
			hasDotnet = true
			break
		}
	}
	if hasDotnet {
		add("dotnet build")
		add("dotnet test --no-build")
	}
	return commands
}

func repoIntelPackageJSONCommands(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return nil
	}
	var out []string
	for _, name := range []string{"lint", "test", "typecheck", "check", "build"} {
		if strings.TrimSpace(pkg.Scripts[name]) != "" {
			out = append(out, "npm run "+name)
		}
	}
	return out
}
