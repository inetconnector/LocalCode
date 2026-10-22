"""Offline tests for EDID Sentinel. No physical hardware is accessed."""
import json
import random
import sys
import tempfile
import unittest
from pathlib import Path
from edidsentinel.edid import HEADER, fix_checksums, make_seed, mutate
from edidsentinel.runner import execute, parse_command, write_finding

class TestEDID(unittest.TestCase):
    def test_seed_shapes_and_checksums(self):
        for kind,size in (("base",128),("cta",256),("displayid",256)):
            blob=make_seed(kind)
            self.assertEqual(len(blob),size)
            self.assertEqual(blob[:8],HEADER)
            for i in range(0,len(blob),128):
                self.assertEqual(sum(blob[i:i+128])%256,0)
            self.assertEqual(blob[126],size//128-1)

    def test_fix_checksum(self):
        blob=bytearray(make_seed())
        blob[90]^=7
        self.assertNotEqual(sum(blob)%256,0)
        repaired=fix_checksums(blob)
        self.assertEqual(sum(repaired)%256,0)

    def test_mutation_bounded(self):
        rng=random.Random(9)
        corpus=[make_seed(x) for x in ("base","cta","displayid")]
        for _ in range(300):
            data,ops=mutate(rng.choice(corpus),rng,256)
            self.assertLessEqual(len(data),256)
            self.assertGreaterEqual(len(ops),1)

    def test_determinism(self):
        a=mutate(make_seed(),random.Random(112))
        b=mutate(make_seed(),random.Random(112))
        self.assertEqual(a,b)

class TestRunner(unittest.TestCase):
    def test_reject_is_not_crash(self):
        with tempfile.TemporaryDirectory() as work:
            script=Path(work)/"reject.py"
            script.write_text("import sys\nsys.exit(2)\n")
            self.assertEqual(execute(b"x",[sys.executable,str(script)],2).status,"reject")

    def test_ok(self):
        with tempfile.TemporaryDirectory() as work:
            script=Path(work)/"ok.py"
            script.write_text("import pathlib,sys\nassert pathlib.Path(sys.argv[1]).read_bytes()==b'ok'\n")
            self.assertEqual(execute(b"ok",[sys.executable,str(script),"{input}"],2).status,"ok")

    @unittest.skipIf(sys.platform=="win32","POSIX signal semantics")
    def test_signal_is_crash(self):
        with tempfile.TemporaryDirectory() as work:
            script=Path(work)/"crash.py"
            script.write_text("import os,signal\nos.kill(os.getpid(),signal.SIGABRT)\n")
            result=execute(b"x",[sys.executable,str(script)],2)
            self.assertEqual(result.status,"crash")
            saved=write_finding(Path(work),b"x",result,["synthetic"],1)
            self.assertEqual(saved.read_bytes(),b"x")
            self.assertEqual(json.loads(saved.with_suffix(".json").read_text())["status"],"crash")

    def test_timeout(self):
        with tempfile.TemporaryDirectory() as work:
            script=Path(work)/"sleep.py"
            script.write_text("import time\ntime.sleep(2)\n")
            self.assertEqual(execute(b"wait",[sys.executable,str(script)],.02).status,"timeout")

    def test_command(self):
        self.assertEqual(parse_command("foo --bar {input}"),["foo","--bar","{input}"])

if __name__=="__main__":
    unittest.main()
