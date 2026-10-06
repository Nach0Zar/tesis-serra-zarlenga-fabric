'use strict';

const timers = require('node:timers/promises');

class ConceptualFixedRate {
    constructor(testMessage, stats, _workerIndex, clock = {}) {
        const operationsPerSecond = Number(testMessage.getRateControlSpec().opts?.operationsPerSecond);
        const workers = testMessage.getWorkersNumber();
        if (!Number.isFinite(operationsPerSecond) || operationsPerSecond <= 0) {
            throw new Error('conceptual fixed rate requires a positive operationsPerSecond');
        }
        if (!Number.isInteger(workers) || workers <= 0) {
            throw new Error('conceptual fixed rate requires a positive worker count');
        }
        this.stats = stats;
        this.intervalMs = 1000 / (operationsPerSecond / workers);
        this.operationCount = 0;
        this.now = clock.now ?? Date.now;
        this.sleep = clock.sleep ?? timers.setTimeout;
    }

    async applyRateControl() {
        const elapsedMs = this.now() - this.stats.getRoundStartTime();
        const delayMs = Math.max(0, this.intervalMs * this.operationCount - elapsedMs);
        if (delayMs > 0) await this.sleep(delayMs);
        this.operationCount += 1;
    }

    async end() {}
}

function createRateController(testMessage, stats, workerIndex) {
    return new ConceptualFixedRate(testMessage, stats, workerIndex);
}

module.exports = {ConceptualFixedRate, createRateController};
