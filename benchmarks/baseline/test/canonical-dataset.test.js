'use strict';

const assert = require('node:assert/strict');
const path = require('node:path');
const test = require('node:test');

const {loadDatasetBundle} = require('../../caliper/src/sources');
const {buildGoldenSnapshotPlan, EXPECTED_COUNTS, profileMatrix} = require('../src/snapshot-plan');
const {repoRoot} = require('../src/paths');
const {CANONICAL_DATASET_SHA256} = require('../src/snapshot');
const commonPlan = require('../../caliper/src/snapshot-plan');

test('canonical dataset yields the exact golden snapshot partition', () => {
    const bundle = loadDatasetBundle(path.join(repoRoot, 'build', 'dataset'));
    assert.equal(bundle.manifest.dataset.sha256, CANONICAL_DATASET_SHA256);
    assert.equal(profileMatrix().length, 20);
    const plan = buildGoldenSnapshotPlan(bundle.dataset, bundle.manifest.seed);
    assert.equal(buildGoldenSnapshotPlan, commonPlan.buildGoldenSnapshotPlan);
    assert.deepEqual(plan.counts, EXPECTED_COUNTS);
    assert.equal(plan.fillerSequences.includes(plan.smokeSequence), true);
    assert.equal(plan.excludedSequences.includes(plan.smokeSequence), false);
    assert.equal(plan.preparedSequences.includes(plan.smokeSequence), false);
    assert.equal(plan.counts.registered, 46920);
    assert.equal(plan.units.length, 50000);
    assert.equal(plan.units.filter((unit) => unit.expected.present).length, 46920);
    assert.ok(plan.units.every((unit) => unit.recipe.every((invocation) =>
        invocation.operation !== 'RegisterUnit'
            || JSON.stringify(invocation.targetMspIds) === JSON.stringify([invocation.invokerMspId]))));
});
