// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type patchOpKind string

const (
	patchOpAdd    patchOpKind = "add"
	patchOpUpdate patchOpKind = "update"
	patchOpDelete patchOpKind = "delete"
)

type patchLineOp byte

const (
	patchLineContext patchLineOp = ' '
	patchLineAdd     patchLineOp = '+'
	patchLineRemove  patchLineOp = '-'
)

type patchLine struct {
	Op   patchLineOp
	Text string
}

type patchHunk struct {
	Header   string
	OldStart int // 1-based line hint if available (0 = unspecified)
	OldCount int
	NewStart int
	NewCount int
	Lines    []patchLine
	EOF      bool
}

type patchFileOp struct {
	Kind    patchOpKind
	Path    string
	MoveTo  string
	AddBody string // raw lines for Add File
	Hunks   []patchHunk
}

var unifiedHunkHeaderRe = regexp.MustCompile(`^@@\s*-(\d+)(?:,(\d+))?\s+\+(\d+)(?:,(\d+))?\s*@@(.*)$`)

// applyPatchBeforeCommitOpHook is an unexported test hook invoked right before
// committing operation index i (nil in production).
var applyPatchBeforeCommitOpHook func(opIndex int, relPath string) error

// createPreEditBackup preserves a file in LocalCode's managed backup cache
// prior to mutating or deleting it.
func createPreEditBackup(projectRoot, fullPath string) error {
	return backupFile(projectRoot, fullPath)
}

func normalizePatchNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

func cleanUnifiedDiffPath(raw string) string {
	p := strings.TrimSpace(raw)
	if idx := strings.IndexByte(p, '\t'); idx >= 0 {
		p = strings.TrimSpace(p[:idx])
	}
	if len(p) >= 2 && strings.HasPrefix(p, `"`) && strings.HasSuffix(p, `"`) {
		if unq, err := strconv.Unquote(p); err == nil {
			p = unq
		}
	}
	if p == "/dev/null" || p == "dev/null" {
		return "/dev/null"
	}
	if strings.HasPrefix(p, "a/") || strings.HasPrefix(p, "b/") {
		p = p[2:]
	}
	return strings.TrimSpace(p)
}

// parseApplyPatch parses either the Codex structured patch format
// (`*** Begin Patch` ... `*** End Patch`) or a standard Unified Diff
// (`--- a/...` / `+++ b/...` / `@@ -a,b +c,d @@`).
func parseApplyPatch(raw string) ([]patchFileOp, error) {
	text := normalizePatchNewlines(stripOuterMarkdownFence(raw))
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("patch is empty")
	}
	if strings.Contains(text, "*** Begin Patch") ||
		strings.Contains(text, "*** Update File:") ||
		strings.Contains(text, "*** Add File:") ||
		strings.Contains(text, "*** Delete File:") {
		return parseStructuredPatch(text)
	}
	return parseUnifiedDiffPatch(text)
}

