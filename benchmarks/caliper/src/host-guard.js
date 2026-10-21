'use strict';
const path = require('node:path');
const {spawnSync} = require('node:child_process');

function command(program, args, cwd) {
    const result = spawnSync(program, args, {cwd, encoding: 'utf8', maxBuffer: 1024 * 1024});
    if (result.error) throw result.error;
    if (result.status !== 0) throw new Error(program + ' guard failed: ' + (result.stderr || result.stdout).trim());
    return result.stdout.trim();
}
function assertPhysicalSpace(minimumGiB) {
    const drive = process.env.SNT_EXPERIMENT_PHYSICAL_DRIVE ?? 'C';
    if (!/^[A-Z]$/u.test(drive)) throw new Error('physical drive must be one uppercase drive letter');
    const free = Number(command('/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe',
        ['-NoProfile', '-NonInteractive', '-Command',
            "[System.IO.DriveInfo]::new('" + drive + ":\\').AvailableFreeSpace"]));
    if (!Number.isFinite(free) || free < minimumGiB * (1024 ** 3)) throw new Error('insufficient verified physical disk space');
    return free;
}
function assertCheckout(root, commit) {
    if (command('git', ['rev-parse', 'HEAD'], root) !== commit
        || command('git', ['status', '--porcelain'], root)) throw new Error('experimental checkout changed or is dirty');
}
function guardAttempt(root, commit) {
    assertCheckout(root, commit);
    command(path.join(root, 'network/scripts/validate-compose-prerequisites.sh'), ['--measurement'], root);
    return assertPhysicalSpace(20);
}
module.exports = {assertCheckout, assertPhysicalSpace, command, guardAttempt};
