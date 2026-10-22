"""EDID Sentinel command line."""
from __future__ import annotations
import argparse
import json
import random
import sys
from collections import Counter
from pathlib import Path
from .edid import make_seed,mutate
from .runner import execute,parse_command,write_finding

def build_parser():
    parser=argparse.ArgumentParser(description="Offline EDID/DisplayID parser robustness tester")
    sub=parser.add_subparsers(dest="action",required=True)
    seeds=sub.add_parser("seeds",help="generate synthetic EDID cases")
    seeds.add_argument("--output",type=Path,default=Path("corpus"))
    fuzz=sub.add_parser("fuzz",help="test an explicitly chosen local parser")
    fuzz.add_argument("--command",required=True,help="use {input} or input path is appended")
    fuzz.add_argument("--iterations",type=int,default=100)
    fuzz.add_argument("--timeout",type=float,default=2)
    fuzz.add_argument("--seed",type=int,default=2026)
    fuzz.add_argument("--corpus",type=Path,default=Path("corpus"))
    fuzz.add_argument("--output",type=Path,default=Path("findings"))
    fuzz.add_argument("--max-bytes",type=int,default=4096)
    fuzz.add_argument("--keep-going",action="store_true")
    return parser

def main(argv=None):
    args=build_parser().parse_args(argv)
    if args.action=="seeds":
        args.output.mkdir(parents=True,exist_ok=True)
        for mode in ("base","cta","displayid"):
            target=args.output/(mode+".bin")
            target.write_bytes(make_seed(mode))
            print("created",target)
        return 0
    if args.iterations<=0 or args.timeout<=0 or args.max_bytes<128:
        raise SystemExit("iterations and timeout must be positive; max-bytes >= 128")
    args.corpus.mkdir(parents=True,exist_ok=True)
    args.output.mkdir(parents=True,exist_ok=True)
    seeds=[p.read_bytes() for p in sorted(args.corpus.glob("*.bin"))
           if 0<p.stat().st_size<=args.max_bytes]
    if not seeds:
        seeds=[make_seed(v) for v in ("base","cta","displayid")]
    command=parse_command(args.command)
    rng=random.Random(args.seed)
    stats=Counter()
    for i in range(args.iterations):
        blob,mutation_names=mutate(rng.choice(seeds),rng,max_bytes=args.max_bytes)
        result=execute(blob,command,args.timeout)
        stats[result.status]+=1
        if result.status in ("crash","timeout"):
            path=write_finding(args.output,blob,result,mutation_names,i)
            print("[%s] iteration=%s saved=%s"%(result.status,i,path),flush=True)
            if result.status=="crash" and not args.keep_going:
                break
        if (i+1)%50==0:
            print("progress",i+1,"/",args.iterations,dict(stats),flush=True)
    report={"command":command,"random_seed":args.seed,
            "planned_iterations":args.iterations,
            "completed_iterations":sum(stats.values()),
            "stats":dict(stats),
            "warning":"An abnormal exit is a candidate, not a verified vulnerability."}
    (args.output/"summary.json").write_text(json.dumps(report,indent=2),encoding="utf-8")
    print(json.dumps(report,indent=2))
    return 1 if stats["crash"] else 0

if __name__=="__main__":
    sys.exit(main())