func parseStructuredPatch(text string) ([]patchFileOp, error) {
	lines := strings.Split(text, "\n")
	start := 0
	for i, line := range lines {
		if strings.TrimSpace(line) == "*** Begin Patch" {
			start = i + 1
			break
		}
	}

	var ops []patchFileOp
	i := start
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "*** End Patch" {
			break
		}
		if trimmed == "" {
			i++
			continue
		}
		switch {
		case strings.HasPrefix(line, "*** Add File:"):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Add File:"))
			if path == "" {
				return nil, fmt.Errorf("line %d: Add File missing path", i+1)
			}
			i++
			var bodyLines []string
			for i < len(lines) {
				cur := lines[i]
				if isStructuredSectionHeader(cur) {
					break
				}
				if strings.HasPrefix(cur, "+") {
					bodyLines = append(bodyLines, cur[1:])
				} else if cur == "" && (i+1 >= len(lines) || isStructuredSectionHeader(lines[i+1])) {
					i++
					break
				} else {
					bodyLines = append(bodyLines, cur)
				}
				i++
			}
			body := strings.Join(bodyLines, "\n")
			if len(bodyLines) > 0 {
				body += "\n"
			}
			ops = append(ops, patchFileOp{
				Kind:    patchOpAdd,
				Path:    path,
				AddBody: body,
			})

		case strings.HasPrefix(line, "*** Delete File:"):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Delete File:"))
			if path == "" {
				return nil, fmt.Errorf("line %d: Delete File missing path", i+1)
			}
			ops = append(ops, patchFileOp{
				Kind: patchOpDelete,
				Path: path,
			})
			i++

		case strings.HasPrefix(line, "*** Update File:"):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Update File:"))
			if path == "" {
				return nil, fmt.Errorf("line %d: Update File missing path", i+1)
			}
			op := patchFileOp{
				Kind: patchOpUpdate,
				Path: path,
			}
			i++
			if i < len(lines) && strings.HasPrefix(lines[i], "*** Move to:") {
				op.MoveTo = strings.TrimSpace(strings.TrimPrefix(lines[i], "*** Move to:"))
				i++
			}
			var current *patchHunk
			flushHunk := func() {
				if current != nil && len(current.Lines) > 0 {
					op.Hunks = append(op.Hunks, *current)
				}
				current = nil
			}
			for i < len(lines) {
				cur := lines[i]
				if isStructuredSectionHeader(cur) {
					break
				}
				if strings.TrimSpace(cur) == "*** End of File" {
					if current != nil {
						current.EOF = true
					}
					i++
					continue
				}
				if strings.HasPrefix(cur, "@@") {
					flushHunk()
					h := parseHunkHeaderLine(cur)
					current = &h
					i++
					continue
				}
				if strings.HasPrefix(cur, `\ No newline at end of file`) {
					i++
					continue
				}
				if cur == "" {
					// Trailing blank line right before next section header or end of patch
					if i+1 >= len(lines) || isStructuredSectionHeader(lines[i+1]) {
						i++
						continue
					}
					if current == nil {
						current = &patchHunk{}
					}
					current.Lines = append(current.Lines, patchLine{Op: patchLineContext, Text: ""})
					i++
					continue
				}
				prefix := cur[0]
				if prefix == ' ' || prefix == '+' || prefix == '-' {
					if current == nil {
						current = &patchHunk{}
					}
					current.Lines = append(current.Lines, patchLine{Op: patchLineOp(prefix), Text: cur[1:]})
					i++
					continue
				}
				return nil, fmt.Errorf("line %d: unexpected patch line %q in Update File %s", i+1, truncateText(cur, 80), path)
			}
			flushHunk()
			if len(op.Hunks) == 0 && op.MoveTo == "" {
				return nil, fmt.Errorf("Update File %s contains no hunks", path)
			}
			ops = append(ops, op)

		default:
			return nil, fmt.Errorf("line %d: unexpected directive %q", i+1, truncateText(trimmed, 80))
		}
	}

	if len(ops) == 0 {
		return nil, errors.New("patch contains no file operations")
	}
	return ops, nil
}

func isStructuredSectionHeader(line string) bool {
	return strings.HasPrefix(line, "*** Update File:") ||
		strings.HasPrefix(line, "*** Add File:") ||
		strings.HasPrefix(line, "*** Delete File:") ||
		strings.TrimSpace(line) == "*** End Patch"
}

func parseHunkHeaderLine(line string) patchHunk {
	if m := unifiedHunkHeaderRe.FindStringSubmatch(line); len(m) >= 6 {
		oldStart, _ := strconv.Atoi(m[1])
		oldCount := 1
		if m[2] != "" {
			oldCount, _ = strconv.Atoi(m[2])
		}
		newStart, _ := strconv.Atoi(m[3])
		newCount := 1
		if m[4] != "" {
			newCount, _ = strconv.Atoi(m[4])
		}
		return patchHunk{
			Header:   strings.TrimSpace(m[5]),
			OldStart: oldStart,
			OldCount: oldCount,
			NewStart: newStart,
			NewCount: newCount,
		}
	}
	hdr := strings.TrimSpace(strings.TrimPrefix(line, "@@"))
	hdr = strings.TrimSpace(strings.TrimSuffix(hdr, "@@"))
	return patchHunk{Header: hdr}
}

