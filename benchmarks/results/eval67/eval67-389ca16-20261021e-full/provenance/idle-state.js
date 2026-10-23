'use strict';
const fs=require('node:fs');
const {spawnSync}=require('node:child_process');
const root=process.cwd();
const {CHAINCODE_CONTAINER}=require(root+'/benchmarks/caliper/src/snapshot-volumes');
function run(args) {
 const result=spawnSync('docker',args,{encoding:'utf8',maxBuffer:64*1024*1024});
 if(result.status!==0) throw new Error(result.stderr||'docker failed');
 return result.stdout;
}
const state=run(['ps','--format','{{.ID}} {{.Names}}']).split('\n').filter(Boolean)
 .flatMap(line=>{const [id,name]=line.split(' '); return name.endsWith('.snt.local')||CHAINCODE_CONTAINER.test(name)?[{id,name}]:[];})
 .map(({id,name})=>{const value=JSON.parse(run(['inspect',id]))[0];return {id,name,restarts:value.RestartCount,
  status:value.State.Status,oom:value.State.OOMKilled};});
if(process.argv[2]==='before') {
 fs.writeFileSync(process.argv[3],JSON.stringify({at:new Date().toISOString(),state},null,2)+'\n',{mode:0o600});
} else {
 const previous=JSON.parse(fs.readFileSync(process.argv[3],'utf8'));
 if(JSON.stringify(state)!==JSON.stringify(previous.state)) throw new Error('runtime changed during idle qualification');
 for(const container of state) {
  if(container.oom||container.status!=='running') throw new Error('unexpected runtime state');
  const logs=spawnSync('docker',['logs','--since',previous.at,container.id],{encoding:'utf8',maxBuffer:64*1024*1024});
  if(/too_many_pings/iu.test(String(logs.stdout??'')+String(logs.stderr??''))) throw new Error('transport failure during idle');
 }
 fs.writeFileSync(process.argv[4],JSON.stringify({at:new Date().toISOString(),state,unchanged:true},null,2)+'\n',{mode:0o600});
}
