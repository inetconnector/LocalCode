from pathlib import Path

path = Path("src/agent.go")
text = path.read_text(encoding="utf-8")
old = "\tcfg := s.Config\n\tthreadID := s.CurrentThread\n\ts.mu.Unlock()"
new = "\tcfg := s.Config\n\tthreadID = s.CurrentThread\n\ts.mu.Unlock()"
count = text.count(old)
if count != 1:
    raise SystemExit(f"expected recovery threadID anchor once, found {count}")
path.write_text(text.replace(old, new, 1), encoding="utf-8", newline="\n")
