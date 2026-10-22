#!/usr/bin/env python3
"""Reproduce evidence strictly; original snapshot bundles are required outside Git."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--fabric-snapshot', type=Path, required=True)
parser.add_argument('--baseline-snapshot', type=Path, required=True)
parser.add_argument('--output', type=Path, required=True)
parser.add_argument('--node', default='node')
args = parser.parse_args()
here = Path(__file__).resolve().parent
root = next(parent for parent in here.parents if (parent / 'benchmarks/analysis/src/process.js').is_file())
output = args.output.resolve()
if output.exists() or output == here or here in output.parents:
    raise SystemExit('Choose a new output directory outside the evidence tree.')
for line in (here / 'SHA256SUMS').read_text().splitlines():
    expected, name = line.split('  ', 1)
    source = (here / name).resolve()
    if here not in source.parents or not source.is_file():
        raise SystemExit('Missing or unsafe evidence path: ' + name)
    if hashlib.sha256(source.read_bytes()).hexdigest() != expected:
        raise SystemExit('Evidence hash mismatch: ' + name)
campaign = json.loads((here / 'campaign.json').read_text())
for sut, bundle in [('fabric', args.fabric_snapshot), ('baseline', args.baseline_snapshot)]:
    manifest = bundle.resolve() / 'manifest.json'
    if hashlib.sha256(manifest.read_bytes()).hexdigest() != campaign['snapshots'][sut]['manifestSHA256']:
        raise SystemExit('Snapshot manifest mismatch: ' + sut)
command = [args.node, str(root / 'benchmarks/analysis/src/process.js'),
           '--input', str(here / 'fabric'), '--input', str(here / 'baseline'),
           '--snapshot', str(args.fabric_snapshot.resolve()), '--snapshot', str(args.baseline_snapshot.resolve()),
           '--campaign', str(here / 'campaign.json'), '--output', str(output)]
subprocess.run(command, cwd=root, check=True)
original = '/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/' + here.name
checked = []
for expected in sorted((here / 'analysis').rglob('*')):
    if not expected.is_file():
        continue
    relative = expected.relative_to(here / 'analysis')
    actual = output / relative
    left = expected.read_text().replace(original, '<RUNS>')
    right = actual.read_text().replace(str(here), '<RUNS>')
    if left != right:
        raise SystemExit('Reproduction differs: ' + str(relative))
    checked.append(str(relative))
print(json.dumps({'status': 'passed', 'outputsCompared': checked,
                  'comparisonSHA256': hashlib.sha256((output / 'comparison.csv').read_bytes()).hexdigest()}, indent=2))
