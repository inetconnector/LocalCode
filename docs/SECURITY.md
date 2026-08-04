# Security Model

LocalCode uses layered controls:

- configurable approval modes
- project/workspace/unrestricted path policy
- additional explicit allowed roots
- command regex denylist
- hard Git guards for destructive history/worktree operations
- network enable switch
- SSRF checks for web fetch
- MCP server allowlist through settings
- explicit approval for MCP tool calls
- timeouts and response size limits
- local file backups before edits

## Approval modes

- `strict`: network requests and all changes/commands require approval.
- `balanced`: reads and web research can run; mutations, commands, Git mutations and MCP tool calls require approval.
- `auto`: project file edits and recognized read-only commands can run; external/risky actions still require approval.
- `dangerous`: no approval prompts. Not recommended.

## Not an OS sandbox

The native Windows backend does not isolate commands in a separate Windows sandbox token or container. Path checks apply to built-in file tools, but arbitrary shell commands can access whatever the user account can access. Use strict approval, a non-administrator account and trusted repositories. An OS-enforced WSL2/bubblewrap or Windows sandbox backend remains future work.
