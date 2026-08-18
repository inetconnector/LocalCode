from pathlib import Path

path = Path("src/types.go")
text = path.read_text(encoding="utf-8")
old = 'import (\n\t"context"\n\t"log"\n\t"sync"\n\t"time"\n)'
new = 'import (\n\t"context"\n\t"log"\n\t"path/filepath"\n\t"strings"\n\t"sync"\n\t"time"\n)'
count = text.count(old)
if count != 1:
    raise SystemExit(f"expected types import block once, found {count}")
path.write_text(text.replace(old, new, 1), encoding="utf-8", newline="\n")
