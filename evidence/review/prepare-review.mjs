import fs from 'node:fs';
import path from 'node:path';
import {buildVerifyRubricYaml,validateVerifyRubricYaml,validateArtifactResultDetail} from 'file:///F:/swe/delivery-package.mjs';
const root='F:/swe/099934/project-01',out=root+'/evidence/review';
const q=JSON.parse(fs.readFileSync(out+'/question-initial.json','utf8'));
const current=(await (await fetch('http://127.0.0.1:5174/api/questions')).json()).find(x=>x.id===q.id);
if(current.syncRevision!==q.syncRevision)throw Error('syncRevision changed: reread code, trace, question before evaluation');
if(q.rounds!==123||q.harness!=='Trae'||q.effectiveRoundsEvidence.source!=='count-trae-cn-steps.py'||q.effectiveRoundsEvidence.effectiveRounds!==q.rounds||q.sessionId!==q.effectiveRoundsEvidence.sessionId)throw Error('scan evidence mismatch');
const log=fs.readFileSync(out+'/runtime.log','utf8'),cli=fs.readFileSync(out+'/cli-final.log','utf8');
if((log.match(/CANCEL source=.*reads=2 error="context canceled" stdout="" stderr=""/g)||[]).length!==3)throw Error('cancel evidence missing');
if((log.match(/DEFAULT_CANCEL_RETRY snapshots=2 check=PASS/g)||[]).length!==3||!cli.includes('RESTORE_BYTE_AND_SYMLINK_COMPARISON PASS')||!cli.includes('CORRUPT_ENCRYPTED_PACK_REJECTED PASS'))throw Error('runtime evidence missing');
const reason='验证阶段第二次读取源快照后取消仅返回 context canceled 且未输出快照和路径';
const rubricResults=q.rubric.map(r=>({id:r.id,passed:r.id!==5,reason:r.id===5?reason:''}));
const result=rubricResults.map(r=>`${r.id} ${r.passed?'通过':`未通过 ${r.reason}`}`).join('\n');
const validation=validateArtifactResultDetail({...q,rubricResults,runResult:result,artifactResult:result});
if(!validation.ok)throw Error(JSON.stringify(validation));
const yaml=buildVerifyRubricYaml(q.rubric),yv=validateVerifyRubricYaml(yaml,q.rubric);
if(!yv.ok)throw Error(JSON.stringify(yv));
fs.writeFileSync(out+'/verify-rubric.yaml',yaml);
fs.writeFileSync(out+'/format-validation.json',JSON.stringify({yamlParsed:true,yaml:yv,result:validation,pageAndPayloadBuilder:'server.mjs:826 -> buildVerifyRubricYaml(question.rubric); delivery-package.mjs:386,466'},null,2)+'\n');
const observations=[
 '零候选被拒绝；两个已保存且 Original 相同的目标快照被拒绝，并列出源与两个目标 ID。',
 '15 组节点差异均返回对应字段和 /file；递归验证逐层比较子树与数据序列。',
 '目标加密 pack 被改写后退出 1，报具体快照、文件路径及 ciphertext verification failed，未打印 verified。',
 '新保存和已跳过快照分别输出 verified 1 snapshots；保存成功后删除 pack 返回快照与根路径错误，未输出成功汇总。',
 '已复制源快照 6f48c3c1 的第二次真实读取成功后，审核 barrier 立即取消父 context。该次读取已提供完整源快照，verifySnapshots 随后在检查 ctx.Err 时直接返回。返回值严格等于 context canceled，捕获的 stdout 和 stderr 均为空，没有具体快照 ID 或路径。相同交错执行三次，三次得到相同返回与空输出；双仓库真实锁文件均为 0。',
 '默认关闭验证，筛选和跳过不输出 verified；首个快照保存后取消，重跑得到 2 个快照且 check 通过。',
 '不同密钥的普通本地仓库通过 copy、check --read-data、restore；恢复文件 SHA256 和符号链接目标均一致。'
];
const direct=[
 'cmd_copy.go:442；TestFindDestinationSnapshot 与 TestReviewPersistentDuplicate 通过，runtime.log:321。',
 'cmd_copy.go:477、553、582；TestCompareNodes 15 子项全部通过，runtime.log:237。',
 'cmd_copy.go:493、666；cli-final.log:92-95；缺失 blob 与明文哈希篡改专项通过。',
 'cmd_copy.go:187、391；cli-final.log:60、64；runtime.log:303 的 POST_SAVE_LOSS 原始错误。',
 'runtime.log:286、356、426 逐次记录 reads=2 error="context canceled" stdout="" stderr=""；cmd_copy.go:401-408 的 callback 丢弃快照标识并直接返回 ctx.Err；review_contract_test.go.txt:51-83 控制真实源 backend.Load 返回后的取消时点。',
 'cmd_copy.go:78、180、187；原 TestCopy/Incremental 全通过；runtime.log:340 的 DEFAULT_CANCEL_RETRY。',
 'cli-final.log:69-83、95；TestCopy 真实目录恢复比较通过；隔离公网、UID 65534，无容器或外部服务。'
];
const impact='启用 --verify 的复制已进入指定源快照的验证阶段，用户只收到取消错误，不能从命令输出定位该次取消所对应的快照与验证路径。命令确实返回失败且双锁确实释放；未通过范围仅为冻结要求中的错误定位信息。';
const report=q.rubric.map((r,i)=>`Rubric ID：${r.id}｜状态：${r.id===5?'未通过':'通过'}｜期望行为：${r.text}｜实际观察：${observations[i]}｜直接证据：${direct[i]}${r.id===5?`｜原因：${reason}｜影响：${impact}｜证据位置：evidence/review/runtime.log:286、356、426；cmd/restic/cmd_copy.go:401-408；evidence/review/review_contract_test.go.txt:51-83。`:''}`);
const header=`restic-8c201f64-9e29-47e5-988e-c76548a3324d | project-01 | 交付复盘\n完成度 部分完成 | 有效轮数 123（仅核对扫描）| 收录 长程题 | Session 2478 秒\n`;
const raw=log.split(/\r?\n/).filter(l=>l.includes('CANCEL source=')).map(l=>l.trim());
const footer='\n关键原始证据（runtime.log；取消交错 -count=3）\n'+raw.join('\n')+'\n代码 cmd_copy.go:401-408\n  FindAll(..., func(_ string, sn *data.Snapshot, err error) error {\n    if err != nil { return err }\n    if err := ctx.Err(); err != nil { return err }\n  // 此处在 findDestinationSnapshot、verifyTree 与 snapshot 错误包装之前返回\n验证结果：原专项 9 项及 15 个元数据子项通过；4 个审核交错各执行 3 次；check/restore 通过。\n取消缺陷复现用例的 PASS 表示上述缺口已被稳定观察，不表示 Rubric 5 通过。\n';
fs.writeFileSync(out+'/rubric-comparison.txt',header+'\n'+report.join('\n\n')+'\n'+footer);
fs.writeFileSync(out+'/rubric-report.json',JSON.stringify({questionId:q.id,completionStatus:'部分完成',rubricResults,report},null,2)+'\n');
fs.copyFileSync(q.traceFile,root+'/evidence/trajectory.md');fs.copyFileSync(q.traceMetadataFile,out+'/trajectory.metadata.json');
const duration=JSON.parse(fs.readFileSync(out+'/session-duration.json','utf8'));
const materials="已沿复制命令审计快照筛选、双仓库锁、批量上传等待、快照持久化和递归验证边界。使用声明的离线材料，在空缓存、阻断公网和普通用户权限下完成真实本地仓库验证。原有专项及元数据差异测试全部通过，取消、保存后数据丢失、重复对应和默认模式中断重跑四类受控交错各执行三次。跨密钥复制后检查完整数据并恢复文件，恢复内容与符号链接均一致。验证阶段取消仅返回取消错误且没有快照和路径，已稳定复现并据此判定第五项未通过；其余六项通过。项目证据目录保存完整单次轨迹、基线补丁、逐条报告、唯一终端证据图、原始验证日志与时长来源。审核用例通过叠加文件执行，没有修改模型生产实现。冻结评分清单已经实际解析并核对顺序、文本和类型。";
const evaluation={id:q.id,expectedSyncRevision:q.syncRevision,rounds:q.rounds,harness:q.harness,completionStatus:'部分完成',collectionDecision:'收录：长程题',rubricResults,runResult:result,artifactResult:result,artifactMaterials:materials,modelPatchFile:root+'/evidence/model.patch',screenshotsDirectory:root+'/evidence/screenshots',exceededTwoHours:duration.exceededTwoHours,sessionStartedAt:duration.sessionStartedAt,sessionCompletedAt:duration.sessionCompletedAt,sessionDurationSeconds:duration.sessionDurationSeconds,sessionDurationSource:duration.sessionDurationSource,sessionDurationEvidence:duration.evidence};
fs.writeFileSync(out+'/evaluation.json',JSON.stringify(evaluation,null,2)+'\n');
console.log(JSON.stringify({revision:q.syncRevision,completion:evaluation.completionStatus,results:result,yamlParsed:yv.ok}));
