"use strict";
const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const [trial,fabric,baseline,out]=process.argv.slice(2);
const root=process.cwd();
const sha=(p)=>crypto.createHash('sha256').update(fs.readFileSync(p)).digest('hex');
const campaignFile=path.join(trial,'campaign.json');
const c=JSON.parse(fs.readFileSync(campaignFile));
if(c.plan!=='trial'||c.status!=='completed'||c.repositoryCommit!=='389ca160b2a490cd66bb0194f012e2b75b3598e8'||c.attempts.length!==18||c.attempts.some(a=>a.status!=='completed'))throw new Error('Previous trial not complete for frozen artifact');
for(const [sut,dir] of [['fabric',fabric],['baseline',baseline]]){
 if(sha(path.join(dir,'manifest.json'))!==c.snapshots[sut].manifestSHA256)throw new Error('Previous trial used another snapshot');
 if(path.resolve(root,c.snapshots[sut].directory)!==path.resolve(dir))throw new Error('Snapshot location mismatch');
}
fs.writeFileSync(out,JSON.stringify({checkedAt:new Date().toISOString(),experimentalCommit:c.repositoryCommit,trialDirectory:path.relative(root,trial),campaignSHA256:sha(campaignFile),snapshots:c.snapshots,attempts:18,nonCitable:true,reason:'User approved retaining complete prior trial for identical commit and snapshots; interrupted repeated trial retained separately. Both snapshots requalified after Docker recovery.'},null,2)+'\n',{mode:0o600});
console.log('Previous complete trial verified for the same immutable snapshots and experimental commit');
