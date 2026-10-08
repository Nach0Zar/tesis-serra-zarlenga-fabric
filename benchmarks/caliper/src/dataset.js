'use strict';

function selectLabRegistration(dataset) {
    if (!Array.isArray(dataset?.units)) {
        throw new Error('dataset must contain an units array');
    }

    for (const unit of dataset.units) {
        if (!Array.isArray(unit.preparation)) {
            continue;
        }
        const invocation = unit.preparation.find((step) =>
            step?.operation === 'RegisterUnit' && step?.invokerMspId === 'LabMSP');
        if (!invocation) {
            continue;
        }
        const request = invocation.request;
        if (!request || !request.gtin || !request.numeroSerie || !request.lote || !request.fechaVencimiento) {
            throw new Error('selected LabMSP RegisterUnit recipe is incomplete');
        }
        return {
            sequence: unit.sequence,
            operation: invocation.operation,
            invokerMspId: invocation.invokerMspId,
            request: {
                gtin: request.gtin,
                numeroSerie: request.numeroSerie,
                lote: request.lote,
                fechaVencimiento: request.fechaVencimiento,
            },
        };
    }

    throw new Error('dataset has no LabMSP RegisterUnit preparation recipe');
}

module.exports = {selectLabRegistration};