func parseUnifiedDiffPatch(text string) ([]patchFileOp, error) {
	lines := strings.Split(text, "\n")
	var ops []patchFileOp
	i := 0
	for i < len(lines) {
		line := lines[i]
		// Skip git preamble headers until --- / +++ pair
		if strings.HasPrefix(line, "diff --git ") ||
			strings.HasPrefix(line, "index ") ||
			strings.HasPrefix(line, "new file mode ") ||
			strings.HasPrefix(line, "deleted file mode ") ||
			strings.HasPrefix(line, "old mode ") ||
			strings.HasPrefix(line, "new mode ") ||
			strings.TrimSpace(line) == "" {
			i++
			continue
		}
		if !strings.HasPrefix(line, "--- ") {
			return nil, fmt.Errorf("line %d: expected unified diff header '--- ', got %q", i+1, truncateText(line, 80))
		}
		oldPath := cleanUnifiedDiffPath(strings.TrimPrefix(line, "--- "))
		i++
		if i >= len(lines) || !strings.HasPrefix(lines[i], "+++ ") {
			return nil, fmt.Errorf("line %d: expected unified diff header '+++ ' after '--- %s'", i+1, oldPath)
		}
		newPath := cleanUnifiedDiffPath(strings.TrimPrefix(lines[i], "+++ "))
		i++

		var hunks []patchHunk
		var current *patchHunk
		flushHunk := func() {
			if current != nil && len(current.Lines) > 0 {
				hunks = append(hunks, *current)
			}
			current = nil
		}
		for i < len(lines) {
			cur := lines[i]
			if strings.HasPrefix(cur, "diff --git ") || strings.HasPrefix(cur, "--- ") {
				break
			}
			if strings.HasPrefix(cur, "@@") {
				flushHunk()
				h := parseHunkHeaderLine(cur)
				current = &h
				i++
				continue
			}
			if strings.HasPrefix(cur, `\ No newline at end of file`) {
				i++
				continue
			}
			if cur == "" {
				if current == nil || i+1 >= len(lines) || strings.HasPrefix(lines[i+1], "diff --git ") || strings.HasPrefix(lines[i+1], "--- ") {
					i++
					continue
				}
				current.Lines = append(current.Lines, patchLine{Op: patchLineContext, Text: ""})
				i++
				continue
			}
			prefix := cur[0]
			if prefix == ' ' || prefix == '+' || prefix == '-' {
				if current == nil {
					current = &patchHunk{}
				}
				current.Lines = append(current.Lines, patchLine{Op: patchLineOp(prefix), Text: cur[1:]})
				i++
				continue
			}
			return nil, fmt.Errorf("line %d: unexpected unified diff line %q", i+1, truncateText(cur, 80))
		}
		flushHunk()

		switch {
		case oldPath == "/dev/null" && newPath != "/dev/null":
			var added []string
			for _, h := range hunks {
				for _, l := range h.Lines {
					if l.Op == patchLineAdd || l.Op == patchLineContext {
						added = append(added, l.Text)
					}
				}
			}
			body := strings.Join(added, "\n")
			if len(added) > 0 {
				body += "\n"
			}
			ops = append(ops, patchFileOp{
				Kind:    patchOpAdd,
				Path:    newPath,
				AddBody: body,
			})
		case newPath == "/dev/null" && oldPath != "/dev/null":
			ops = append(ops, patchFileOp{
				Kind:  patchOpDelete,
				Path:  oldPath,
				Hunks: hunks,
			})
		default:
			if oldPath == "" || newPath == "" || oldPath == "/dev/null" {
				return nil, errors.New("invalid unified diff file header paths")
			}
			op := patchFileOp{
				Kind:  patchOpUpdate,
				Path:  oldPath,
				Hunks: hunks,
			}
			if !strings.EqualFold(filepath.ToSlash(oldPath), filepath.ToSlash(newPath)) {
				op.MoveTo = newPath
			}
			if len(op.Hunks) == 0 && op.MoveTo == "" {
				return nil, fmt.Errorf("unified diff for %s contains no hunks", oldPath)
			}
			ops = append(ops, op)
		}
	}

	if len(ops) == 0 {
		return nil, errors.New("unified diff contains no file operations")
	}
	return ops, nil
}

// extractApplyPatchPaths returns all distinct target paths referenced in a patch.
func extractApplyPatchPaths(patchText string) []string {
	ops, err := parseApplyPatch(patchText)
	if err != nil {
		return nil
	}
	seen := make(map[string]bool, len(ops))
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	for _, op := range ops {
		add(op.Path)
		if op.MoveTo != "" {
			add(op.MoveTo)
		}
	}
	return out
}

