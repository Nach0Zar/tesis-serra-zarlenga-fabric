'use strict';

const assert = require('node:assert/strict');
const path = require('node:path');
const test = require('node:test');

const {loadDatasetBundle} = require('../../caliper/src/sources');
const {buildGoldenSnapshotPlan, EXPECTED_COUNTS, profileMatrix} = require('../src/snapshot-plan');
const {repoRoot} = require('../src/paths');
const {CANONICAL_DATASET_SHA256} = require('../src/snapshot');

test('canonical dataset yields the exact golden snapshot partition', () => {
    const bundle = loadDatasetBundle(path.join(repoRoot, 'build', 'dataset'));
    assert.equal(bundle.manifest.dataset.sha256, CANONICAL_DATASET_SHA256);
    assert.equal(profileMatrix().length, 20);
    const plan = buildGoldenSnapshotPlan(bundle.dataset, bundle.manifest.seed);
    assert.deepEqual(plan.counts, EXPECTED_COUNTS);
    assert.equal(plan.fillerSequences.includes(plan.smokeSequence), true);
    assert.equal(plan.excludedSequences.includes(plan.smokeSequence), false);
    assert.equal(plan.preparedSequences.includes(plan.smokeSequence), false);
});
