'use strict';

const assert = require('node:assert/strict');
const test = require('node:test');

const {parseCredentials, publicCredentialConfiguration} = require('../src/credentials');

const manifest = {organizations: [
    {mspId: 'LabMSP', active: true, clientRole: 'operator'},
    {mspId: 'FarmaciaMSP', active: true, clientRole: 'operator'},
]};

test('API keys are validated by organization and never enter public configuration', () => {
    const credentials = parseCredentials(JSON.stringify([
        {mspId: 'LabMSP', role: 'operator', key: 'lab-secret'},
        {mspId: 'FarmaciaMSP', role: 'operator', key: 'pharmacy-secret'},
    ]), manifest, ['LabMSP', 'FarmaciaMSP']);
    assert.deepEqual(publicCredentialConfiguration(credentials), [
        {mspId: 'LabMSP', role: 'operator'},
        {mspId: 'FarmaciaMSP', role: 'operator'},
    ]);
    assert.doesNotMatch(JSON.stringify(publicCredentialConfiguration(credentials)), /secret/u);
});

test('credential parsing rejects missing, duplicate, inactive and role-mismatched entries', () => {
    assert.throws(() => parseCredentials('[]', manifest, ['LabMSP']), /missing/u);
    assert.throws(() => parseCredentials(JSON.stringify([
        {mspId: 'LabMSP', role: 'operator', key: 'one'},
        {mspId: 'LabMSP', role: 'operator', key: 'two'},
    ]), manifest), /duplicate/u);
    assert.throws(() => parseCredentials(JSON.stringify([
        {mspId: 'LabMSP', role: 'regulatory-admin', key: 'one'},
    ]), manifest), /invalid/u);
});
