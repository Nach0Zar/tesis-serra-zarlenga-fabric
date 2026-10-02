'use strict';

const fs = require('node:fs');

const {WorkloadModuleBase} = require('@hyperledger/caliper-core');
const {buildMetadata, writeJSONAtomic} = require('../src/metadata');
const {IDENTITY_NAME} = require('../src/network-config');

class SmokeReadUnitWorkload extends WorkloadModuleBase {
    constructor() {
        super();
        this.startedAt = undefined;
        this.successfulTransactions = 0;
    }

    async initializeWorkloadModule(workerIndex, totalWorkers, roundIndex, roundArguments, sutAdapter, sutContext) {
        await super.initializeWorkloadModule(
            workerIndex,
            totalWorkers,
            roundIndex,
            roundArguments,
            sutAdapter,
            sutContext,
        );
        if (totalWorkers !== 1 || workerIndex !== 0) {
            throw new Error('Caliper smoke requires exactly one worker');
        }
        for (const field of ['gtin', 'serialNumber', 'runContextPath', 'metadataPath']) {
            if (typeof roundArguments[field] !== 'string' || roundArguments[field] === '') {
                throw new Error(`missing workload argument ${field}`);
            }
        }
        this.startedAt = new Date().toISOString();
    }

    async submitTransaction() {
        const result = await this.sutAdapter.sendRequests({
            contractId: 'snt',
            contractFunction: 'ReadUnit',
            contractArguments: [this.roundArguments.gtin, this.roundArguments.serialNumber],
            readOnly: true,
            invokerIdentity: IDENTITY_NAME,
            invokerMspId: 'LabMSP',
        });
        const status = Array.isArray(result) ? result[0] : result;
        if (!status || status.GetStatus() !== 'success') {
            throw new Error('ReadUnit did not return a successful Caliper transaction status');
        }
        this.successfulTransactions += 1;
    }

    async cleanupWorkloadModule() {
        const endedAt = new Date().toISOString();
        const context = JSON.parse(fs.readFileSync(this.roundArguments.runContextPath, 'utf8'));
        const completed = this.successfulTransactions === context.expectedTransactions;
        const discardReason = completed
            ? undefined
            : `Smoke completed ${this.successfulTransactions} of ${context.expectedTransactions} ReadUnit requests`;
        const metadata = buildMetadata(context, {
            startedAt: this.startedAt,
            endedAt,
            successfulTransactions: this.successfulTransactions,
            discardReason,
        });
        writeJSONAtomic(this.roundArguments.metadataPath, metadata);

        if (!completed) {
            throw new Error(discardReason);
        }
    }
}

function createWorkloadModule() {
    return new SmokeReadUnitWorkload();
}

module.exports.createWorkloadModule = createWorkloadModule;
