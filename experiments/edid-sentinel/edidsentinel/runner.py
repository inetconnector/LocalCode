"""Execute explicitly selected local parser in isolated subprocess and collect evidence."""
from __future__ import annotations
import hashlib
import json
import os
import re
import shlex
import subprocess
import tempfile
from dataclasses import dataclass
from pathlib import Path

ASAN = re.compile(r"(AddressSanitizer|UndefinedBehaviorSanitizer|runtime error:|LeakSanitizer|MemorySanitizer|SUMMARY:.*Sanitizer)", re.I)
WINDOWS_CRASH_CODES = {0xC0000005,0xC000001D,0xC0000094,0xC00000FD,0xC0000409}

@dataclass
class Execution:
    status: str
    returncode: int | None
    stderr: str
    stdout: str

def parse_command(command: str) -> list[str]:
    argv = shlex.split(command,posix=(os.name!="nt"))
    if not argv:
        raise ValueError("empty command")
    return argv

def execute(blob:bytes,command:list[str],timeout:float)->Execution:
    with tempfile.TemporaryDirectory(prefix="edid-sentinel-") as tmp:
        path=Path(tmp)/"input.bin"
        path.write_bytes(blob)
        argv=[s.replace("{input}",str(path)) for s in command]
        if not any("{input}" in s for s in command):
            argv.append(str(path))
        try:
            result=subprocess.run(argv,stdout=subprocess.PIPE,stderr=subprocess.PIPE,
                                  timeout=timeout,check=False,stdin=subprocess.DEVNULL)
        except subprocess.TimeoutExpired as exc:
            return Execution("timeout",None,(exc.stderr or b"").decode(errors="replace")[-16000:],
                             (exc.stdout or b"").decode(errors="replace")[-16000:])
        stdout=result.stdout.decode(errors="replace")[-16000:]
        stderr=result.stderr.decode(errors="replace")[-16000:]
        rc=result.returncode
        if ASAN.search(stderr) or rc<0 or (rc&0xffffffff) in WINDOWS_CRASH_CODES:
            state="crash"
        elif rc:
            state="reject"
        else:
            state="ok"
        return Execution(state,rc,stderr,stdout)

def write_finding(output_dir:Path,blob:bytes,result:Execution,mutations:list[str],iteration:int)->Path:
    digest=hashlib.sha256(blob).hexdigest()
    kind="crashes" if result.status=="crash" else "timeouts"
    target=output_dir/kind
    target.mkdir(parents=True,exist_ok=True)
    path=target/(digest+".bin")
    path.write_bytes(blob)
    report={
        "case_sha256":digest,"iteration":iteration,"status":result.status,
        "returncode":result.returncode,"size":len(blob),"mutations":mutations,
        "stderr_tail":result.stderr,"stdout_tail":result.stdout,
        "note":"Candidate only. Reproduce with sanitizers before vendor disclosure."
    }
    (target/(digest+".json")).write_text(json.dumps(report,indent=2),encoding="utf-8")
    return path
