from pathlib import Path


def replace_once(text: str, old: str, new: str, label: str) -> str:
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{label}: expected anchor once, found {count}")
    return text.replace(old, new, 1)


agent_path = Path("src/agent.go")
text = agent_path.read_text(encoding="utf-8")
if 'json:"line,omitempty"' not in text:
    text = replace_once(
        text,
        '\tHeight              int            `json:"height,omitempty"`\n\tURL                 string         `json:"url,omitempty"`',
        '\tHeight              int            `json:"height,omitempty"`\n\tLine                int            `json:"line,omitempty"`\n\tCharacter           int            `json:"character,omitempty"`\n\tURL                 string         `json:"url,omitempty"`',
        "AgentAction line fields",
    )
    text = replace_once(
        text,
        '"list_files", "read_file", "search_text", "replace_text", "write_file", "delete_file",',
        '"list_files", "read_file", "search_text", "lsp", "replace_text", "write_file", "delete_file",',
        "action enum",
    )
    text = replace_once(
        text,
        '\t\t"command": map[string]any{"type": "string"}, "max_depth": map[string]any{"type": "integer", "minimum": 1, "maximum": 8},\n\t\t"width": map[string]any{"type": "integer", "minimum": 1, "maximum": 4096}, "height": map[string]any{"type": "integer", "minimum": 1, "maximum": 4096},',
        '\t\t"command": map[string]any{"type": "string"}, "max_depth": map[string]any{"type": "integer", "minimum": 1, "maximum": 8},\n\t\t"width": map[string]any{"type": "integer", "minimum": 1, "maximum": 4096}, "height": map[string]any{"type": "integer", "minimum": 1, "maximum": 4096},\n\t\t"line": map[string]any{"type": "integer", "minimum": 1}, "character": map[string]any{"type": "integer", "minimum": 1},',
        "schema line fields",
    )
    text = replace_once(
        text,
        '\t\tconditionalRequired("search_text", "query"),\n\t\tconditionalRequired("replace_text", "path", "old_text"),',
        '\t\tconditionalRequired("search_text", "query"),\n\t\tconditionalRequired("lsp", "path", "name"),\n\t\tconditionalRequired("replace_text", "path", "old_text"),',
        "lsp conditional required",
    )
    text = replace_once(
        text,
        '- Rate nicht über vorhandenen Code. Lies relevante Dateien und suche gezielt.\n',
        '- Rate nicht über vorhandenen Code. Lies relevante Dateien und suche gezielt.\n- Nutze lsp(name,path,line,character,query) für semantische Navigation, wenn ein passender lokaler Language Server verfügbar ist. Unterstützt werden definition, references, hover, documentSymbol, workspaceSymbol, implementation, prepareCallHierarchy, incomingCalls und outgoingCalls. LSP ist unverändernd; wenn kein Server verfügbar ist, falle auf Repository-Intelligence, search_text und read_file zurück.\n',
        "system prompt lsp guidance",
    )
    text = replace_once(
        text,
        '- list_files, read_file, search_text\n',
        '- list_files, read_file, search_text\n- lsp(name,path,line,character,query) für read-only Definitionen/Referenzen/Symbole/Hover/Implementierungen/Call-Hierarchy\n',
        "tool list lsp",
    )
    text = replace_once(
        text,
        '\tcase "search_text":\n\t\treturn require("query", a.Query)\n\tcase "replace_text":',
        '\tcase "search_text":\n\t\treturn require("query", a.Query)\n\tcase "lsp":\n\t\tif err := require("path", a.Path); err != nil {\n\t\t\treturn err\n\t\t}\n\t\tif err := require("name", a.Name); err != nil {\n\t\t\treturn err\n\t\t}\n\t\tswitch normalizeLSPOperation(a.Name) {\n\t\tcase "documentsymbol", "symbols", "workspacesymbol":\n\t\t\treturn nil\n\t\tcase "definition", "gotodefinition", "references", "findreferences", "hover", "implementation", "gotoimplementation", "preparecallhierarchy", "incomingcalls", "outgoingcalls":\n\t\t\tif a.Line < 1 || a.Character < 1 {\n\t\t\t\treturn errors.New("lsp position operation requires positive line and character")\n\t\t\t}\n\t\t\treturn nil\n\t\tdefault:\n\t\t\treturn fmt.Errorf("unsupported LSP operation %q", a.Name)\n\t\t}\n\tcase "replace_text":',
        "validate lsp",
    )
    text = replace_once(
        text,
        'case "mcp_call_tool", "replace_text", "write_file", "delete_file",',
        'case "mcp_call_tool", "lsp", "replace_text", "write_file", "delete_file",',
        "dispatch lsp approval path",
    )
    text = replace_once(
        text,
        '\tcase "web_search", "web_fetch":\n\t\treturn cfg.ApprovalMode == "strict"\n\tcase "replace_text",',
        '\tcase "web_search", "web_fetch":\n\t\treturn cfg.ApprovalMode == "strict"\n\tcase "lsp":\n\t\t// A language server is an external project-aware process. The protocol\n\t\t// path is read-only, but strict mode still surfaces the process start.\n\t\treturn cfg.ApprovalMode == "strict"\n\tcase "replace_text",',
        "lsp approval",
    )
    text = replace_once(
        text,
        '\tcase "delete_file":\n\t\told, err := readProjectFile(project, a.Path)\n\t\tif err != nil {\n\t\t\treturn "", err\n\t\t}\n\t\treturn simpleDiff(old, ""), nil\n\tcase "run_tool":',
        '\tcase "delete_file":\n\t\told, err := readProjectFile(project, a.Path)\n\t\tif err != nil {\n\t\t\treturn "", err\n\t\t}\n\t\treturn simpleDiff(old, ""), nil\n\tcase "lsp":\n\t\tfull, err := ensureWithinRoot(project, a.Path)\n\t\tif err != nil {\n\t\t\treturn "", err\n\t\t}\n\t\tspec, err := resolveLSPServer(project, cfg, full)\n\t\tif err != nil {\n\t\t\treturn "", err\n\t\t}\n\t\tposition := ""\n\t\tif a.Line > 0 && a.Character > 0 {\n\t\t\tposition = fmt.Sprintf("\\nPosition: %d:%d", a.Line, a.Character)\n\t\t}\n\t\treturn fmt.Sprintf("Read-only LSP query\\nServer: %s\\nOperation: %s\\nPath: %s%s\\nServer-originated workspace/applyEdit requests are refused; query has a bounded timeout and process-tree cancellation.", spec.Tool, a.Name, a.Path, position), nil\n\tcase "run_tool":',
        "lsp preview",
    )
    text = replace_once(
        text,
        '\t\treturn deleteProjectFile(project, a.Path)\n\tcase "build_project":',
        '\t\treturn deleteProjectFile(project, a.Path)\n\tcase "lsp":\n\t\treturn runLSPAction(ctx, project, cfg, a)\n\tcase "build_project":',
        "lsp execute",
    )
    agent_path.write_text(text, encoding="utf-8", newline="\n")

