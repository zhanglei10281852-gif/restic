# restic copy --verify 代码与运行审计

题目 restic-8c201f64-9e29-47e5-988e-c76548a3324d；项目 F:\swe\099934\project-01。
唯一基线 c734245fd3ae9bcac8ab3aaafb4482d4b5d0ae92；冻结 Rubric 7 条，冻结时间 2026-11-09T09:45:00+08:00。
真实模型轨迹见 evidence/trajectory.md；新增 review 测试只是审核原最终代码，不修改生产实现。

## 入口、状态所有者与顺序

- `cmd/restic/cmd_copy.go:26-78` Cobra copy 入口绑定默认 false 的 Verify；Finalize 后调用 runCopy。
- `runCopy:126-202` 解析双仓库方向后，依次取得源 read lock、目标 append lock，注册两个 defer；先缓存 snapshot 列表、加载两个索引，再建立目标持久身份映射。
- `collectAllSnapshots:84-123` 迭代原 SnapshotFilter.FindAll，依 Original/self ID 与 similarSnapshots 跳过已复制对象；过滤条件与选择错误仍沿原 iterator 交给复制入口。
- `copyTreeBatched:227-295` 所有树 repack 完成后等待 WithBlobUploader 返回，才逐项 SaveSnapshot；原始内容由 Repository 与真实本地后端持有，savedSnapshotIDs 只保存新快照 ID。
- `internal/repository/repository.go:571-600` WithBlobUploader 使用 errgroup 管理 pack uploader、blob saver 和 flush，返回 wg.Wait；没有在保存快照前遗留异步上传的新增边界。
- `copySaveSnapshot:372-386` 清除 Parent、保留已有 Original 或设为源 ID，实际 SaveSnapshot 后返回 ID。失败直接上抛；成功对象不回滚，供中断重跑跳过。
- `runCopy:187-199` 仅开启 Verify 时重读新保存 snapshot 并放入同一目标映射，随后第二次选择所有源快照，覆盖跳过与新保存两类。
- `findDestinationSnapshot:442-470` 以持久身份查候选，再按 similarSnapshots 筛选；零个与多个均错误，不择一放行。审核测试保存 ProgramVersion 不同的第二个真实 snapshot，验证两个不同 ID 被拒绝。
- `verifyTree:477-548` 查目标索引，读取目标 blob，校验 plaintext hash，解析目标树并读源树；DualTreeIterator 比较双树节点，错误加入 path。完成树后才加入本次 verifiedBlobs 缓存。
- `verifyNode/compareNodes:553-660` 比较 name/type/mode/size、三时间、所有用户/设备/链接字段、content 长度和每个 ID、xattr/generic attributes、subtree；逐项读取 data blob，并递归目录。共享 blob 仅在校验完成后复用。
- `verifyDataBlob:666-690` LookupBlobSize + LoadBlob（实际解密/解压）+ restic.Hash；缺失/读取错误/hash 错误携带节点 path 并上抛 snapshot 包装。
- `verifySnapshots:391-426` 单快照完整验证后输出 verified，全部成功才输出总数。错误直接退出，不打印最终成功汇总。

## 取消与失败交错

1. 源快照第二次实际读取成功后取消父 context。hook 只代理真实 backend.Load，目标已在先前真实 copy 中保存。`verifySnapshots:401-408` 丢弃 callback 的 snapshot 标识，并直接返回 ctx.Err。实测三次均仅 `context canceled`，stdout/stderr 为空；这违反 Rubric 5 的快照/路径错误上下文要求。没有把非零退出或锁释放误判为失败。
2. 目标真实 SnapshotFile Save 成功后，删除同仓库真实 data pack。此时 WithBlobUploader 已等待完成，保存成功后读取目标 tree 失败；错误含 snapshot 与 path `/`，没有 verified 成功输出。对应生产条件是保存与读取间的文件系统数据丢失。
3. 默认 Verify=false，在首个真实快照保存成功后取消；返回 cancellation，释放双锁；关闭 hook 重跑后目标为两个快照且 check 成功。验证中断后重跑的原契约。
4. 两个已持久化目标快照保留相同 Original/Tree/筛选字段，ProgramVersion 不同使磁盘 ID 不同；开启 Verify 时拒绝重复映射，并指出源和两个目标 ID。

## 资源停止与清理

`copyTreeBatched` defer stop 结束 pull iterator；WithBlobUploader 等待全部 goroutine。runCopy 两个 defer 逆序释放锁。
`internal/repository/lock.go` Unlock 取消刷新并等待 refreshWG；refreshLocks defer 删除锁、停止 ticker、Done。`lock_file.go:233` 使用 delayedCancelContext 执行已取消后的锁删除。
可控失败与取消测试均检查真实锁文件数为 0；fixture 的 `tmp12345` 是 cmd_init_integration_test.go:26-29 主动写入的垃圾文件，按原 backend 规则忽略，未把它算作锁。
本题不涉及服务订阅/重建、共享多接收者准入或异步更新覆盖，故不引入这些额外义务。

## 环境适用性

核心入口为本地目录 backend 的 init/backup/copy/check/restore 和 cmd/restic copy 集成测试；fixture 创建临时目录、文件与 symlink，调用真实 Repository。没有 testcontainers、服务容器、daemon/socket、挂载或云账号路径。
依据冻结 Rubric 7 与甲方普通本地文件支持范围，使用 Linux UID 65534，无额外服务或特殊挂载权限。WSL 仅承载 Linux 进程，发行版名称不表示调用 Docker；执行命令为 python3、unshare、Go、restic。
`verify-offline.py` 校验声明预置材料 manifest 的 301 个 SHA256，建立空 HOME/Go module/build/runtime cache，用 file GOPROXY、GOSUMDB=off、GOTOOLCHAIN=local；unshare -n 隔离后 socket 连接 1.1.1.1:443 返回 Network unreachable。相关测试 -count=1，故障时序测试 -count=3；真实 CLI check --read-data 和 restore 后逐文件 SHA256/symlink 比对。
初次 build 的 VCS stamping 身份错误通过 -buildvcs=false 修复于验证命令；初次审核断言误计 fixture 垃圾文件和 CLI 日志 Path 序列化问题已修正。历史探测日志仅保留审计，不作为模型失败证据。

## 基础记录

核对当前 SQLite/API syncRevision=5；rounds=123、Harness=Trae、effectiveRoundsEvidence=123，source=count-trae-cn-steps.py，scope=latest_turn，完整 Session ID 与当前轨迹一致，warnings=[]。未执行轮数统计器，未覆盖扫描值。
按当前完整 Session ID 运行 measure-trae-session-duration.mjs，2478 秒，renderer.log 第 683/5220 行分别为开始/完成，超过两小时=否；该值不参与完成度与收录计算。
已有 artifactRepo 经 GitHub API 确认 fork=true、parent=restic/restic、当前授权身份 admin/push=true；只向该 Fork 推送本题产物，不创建仓库或最终交付记录。
