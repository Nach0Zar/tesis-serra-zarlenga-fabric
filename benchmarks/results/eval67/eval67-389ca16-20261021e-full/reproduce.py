#!/usr/bin/env python3
"""Reproduce release evidence strictly; original snapshot bundles remain external."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--evidence-dir', type=Path, required=True,
                    help='Campaign directory extracted from the evidence release archive')
parser.add_argument('--fabric-snapshot', type=Path, required=True)
parser.add_argument('--baseline-snapshot', type=Path, required=True)
parser.add_argument('--output', type=Path, required=True)
parser.add_argument('--node', default='node')
args = parser.parse_args()
here = Path(__file__).resolve().parent
root = next(parent for parent in here.parents if (parent / 'benchmarks/analysis/src/process.js').is_file())
evidence = args.evidence_dir.resolve()
if not (evidence / 'campaign.json').is_file():
    raise SystemExit('Missing campaign.json: download and extract the release, then pass its campaign directory.')
output = args.output.resolve()
protected = [here, evidence, args.fabric_snapshot.resolve(), args.baseline_snapshot.resolve()]
if output.exists() or any(output == folder or folder in output.parents for folder in protected):
    raise SystemExit('Choose a new output directory outside the evidence, repository results and snapshot bundles.')
manifest = here / 'SHA256SUMS'
if (evidence / 'SHA256SUMS').read_bytes() != manifest.read_bytes():
    raise SystemExit('Release evidence manifest differs from the pinned campaign SHA256SUMS.')
for line in manifest.read_text().splitlines():
    expected, name = line.split('  ', 1)
    source = (evidence / name).resolve()
    if evidence not in source.parents or not source.is_file():
        raise SystemExit('Missing or unsafe evidence path: ' + name)
    if hashlib.sha256(source.read_bytes()).hexdigest() != expected:
        raise SystemExit('Evidence hash mismatch: ' + name)
    if name.startswith('analysis/'):
        reference = here / name
        if not reference.is_file() or hashlib.sha256(reference.read_bytes()).hexdigest() != expected:
            raise SystemExit('Repository analysis differs from pinned evidence: ' + name)
campaign = json.loads((evidence / 'campaign.json').read_text())
for sut, bundle in [('fabric', args.fabric_snapshot), ('baseline', args.baseline_snapshot)]:
    snapshot_manifest = bundle.resolve() / 'manifest.json'
    if hashlib.sha256(snapshot_manifest.read_bytes()).hexdigest() != campaign['snapshots'][sut]['manifestSHA256']:
        raise SystemExit('Snapshot manifest mismatch: ' + sut)
command = [args.node, str(root / 'benchmarks/analysis/src/process.js'),
           '--input', str(evidence / 'fabric'), '--input', str(evidence / 'baseline'),
           '--snapshot', str(args.fabric_snapshot.resolve()), '--snapshot', str(args.baseline_snapshot.resolve()),
           '--campaign', str(evidence / 'campaign.json'), '--output', str(output)]
subprocess.run(command, cwd=root, check=True)
original = '/home/nacho/pfi/tesis-serra-zarlenga-fabric/build/benchmarks/paired/' + here.name
checked = []
for expected in sorted((here / 'analysis').rglob('*')):
    if not expected.is_file():
        continue
    relative = expected.relative_to(here / 'analysis')
    actual = output / relative
    left = expected.read_text().replace(original, '<RUNS>')
    right = actual.read_text().replace(str(evidence), '<RUNS>')
    if left != right:
        raise SystemExit('Reproduction differs: ' + str(relative))
    checked.append(str(relative))
print(json.dumps({'status': 'passed', 'outputsCompared': checked,
                  'comparisonSHA256': hashlib.sha256((output / 'comparison.csv').read_bytes()).hexdigest()}, indent=2))