func applyPatchHasDeletions(patchText string) bool {
	ops, err := parseApplyPatch(patchText)
	if err != nil {
		return false
	}
	for _, op := range ops {
		if op.Kind == patchOpDelete {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Hunk application with exact + whitespace-tolerant fuzzy matching
// ---------------------------------------------------------------------------

func splitFileLinesPreserve(content string) (lines []string, lineEnding string, hadTrailingNL bool) {
	lineEnding = "\n"
	if strings.Contains(content, "\r\n") {
		lineEnding = "\r\n"
	}
	norm := normalizePatchNewlines(content)
	if norm == "" {
		return []string{}, lineEnding, false
	}
	if strings.HasSuffix(norm, "\n") {
		hadTrailingNL = true
		norm = strings.TrimSuffix(norm, "\n")
	}
	if norm == "" {
		return []string{""}, lineEnding, hadTrailingNL
	}
	return strings.Split(norm, "\n"), lineEnding, hadTrailingNL
}

func joinFileLinesPreserve(lines []string, lineEnding string, hadTrailingNL bool) string {
	if len(lines) == 0 {
		return ""
	}
	joined := strings.Join(lines, lineEnding)
	if hadTrailingNL {
		joined += lineEnding
	}
	return joined
}

func normalizePatchLineWS(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

type lineComparator func(fileLine, patchLine string) bool

var patchMatchPasses = []struct {
	name string
	eq   lineComparator
}{
	{
		name: "exact",
		eq: func(a, b string) bool {
			return a == b
		},
	},
	{
		name: "trim_right",
		eq: func(a, b string) bool {
			return strings.TrimRight(a, " \t") == strings.TrimRight(b, " \t")
		},
	},
	{
		name: "fuzzy_whitespace",
		eq: func(a, b string) bool {
			return normalizePatchLineWS(a) == normalizePatchLineWS(b)
		},
	},
}

func findHunkPosition(fileLines []string, oldPattern []string, searchStart int, hintLine int, anchor string, eof bool) (int, error) {
	if len(oldPattern) == 0 {
		// Pure insertion hunk
		if anchor != "" {
			for _, pass := range patchMatchPasses {
				for idx := searchStart; idx < len(fileLines); idx++ {
					if pass.eq(fileLines[idx], anchor) || strings.Contains(normalizePatchLineWS(fileLines[idx]), normalizePatchLineWS(anchor)) {
						return idx + 1, nil
					}
				}
			}
		}
		if hintLine > 0 {
			pos := hintLine - 1
			if pos < 0 {
				pos = 0
			}
			if pos > len(fileLines) {
				pos = len(fileLines)
			}
			return pos, nil
		}
		return len(fileLines), nil
	}

	if len(oldPattern) > len(fileLines) {
		return -1, fmt.Errorf("hunk has %d lines but file only has %d lines", len(oldPattern), len(fileLines))
	}

	// Narrow search floor if anchor text is provided in @@ header.
	anchorFloor := searchStart
	if anchor != "" {
		normAnchor := normalizePatchLineWS(anchor)
		for idx := searchStart; idx < len(fileLines); idx++ {
			if strings.Contains(normalizePatchLineWS(fileLines[idx]), normAnchor) {
				anchorFloor = idx
				break
			}
		}
	}

	maxStart := len(fileLines) - len(oldPattern)

	findWithPass := func(eq lineComparator, floor int) []int {
		var matches []int
		if floor < 0 {
			floor = 0
		}
		for pos := floor; pos <= maxStart; pos++ {
			matched := true
			for j := 0; j < len(oldPattern); j++ {
				if !eq(fileLines[pos+j], oldPattern[j]) {
					matched = false
					break
				}
			}
			if matched {
				matches = append(matches, pos)
			}
		}
		return matches
	}

	pickBest := func(matches []int) int {
		if len(matches) == 1 {
			return matches[0]
		}
		if eof {
			return matches[len(matches)-1]
		}
		if hintLine > 0 {
			target := hintLine - 1
			best := matches[0]
			bestDist := absInt(best - target)
			for _, m := range matches[1:] {
				if d := absInt(m - target); d < bestDist {
					best = m
					bestDist = d
				}
			}
			return best
		}
		return matches[0]
	}

	for _, pass := range patchMatchPasses {
		// Try starting from anchorFloor first, then searchStart, then whole file.
		for _, floor := range []int{anchorFloor, searchStart, 0} {
			if matches := findWithPass(pass.eq, floor); len(matches) > 0 {
				return pickBest(matches), nil
			}
		}
	}

	preview := oldPattern[0]
	if len(oldPattern) > 1 {
		preview += " / " + oldPattern[1]
	}
	return -1, fmt.Errorf("could not locate hunk context starting near %q", truncateText(preview, 100))
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func applyHunksToContent(original string, hunks []patchHunk) (string, error) {
	lines, lineEnding, hadTrailingNL := splitFileLinesPreserve(original)
	searchStart := 0

	for hi, h := range hunks {
		var oldPattern []string
		var newSegment []string
		for _, l := range h.Lines {
			switch l.Op {
			case patchLineContext:
				oldPattern = append(oldPattern, l.Text)
				newSegment = append(newSegment, l.Text)
			case patchLineRemove:
				oldPattern = append(oldPattern, l.Text)
			case patchLineAdd:
				newSegment = append(newSegment, l.Text)
			}
		}
		pos, err := findHunkPosition(lines, oldPattern, searchStart, h.OldStart, h.Header, h.EOF)
		if err != nil {
			return "", fmt.Errorf("hunk #%d: %w", hi+1, err)
		}
		updated := make([]string, 0, len(lines)-len(oldPattern)+len(newSegment))
		updated = append(updated, lines[:pos]...)
		updated = append(updated, newSegment...)
		updated = append(updated, lines[pos+len(oldPattern):]...)
		lines = updated
		searchStart = pos + len(newSegment)
	}

	if original == "" && len(lines) > 0 {
		hadTrailingNL = true
	}
	return joinFileLinesPreserve(lines, lineEnding, hadTrailingNL), nil
}

// ---------------------------------------------------------------------------
// Transactional Multi-File Planner, Preview, Commit & Atomic Rollback
// ---------------------------------------------------------------------------

type plannedFileMutation struct {
	opIndex         int
	kind            patchOpKind
	relPath         string
	fullPath        string
	moveToRel       string
	moveToFull      string
	expectedVersion fileVersion
	moveDestVersion fileVersion
	originalExists  bool
	originalBytes   []byte
	originalMode    os.FileMode
	moveDestExists  bool
	moveDestBytes   []byte
	moveDestMode    os.FileMode
	newBytes        []byte
	diffPreview     string
}

type committedFileSnapshot struct {
	kind           patchOpKind
	fullPath       string
	originalExists bool
	originalBytes  []byte
	originalMode   os.FileMode
	moved          bool
	moveToFull     string
	moveDestExists bool
	moveDestBytes  []byte
	moveDestMode   os.FileMode
}

func planApplyPatch(projectRoot string, cfg Config, patchText string) ([]plannedFileMutation, []string, error) {
	ops, err := parseApplyPatch(patchText)
	if err != nil {
		return nil, nil, err
	}

	// Step 1: Validate every path with strict ensureWithinRoot containment BEFORE reading/mutating.
	lockSet := make(map[string]string) // lockKey -> fullPath
	resolvedPaths := make([]struct {
		full     string
		moveFull string
	}, len(ops))

	for i, op := range ops {
		if strings.TrimSpace(op.Path) == "" {
			return nil, nil, fmt.Errorf("patch operation #%d has empty path", i+1)
		}
		full, err := ensureWithinRoot(projectRoot, op.Path)
		if err != nil {
			return nil, nil, fmt.Errorf("patch path rejected (%s): %w", op.Path, err)
		}
		resolvedPaths[i].full = full
		lockSet[editLockKey(full)] = full
		if op.MoveTo != "" {
			moveFull, err := ensureWithinRoot(projectRoot, op.MoveTo)
			if err != nil {
				return nil, nil, fmt.Errorf("patch move destination rejected (%s): %w", op.MoveTo, err)
			}
			resolvedPaths[i].moveFull = moveFull
			lockSet[editLockKey(moveFull)] = moveFull
		}
	}

	// Collect lock paths in deterministic sorted order to prevent deadlocks.
	lockKeys := make([]string, 0, len(lockSet))
	for k := range lockSet {
		lockKeys = append(lockKeys, k)
	}
	sort.Strings(lockKeys)
	lockPaths := make([]string, 0, len(lockKeys))
	for _, k := range lockKeys {
		lockPaths = append(lockPaths, lockSet[k])
	}

	// Step 2: Prepare all file mutations in memory (supports chained ops on the same file).
	plans := make([]plannedFileMutation, 0, len(ops))
	virtualContent := make(map[string]string)
	virtualExists := make(map[string]bool)

	for i, op := range ops {
		full := resolvedPaths[i].full
		key := editLockKey(full)

		plan := plannedFileMutation{
			opIndex:    i,
			kind:       op.Kind,
			relPath:    filepath.ToSlash(op.Path),
			fullPath:   full,
			moveToRel:  filepath.ToSlash(op.MoveTo),
			moveToFull: resolvedPaths[i].moveFull,
		}

		// Capture real on-disk state.
		var diskBytes []byte
		var diskExists bool
		diskMode := os.FileMode(0o644)
		if info, statErr := os.Stat(full); statErr == nil {
			if info.IsDir() {
				return nil, nil, fmt.Errorf("patch target is a directory: %s", op.Path)
			}
			diskExists = true
			diskMode = info.Mode().Perm()
			diskBytes, err = os.ReadFile(full)
			if err != nil {
				return nil, nil, fmt.Errorf("read %s: %w", op.Path, err)
			}
			plan.expectedVersion = versionForBytes(diskBytes)
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return nil, nil, fmt.Errorf("stat %s: %w", op.Path, statErr)
		}
		plan.originalExists = diskExists
		plan.originalBytes = diskBytes
		plan.originalMode = diskMode

		// Determine effective working content if an earlier op in the same patch touched this path.
		curExists := diskExists
		curContent := string(diskBytes)
		if prevEx, seen := virtualExists[key]; seen {
			curExists = prevEx
			curContent = virtualContent[key]
		}

		switch op.Kind {
		case patchOpAdd:
			if curExists {
				return nil, nil, fmt.Errorf("Add File %s failed: file already exists", op.Path)
			}
			if err := validateManagedStateWrite(projectRoot, cfg, op.Path, op.AddBody); err != nil {
				return nil, nil, err
			}
			plan.newBytes = []byte(op.AddBody)
			plan.diffPreview = fmt.Sprintf("=== ADD %s ===\n%s", plan.relPath, simpleDiff("", op.AddBody))
			virtualExists[key] = true
			virtualContent[key] = op.AddBody

		case patchOpDelete:
			if !curExists {
				return nil, nil, fmt.Errorf("Delete File %s failed: file does not exist", op.Path)
			}
			plan.diffPreview = fmt.Sprintf("=== DELETE %s ===\n%s", plan.relPath, simpleDiff(curContent, ""))
			virtualExists[key] = false
			virtualContent[key] = ""

		case patchOpUpdate:
			if !curExists {
				return nil, nil, fmt.Errorf("Update File %s failed: file does not exist", op.Path)
			}
			if diskExists && !isProbablyText(diskBytes) {
				return nil, nil, fmt.Errorf("refusing to patch binary or non-UTF-8 file: %s", op.Path)
			}
			updated, applyErr := applyHunksToContent(curContent, op.Hunks)
			if applyErr != nil {
				return nil, nil, fmt.Errorf("Update File %s failed: %w", op.Path, applyErr)
			}
			targetRel := op.Path
			if op.MoveTo != "" {
				targetRel = op.MoveTo
				moveFull := resolvedPaths[i].moveFull
				if mInfo, mErr := os.Stat(moveFull); mErr == nil {
					if mInfo.IsDir() {
						return nil, nil, fmt.Errorf("patch move target is a directory: %s", op.MoveTo)
					}
					plan.moveDestExists = true
					plan.moveDestMode = mInfo.Mode().Perm()
					mBytes, rErr := os.ReadFile(moveFull)
					if rErr != nil {
						return nil, nil, fmt.Errorf("read move destination %s: %w", op.MoveTo, rErr)
					}
					plan.moveDestBytes = mBytes
					plan.moveDestVersion = versionForBytes(mBytes)
				} else if !errors.Is(mErr, os.ErrNotExist) {
					return nil, nil, fmt.Errorf("stat move destination %s: %w", op.MoveTo, mErr)
				}
			}
			if err := validateManagedStateWrite(projectRoot, cfg, targetRel, updated); err != nil {
				return nil, nil, err
			}
			plan.newBytes = []byte(updated)
			header := fmt.Sprintf("=== UPDATE %s ===", plan.relPath)
			if op.MoveTo != "" {
				header = fmt.Sprintf("=== UPDATE & MOVE %s -> %s ===", plan.relPath, plan.moveToRel)
			}
			plan.diffPreview = header + "\n" + simpleDiff(curContent, updated)
			if op.MoveTo != "" {
				moveKey := editLockKey(resolvedPaths[i].moveFull)
				virtualExists[key] = false
				virtualContent[key] = ""
				virtualExists[moveKey] = true
				virtualContent[moveKey] = updated
			} else {
				virtualExists[key] = true
				virtualContent[key] = updated
			}
		}

		plans = append(plans, plan)
	}

	return plans, lockPaths, nil
}

func previewApplyPatch(projectRoot string, cfg Config, patchText string) (string, error) {
	plans, _, err := planApplyPatch(projectRoot, cfg, patchText)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Multi-file patch preview (%d file operation(s)):\n\n", len(plans))
	for _, p := range plans {
		b.WriteString(p.diffPreview)
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String()), nil
}

func restoreFileBytesAtomic(fullPath string, data []byte, mode os.FileMode) error {
	if mode == 0 {
		mode = 0o644
	}
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".localcode-rollback-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	keep := true
	defer func() {
		if keep {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(mode.Perm()); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	_, statErr := os.Stat(fullPath)
	targetExists := statErr == nil
	if err := replaceFileAtomic(tmpName, fullPath, targetExists); err != nil {
		return err
	}
	keep = false
	return nil
}

func rollbackCommittedPatches(committed []committedFileSnapshot) error {
	var rollbackErrs []string
	for i := len(committed) - 1; i >= 0; i-- {
		snap := committed[i]
		if snap.moved {
			// Restore or remove the move destination first
			if snap.moveDestExists {
				if err := restoreFileBytesAtomic(snap.moveToFull, snap.moveDestBytes, snap.moveDestMode); err != nil {
					rollbackErrs = append(rollbackErrs, fmt.Sprintf("restore move dest %s: %v", snap.moveToFull, err))
				}
			} else {
				if err := os.Remove(snap.moveToFull); err != nil && !errors.Is(err, os.ErrNotExist) {
					rollbackErrs = append(rollbackErrs, fmt.Sprintf("remove move dest %s: %v", snap.moveToFull, err))
				}
			}
			// Restore original source file
			if snap.originalExists {
				if err := restoreFileBytesAtomic(snap.fullPath, snap.originalBytes, snap.originalMode); err != nil {
					rollbackErrs = append(rollbackErrs, fmt.Sprintf("restore move source %s: %v", snap.fullPath, err))
				}
			}
			continue
		}
		if snap.originalExists {
			if err := restoreFileBytesAtomic(snap.fullPath, snap.originalBytes, snap.originalMode); err != nil {
				rollbackErrs = append(rollbackErrs, fmt.Sprintf("restore %s: %v", snap.fullPath, err))
			}
		} else {
			if err := os.Remove(snap.fullPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				rollbackErrs = append(rollbackErrs, fmt.Sprintf("remove added %s: %v", snap.fullPath, err))
			}
		}
	}
	if len(rollbackErrs) > 0 {
		return errors.New(strings.Join(rollbackErrs, "; "))
	}
	return nil
}

// applyPatchToProject applies a single- or multi-file patch transactionally
// with strict root containment, edit path locking, pre-edit backups,
// optimistic concurrency verification, and full atomic rollback on any failure.
func applyPatchToProject(projectRoot string, cfg Config, patchText string) (string, error) {
	// Parse and validate paths first so we can acquire locks in deterministic order
	// BEFORE reading file versions and computing hunks.
	ops, err := parseApplyPatch(patchText)
	if err != nil {
		return "", err
	}
	lockSet := make(map[string]string)
	for i, op := range ops {
		if strings.TrimSpace(op.Path) == "" {
			return "", fmt.Errorf("patch operation #%d has empty path", i+1)
		}
		full, err := ensureWithinRoot(projectRoot, op.Path)
		if err != nil {
			return "", fmt.Errorf("patch path rejected (%s): %w", op.Path, err)
		}
		lockSet[editLockKey(full)] = full
		if op.MoveTo != "" {
			moveFull, err := ensureWithinRoot(projectRoot, op.MoveTo)
			if err != nil {
				return "", fmt.Errorf("patch move destination rejected (%s): %w", op.MoveTo, err)
			}
			lockSet[editLockKey(moveFull)] = moveFull
		}
	}
	lockKeys := make([]string, 0, len(lockSet))
	for k := range lockSet {
		lockKeys = append(lockKeys, k)
	}
	sort.Strings(lockKeys)
	releases := make([]func(), 0, len(lockKeys))
	for _, k := range lockKeys {
		releases = append(releases, acquireEditPathLock(lockSet[k]))
	}
	defer func() {
		for i := len(releases) - 1; i >= 0; i-- {
			releases[i]()
		}
	}()

	// Re-plan under held path locks so versions and contents are consistent.
	plans, _, err := planApplyPatch(projectRoot, cfg, patchText)
	if err != nil {
		return "", err
	}

	// Track live expected versions if multiple operations target the same file.
	liveVersion := make(map[string]fileVersion)
	for _, p := range plans {
		k := editLockKey(p.fullPath)
		if _, seen := liveVersion[k]; !seen {
			liveVersion[k] = p.expectedVersion
		}
		if p.moveToFull != "" {
			mk := editLockKey(p.moveToFull)
			if _, seen := liveVersion[mk]; !seen {
				liveVersion[mk] = p.moveDestVersion
			}
		}
	}

	committed := make([]committedFileSnapshot, 0, len(plans))
	rollbackWithErr := func(opIdx int, relPath string, cause error) (string, error) {
		rbErr := rollbackCommittedPatches(committed)
		if rbErr != nil {
			return "", fmt.Errorf("apply_patch failed at file #%d (%s): %w (atomic rollback encountered errors: %v)", opIdx+1, relPath, cause, rbErr)
		}
		return "", fmt.Errorf("apply_patch failed at file #%d (%s): %w (rolled back %d previously applied file(s))", opIdx+1, relPath, cause, len(committed))
	}

	for _, p := range plans {
		if applyPatchBeforeCommitOpHook != nil {
			if hookErr := applyPatchBeforeCommitOpHook(p.opIndex, p.relPath); hookErr != nil {
				return rollbackWithErr(p.opIndex, p.relPath, hookErr)
			}
		}

		// Capture pre-mutation snapshot for rollback (using current live disk bytes).
		snap := committedFileSnapshot{
			kind:     p.kind,
			fullPath: p.fullPath,
		}
		if info, statErr := os.Stat(p.fullPath); statErr == nil && !info.IsDir() {
			snap.originalExists = true
			snap.originalMode = info.Mode().Perm()
			if curBytes, rErr := os.ReadFile(p.fullPath); rErr == nil {
				snap.originalBytes = curBytes
			}
		}

		key := editLockKey(p.fullPath)
		expectedVer := liveVersion[key]

		switch p.kind {
		case patchOpAdd:
			if err := atomicWriteFileIfVersion(p.fullPath, p.newBytes, 0o644, expectedVer); err != nil {
				return rollbackWithErr(p.opIndex, p.relPath, err)
			}
			liveVersion[key] = versionForBytes(p.newBytes)
			committed = append(committed, snap)

		case patchOpDelete:
			if err := createPreEditBackup(projectRoot, p.fullPath); err != nil {
				return rollbackWithErr(p.opIndex, p.relPath, err)
			}
			if err := verifyFileVersion(p.fullPath, expectedVer); err != nil {
				return rollbackWithErr(p.opIndex, p.relPath, err)
			}
			if err := os.Remove(p.fullPath); err != nil {
				return rollbackWithErr(p.opIndex, p.relPath, err)
			}
			liveVersion[key] = fileVersion{}
			committed = append(committed, snap)

		case patchOpUpdate:
			if err := createPreEditBackup(projectRoot, p.fullPath); err != nil {
				return rollbackWithErr(p.opIndex, p.relPath, err)
			}
			mode := p.originalMode
			if mode == 0 {
				mode = 0o644
			}
			if p.moveToFull != "" {
				snap.moved = true
				snap.moveToFull = p.moveToFull
				if mInfo, mErr := os.Stat(p.moveToFull); mErr == nil && !mInfo.IsDir() {
					snap.moveDestExists = true
					snap.moveDestMode = mInfo.Mode().Perm()
					if mBytes, rErr := os.ReadFile(p.moveToFull); rErr == nil {
						snap.moveDestBytes = mBytes
					}
					if err := createPreEditBackup(projectRoot, p.moveToFull); err != nil {
						return rollbackWithErr(p.opIndex, p.moveToRel, err)
					}
				}
				moveKey := editLockKey(p.moveToFull)
				if err := verifyFileVersion(p.fullPath, expectedVer); err != nil {
					return rollbackWithErr(p.opIndex, p.relPath, err)
				}
				if err := atomicWriteFileIfVersion(p.moveToFull, p.newBytes, mode, liveVersion[moveKey]); err != nil {
					return rollbackWithErr(p.opIndex, p.moveToRel, err)
				}
				if err := os.Remove(p.fullPath); err != nil {
					// Record move destination before rolling back so it gets cleaned up too.
					committed = append(committed, snap)
					return rollbackWithErr(p.opIndex, p.relPath, err)
				}
				liveVersion[key] = fileVersion{}
				liveVersion[moveKey] = versionForBytes(p.newBytes)
				committed = append(committed, snap)
			} else {
				if bytes.Equal(snap.originalBytes, p.newBytes) && snap.originalExists {
					// Still write/verify to honor version check and keep deterministic result.
					if err := verifyFileVersion(p.fullPath, expectedVer); err != nil {
						return rollbackWithErr(p.opIndex, p.relPath, err)
					}
				} else {
					if err := atomicWriteFileIfVersion(p.fullPath, p.newBytes, mode, expectedVer); err != nil {
						return rollbackWithErr(p.opIndex, p.relPath, err)
					}
				}
				liveVersion[key] = versionForBytes(p.newBytes)
				committed = append(committed, snap)
			}
		}
	}

	var out strings.Builder
	fmt.Fprintf(&out, "PATCH APPLIED (%d file(s)):\n", len(plans))
	for _, p := range plans {
		if p.moveToRel != "" {
			fmt.Fprintf(&out, "- %s: %s -> %s\n", strings.ToUpper(string(p.kind)), p.relPath, p.moveToRel)
		} else {
			fmt.Fprintf(&out, "- %s: %s\n", strings.ToUpper(string(p.kind)), p.relPath)
		}
	}
	out.WriteByte('\n')
	for _, p := range plans {
		out.WriteString(p.diffPreview)
		out.WriteString("\n\n")
	}
	return strings.TrimSpace(out.String()), nil
}

func patchTextForAction(a AgentAction) string {
	if strings.TrimSpace(a.Patch) != "" {
		return a.Patch
	}
	return a.Content
}
