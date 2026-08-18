from pathlib import Path

path = Path("src/agent.go")
text = path.read_text(encoding="utf-8")

if "expectedFileVersion *fileVersion" in text:
    raise SystemExit(0)

replacements = [
    (
        '\tResource      string         `json:"resource,omitempty"`\n\tScript        string         `json:"script,omitempty"`\n}',
        '\tResource            string         `json:"resource,omitempty"`\n\tScript              string         `json:"script,omitempty"`\n\texpectedFileVersion *fileVersion\n}',
    ),
    (
        '\tpreview, err := previewAction(project, cfg, a)\n\tif err != nil {\n\t\treturn "ERROR: " + err.Error(), false\n\t}\n\tapproved := true',
        '\tprecondition, err := captureApprovedFilePrecondition(project, a)\n\tif err != nil {\n\t\treturn "ERROR: " + err.Error(), false\n\t}\n\tpreview, err := previewAction(project, cfg, a)\n\tif err != nil {\n\t\treturn "ERROR: " + err.Error(), false\n\t}\n\tif precondition != nil {\n\t\tif err := verifyApprovedFilePrecondition(*precondition); err != nil {\n\t\t\treturn "ERROR: " + err.Error(), false\n\t\t}\n\t}\n\tapproved := true',
    ),
    (
        '\tif !approved {\n\t\treturn "REJECTED BY USER", false\n\t}\n\ts.AddEvent(UIEvent{Type: "action_running", Message: a.Message, Action: a.Action, Path: a.Path, Command: a.Command, Preview: preview})',
        '\tif !approved {\n\t\treturn "REJECTED BY USER", false\n\t}\n\tif precondition != nil {\n\t\tif err := verifyApprovedFilePrecondition(*precondition); err != nil {\n\t\t\tdetail := "ERROR: " + err.Error()\n\t\t\ts.AddEvent(UIEvent{Type: "tool_error", Message: a.Message, Detail: detail, Preview: preview, Action: a.Action, Path: a.Path, Command: a.Command})\n\t\t\treturn detail, false\n\t\t}\n\t\tversion := precondition.Version\n\t\ta.expectedFileVersion = &version\n\t}\n\ts.AddEvent(UIEvent{Type: "action_running", Message: a.Message, Action: a.Action, Path: a.Path, Command: a.Command, Preview: preview})',
    ),
    (
        '\t\treturn replaceText(project, a.Path, a.OldText, a.NewText)\n\tcase "write_file":\n\t\tif err := validateManagedStateWrite(project, cfg, a.Path, a.Content); err != nil {\n\t\t\treturn "", err\n\t\t}\n\t\treturn writeProjectFile(project, a.Path, a.Content)',
        '\t\tif a.expectedFileVersion != nil {\n\t\t\treturn replaceTextAtVersion(project, a.Path, a.OldText, a.NewText, *a.expectedFileVersion)\n\t\t}\n\t\treturn replaceText(project, a.Path, a.OldText, a.NewText)\n\tcase "write_file":\n\t\tif err := validateManagedStateWrite(project, cfg, a.Path, a.Content); err != nil {\n\t\t\treturn "", err\n\t\t}\n\t\tif a.expectedFileVersion != nil {\n\t\t\treturn writeProjectFileAtVersion(project, a.Path, a.Content, *a.expectedFileVersion)\n\t\t}\n\t\treturn writeProjectFile(project, a.Path, a.Content)',
    ),
]

for old, new in replacements:
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"patch anchor expected once, found {count}: {old[:100]!r}")
    text = text.replace(old, new, 1)

old = '\tcase "delete_file":\n\t\treturn deleteProjectFile(project, a.Path)'
pos = text.rfind(old)
if pos < 0:
    raise SystemExit("delete_file execution anchor not found")
new = '\tcase "delete_file":\n\t\tif a.expectedFileVersion != nil {\n\t\t\treturn deleteProjectFileAtVersion(project, a.Path, *a.expectedFileVersion)\n\t\t}\n\t\treturn deleteProjectFile(project, a.Path)'
text = text[:pos] + new + text[pos + len(old):]

path.write_text(text, encoding="utf-8", newline="\n")