supervisor_path = Path("src/agent_supervisor.go")
supervisor = supervisor_path.read_text(encoding="utf-8")
if '"search_text", "lsp", "command_list"' not in supervisor:
    supervisor = replace_once(
        supervisor,
        '"list_files", "read_file", "search_text", "command_list",',
        '"list_files", "read_file", "search_text", "lsp", "command_list",',
        "supervisor analyze lsp",
    )
    supervisor_path.write_text(supervisor, encoding="utf-8", newline="\n")

approval_path = Path("src/approval_rules.go")
approval = approval_path.read_text(encoding="utf-8")
if 'case "lsp":' not in approval:
    approval = replace_once(
        approval,
        '\tcase "write_file", "replace_text", "delete_file", "create_svg_asset", "create_image_asset":\n\t\treturn []string{a.Action, filepath.ToSlash(filepath.Clean(a.Path))}',
        '\tcase "write_file", "replace_text", "delete_file", "create_svg_asset", "create_image_asset":\n\t\treturn []string{a.Action, filepath.ToSlash(filepath.Clean(a.Path))}\n\tcase "lsp":\n\t\treturn []string{"lsp", normalizeLSPOperation(a.Name), filepath.ToSlash(filepath.Clean(a.Path))}',
        "approval lsp tokens",
    )
    approval_path.write_text(approval, encoding="utf-8", newline="\n")
