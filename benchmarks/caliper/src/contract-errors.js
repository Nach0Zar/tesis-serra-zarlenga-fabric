'use strict';

function jsonObjects(text) {
    const values = [];
    for (let start = 0; start < text.length; start += 1) {
        if (text[start] !== '{') continue;
        let depth = 0;
        let quoted = false;
        let escaped = false;
        for (let end = start; end < text.length; end += 1) {
            const character = text[end];
            if (quoted) {
                if (escaped) escaped = false;
                else if (character === '\\') escaped = true;
                else if (character === '"') quoted = false;
                continue;
            }
            if (character === '"') quoted = true;
            else if (character === '{') depth += 1;
            else if (character === '}') {
                depth -= 1;
                if (depth === 0) {
                    try {
                        values.push(JSON.parse(text.slice(start, end + 1)));
                    } catch {
                        // Fabric may prefix arbitrary text before the next JSON object.
                    }
                    break;
                }
            }
        }
    }
    return values;
}

function extractContractError(error) {
    if (!error) return undefined;
    const texts = [error.message, error.toString?.()];
    if (Array.isArray(error.details)) {
        for (const detail of error.details) texts.push(detail?.message);
    }
    for (const text of texts.filter((value) => typeof value === 'string')) {
        for (const candidate of jsonObjects(text)) {
            if (typeof candidate.code === 'string' && candidate.code !== ''
                && typeof candidate.message === 'string' && candidate.message !== '') {
                return candidate;
            }
        }
    }
    return undefined;
}

function isPrivateDataNotDisseminated(envelope) {
    return envelope?.code === 'INTERNAL_ERROR'
        && envelope.details?.reintentable === true
        && envelope.details?.causa === 'PRIVATE_DATA_NOT_DISSEMINATED';
}

module.exports = {extractContractError, isPrivateDataNotDisseminated, jsonObjects};
