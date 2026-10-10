'use strict';

const fs = require('node:fs');
const os = require('node:os');

function readOSName() {
    try {
        const contents = fs.readFileSync('/etc/os-release', 'utf8');
        return contents.match(/^PRETTY_NAME=(?:"([^"]+)"|(.*))$/mu)?.slice(1).find(Boolean) ?? os.type();
    } catch {
        return os.type();
    }
}

function buildHostMetadata() {
    const cpu = os.cpus()[0];
    if (!cpu) throw new Error('cannot identify benchmark host CPU');
    const host = {
        cpu: cpu.model.trim(),
        cpuCores: os.cpus().length,
        memoryGB: Number((os.totalmem() / (1024 ** 3)).toFixed(3)),
        os: readOSName(),
        kernel: os.release(),
    };
    if (/microsoft/iu.test(os.release())) {
        host.wsl = process.env.WSL_DISTRO_NAME ? `WSL2 ${process.env.WSL_DISTRO_NAME}` : 'WSL2';
    }
    return host;
}

module.exports = {buildHostMetadata, readOSName};
