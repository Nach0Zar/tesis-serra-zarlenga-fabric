#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
cd /home/nacho/pfi/tesis-serra-zarlenga-fabric
source build/benchmarks/paired/preparation/environment.sh
readonly experiment_commit=389ca160b2a490cd66bb0194f012e2b75b3598e8
readonly token=eval67-389ca16-20261021e
readonly prep="$PWD/build/benchmarks/paired/preparation"
readonly gate="$PWD/build/benchmarks/paired/gate-$token"
mkdir "$gate"
printf 'phase\tstartedAt\tendedAt\tstatus\n' > "$gate/phases.tsv"
current_phase=preflight
phase_start="$(date -u +%FT%TZ)"
sampler_pid=""
failure() {
 local code=$?
 trap - ERR
 if [[ -n "$sampler_pid" ]]; then kill "$sampler_pid" 2>/dev/null || true; wait "$sampler_pid" 2>/dev/null || true; fi
 printf '%s\t%s\t%s\tfailed:%s\n' "$current_phase" "$phase_start" "$(date -u +%FT%TZ)" "$code" >> "$gate/phases.tsv"
 printf 'failed: %s (exit %s)\n' "$current_phase" "$code" > "$gate/status.txt"
 timeout 45s node "$prep/capture-failure.js" "$gate/failure-logs" || true
 exit "$code"
}
trap failure ERR
phase() {
 current_phase="$1"; shift
 phase_start="$(date -u +%FT%TZ)"
 printf 'running: %s\n' "$current_phase" > "$gate/status.txt"
 "$@" > "$gate/$current_phase.log" 2>&1
 printf '%s\t%s\t%s\tcompleted\n' "$current_phase" "$phase_start" "$(date -u +%FT%TZ)" >> "$gate/phases.tsv"
}
readonly previous_gate="$PWD/build/benchmarks/paired/gate-eval67-389ca16-20261021b"
FABRIC_SNAPSHOT="$(cat "$previous_gate/fabric-snapshot-path.txt")"
BASELINE_SNAPSHOT="$(cat "$previous_gate/baseline-snapshot-path.txt")"
printf '%s\n' "$FABRIC_SNAPSHOT" > "$gate/fabric-snapshot-path.txt"
printf '%s\n' "$BASELINE_SNAPSHOT" > "$gate/baseline-snapshot-path.txt"
phase preflight env --chdir=/mnt/c/Users/Nacho node -e 'const root="/home/nacho/pfi/tesis-serra-zarlenga-fabric";const g=require(root+"/benchmarks/caliper/src/host-guard");g.assertCheckout(root,process.argv[1]);g.assertPhysicalSpace(20)' "$experiment_commit"
phase profile ./network/scripts/validate-compose-prerequisites.sh --measurement
phase prior-runtime-logs timeout 45s node "$prep/capture-failure.js" "$gate/prior-runtime-logs"
phase stale-lock node -e 'if(require("node:fs").existsSync("build/benchmarks/paired/.active.lock")) throw new Error("A controller lock remains; inspect owner before proceeding");'
phase snapshot-pair-before node -e 'const p=require("./benchmarks/caliper/src/run-paired-series");p.loadSnapshots(process.cwd(),{fabricSnapshotDirectory:process.argv[1],baselineSnapshotDirectory:process.argv[2]},process.argv[3]);' "$FABRIC_SNAPSHOT" "$BASELINE_SNAPSHOT" "$experiment_commit"
phase stop-baseline node -e 'require("./benchmarks/caliper/src/run-paired-series").stopBaseline(process.cwd())'
phase stop-fabric-before-restore node -e 'require("./benchmarks/caliper/src/run-paired-series").stopFabric(process.cwd())'
for number in 1 2; do
 phase "fabric-restore-$number" npm --prefix benchmarks/caliper run snapshot:restore -- --snapshot-dir "$FABRIC_SNAPSHOT"
 phase "fabric-verify-$number" npm --prefix benchmarks/caliper run snapshot:verify -- --snapshot-dir "$FABRIC_SNAPSHOT" --concurrency 64
done
phase resources ./network/scripts/measure-resources.sh
phase idle-warm-before npm --prefix benchmarks/caliper run warm:peers -- --snapshot-dir "$FABRIC_SNAPSHOT"
phase idle-before node "$prep/idle-state.js" before "$gate/idle-before.json"
phase idle-15-minutes sleep 900
phase idle-warm-after npm --prefix benchmarks/caliper run warm:peers -- --snapshot-dir "$FABRIC_SNAPSHOT"
phase idle-after node "$prep/idle-state.js" after "$gate/idle-before.json" "$gate/idle-after.json"
phase stop-fabric node -e 'require("./benchmarks/caliper/src/run-paired-series").stopFabric(process.cwd())'
for number in 1 2; do
 phase "baseline-restore-$number" node "$prep/verify-baseline-restoration.js" "$BASELINE_SNAPSHOT" "$gate/baseline-restore-$number"
done
phase snapshot-pair node -e 'const p=require("./benchmarks/caliper/src/run-paired-series");p.loadSnapshots(process.cwd(),{fabricSnapshotDirectory:process.argv[1],baselineSnapshotDirectory:process.argv[2]},process.argv[3]);' "$FABRIC_SNAPSHOT" "$BASELINE_SNAPSHOT" "$experiment_commit"
readonly trial_dir="$PWD/build/benchmarks/paired/eval67-389ca16-20261021b-trial"
phase prior-trial-proof node "$prep/verify-prior-trial.js" "$trial_dir" "$FABRIC_SNAPSHOT" "$BASELINE_SNAPSHOT" "$gate/prior-trial-proof.json"
phase prior-trial-analysis npm --prefix benchmarks/analysis run process -- --input "$trial_dir/fabric" --input "$trial_dir/baseline" --snapshot "$FABRIC_SNAPSHOT" --snapshot "$BASELINE_SNAPSHOT" --campaign "$trial_dir/campaign.json" --allow-noncitable --output "$gate/prior-trial-analysis"
phase full env --chdir=/mnt/c/Users/Nacho node "$PWD/benchmarks/caliper/src/run-paired-series.js" --plan full --fabric-snapshot-dir "$FABRIC_SNAPSHOT" --baseline-snapshot-dir "$BASELINE_SNAPSHOT" --dataset-dir build/dataset --campaign-token "$token-full"
readonly full_dir="$PWD/build/benchmarks/paired/$token-full"
phase full-analysis npm --prefix benchmarks/analysis run process -- --input "$full_dir/fabric" --input "$full_dir/baseline" --snapshot "$FABRIC_SNAPSHOT" --snapshot "$BASELINE_SNAPSHOT" --campaign "$full_dir/campaign.json" --output "$gate/full-analysis"
printf 'completed: measurements and strict processing\n' > "$gate/status.txt"
