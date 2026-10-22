from pathlib import Path
import json,os
r=Path('/home/nacho/pfi/tesis-serra-zarlenga-fabric')
p=r/'build/benchmarks/paired/.active.lock'
for proc in Path('/proc').iterdir():
 if not proc.name.isdigit(): continue
 try: args=(proc/'cmdline').read_bytes().split(b'\0')
 except (FileNotFoundError,PermissionError,ProcessLookupError): continue
 if args and b'node' in Path(os.fsdecode(args[0])).name.encode() and any(a.endswith(b'/run-paired-series.js') for a in args[1:]):
  raise SystemExit('A paired controller is active; keep lock')
if p.exists():
 d=json.loads(p.read_text())
 assert d['campaignToken']=='eval67-389ca16-20261021b-full', 'unexpected lock owner'
 target=p.with_name('.active.lock.interrupted-eval67-389ca16-20261021b')
 assert not target.exists(), 'archive exists; do not replace'
 p.rename(target)
 print('Interrupted controller lock archived; originals retained')
else: print('No stale lock to archive')
