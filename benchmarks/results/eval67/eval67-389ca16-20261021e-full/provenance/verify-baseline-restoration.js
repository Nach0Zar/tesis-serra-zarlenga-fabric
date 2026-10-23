'use strict';
const fs = require('node:fs');
const path = require('node:path');
const root = process.cwd();
const load = (name) => require(path.join(root, name));
const {restoreSnapshot, verifyAllUnits} = load('benchmarks/baseline/src/snapshot');
const {readPreconditions} = load('benchmarks/baseline/src/preconditions');
const {loadDatasetBundle, readSourceTruth} = load('benchmarks/caliper/src/sources');
const {resolveCredentials} = load('benchmarks/baseline/src/credentials');
const {BaselineClient} = load('benchmarks/baseline/src/http-client');
const {executeSmoke} = load('benchmarks/baseline/src/runner');
const {compose} = load('benchmarks/baseline/src/runtime');
(async () => {
 const directory = path.resolve(process.argv[3]);
 fs.mkdirSync(directory, {recursive:false});
 let restored;
 try {
  restored = restoreSnapshot(process.argv[2]);
  const bundle = loadDatasetBundle(path.join(root,'build/dataset'));
  const sourceTruth = readSourceTruth(root);
  const credentials = resolveCredentials(root, [], restored.environment);
  const client = new BaselineClient({baseURL:restored.apiURL,credentials});
  const verified = await verifyAllUnits(client,bundle.dataset,readPreconditions(restored),8);
  if(verified.present!==46920 || verified.absent!==3080) throw new Error('canonical restore counts mismatch');
  fs.writeFileSync(path.join(directory,'semantic-verification.json'),JSON.stringify(verified,null,2)+'\n',{mode:0o600});
  await executeSmoke({bundle,restored,sourceTruth,credentials,outputDirectory:path.join(directory,'smoke')});
 } catch(error) {
  if(restored) {
   const logs=compose(root,['logs','--no-color'],{env:restored.environment,allowFailure:true});
   fs.writeFileSync(path.join(directory,'sut.log'),String(logs.stdout??'')+String(logs.stderr??''),{mode:0o600});
  }
  throw error;
 } finally {
  if(restored) compose(root,['down','--volumes','--remove-orphans'],{env:restored.environment,allowFailure:true});
 }
})().catch(error=>{process.stderr.write(error.message+'\n');process.exitCode=1;});
