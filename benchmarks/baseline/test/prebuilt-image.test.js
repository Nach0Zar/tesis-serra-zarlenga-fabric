'use strict';
const assert = require('node:assert/strict');
const test = require('node:test');
const {snapshotBuildFlag} = require('../src/snapshot');
test('prebuilt snapshots require the exact frozen image and retain historical build default', () => {
    const expected = 'sha256:' + 'a'.repeat(64);
    assert.equal(snapshotBuildFlag({}, undefined), '--build');
    assert.equal(snapshotBuildFlag({SNT_BASELINE_PREBUILT_IMAGE_ID: expected}, expected), '--no-build');
    assert.throws(() => snapshotBuildFlag({SNT_BASELINE_PREBUILT_IMAGE_ID: expected}, 'sha256:' + 'b'.repeat(64)));
    assert.throws(() => snapshotBuildFlag({SNT_BASELINE_PREBUILT_IMAGE_ID: 'latest'}, 'latest'));
});
