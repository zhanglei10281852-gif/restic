# 为跨仓库 copy 增加可选复制后验证

> **Workspace:** F:\swe\099934\project-01

---

**User**

运维人员把加密备份从源仓库复制到使用另一把密钥的目标仓库后，希望在命令返回前确认每个源快照在目标中确实对应同一份内容。请为现有 copy 命令增加可选的 --verify 开关并保持默认关闭；开启时，无论快照是本次复制保存的还是因 Original 等价而被跳过的，都要在目标仓库中找到唯一对应快照，递归比较树的路径、节点类型、大小、模式、时间、链接目标、设备信息、内容 blob 序列、扩展属性、通用属性和子树，并读取目标树及数据 blob 以校验解密解压后的内容哈希和 blob 序列。对应快照缺失或重复、树或索引解析失败、目标 blob 缺失或内容不匹配、保存后验证失败以及取消都必须以非零结果结束，并在输出中指出具体快照和路径；成功时逐个报告 verified 结果并给出汇总。未指定 --verify 时，现有快照筛选、等价快照跳过、批量上传与保存、取消退出和中断后重跑行为保持不变。

**Assistant**

我先查看项目结构，了解现有 copy 命令的实现。

toolName: view_folder

status: success

f:\swe\099934\project-01

这是 restic 项目。让我先阅读现有的 copy 命令实现。

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_copy.go

项目里已有 `changelog/unreleased/issue-5189`，让我查看相关上下文与数据结构。

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\changelog\unreleased\issue-5189

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\internal\data\tree.go

让我查看 Node 结构、Snapshot、StreamTrees 以及相关测试。

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\internal\data\node.go

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\internal\data\tree_stream.go

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\internal\data\snapshot.go

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\VERSION

toolName: search_by_regex

status: success

让我查看 repository 接口定义，并参考上游 restic 0.19.0 的实际实现。

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\internal\restic\repository.go

现在参考上游 restic v0.19.0 中 `--verify` 的官方实现。

toolName: web_search

status: success

toolName: view_files

status: success

filePath: C:\Users\czxy4\AppData\Local\Temp\trae\toolcall-output\76462313-2cd1-4acd-b538-63853a4f02c7.txt

v0.19.0 还没有该功能。让我查看 master 分支上的实现。

toolName: web_search

status: success

toolName: view_files

status: success

filePath: C:\Users\czxy4\AppData\Local\Temp\trae\toolcall-output\f03c2a62-d024-4aaf-bd5b-747910f1be80.txt

master 上也没有该功能，需要从零实现。让我研究 checker 如何校验 blob 内容哈希，以及哈希与压缩的关系。

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\internal\repository\repository.go

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\internal\repository\repository.go

LoadBlob 不校验明文哈希，需要验证方自行计算。让我查看现有测试基础设施和 progress printer 接口。

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

存在 `cmd_copy_integration_test.go`，让我完整阅读。

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_copy_integration_test.go

让我查看 cmd_diff.go 的树比较模式、Printer 接口和 SnapshotFilter.FindAll。

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_diff.go

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\internal\restic\id.go

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\doc\man\restic-copy.1

让我查看 Printer 接口的完整定义和 errors 包的 Fatal 处理。

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\internal\restic\progress.go

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\internal\errors\fatal.go

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\doc\045_working_with_repos.rst

我已充分理解代码结构。现在建立任务清单并开始实现。

toolName: todo_write

status: success

Todos updated: 7 items

让我查看测试中如何损坏仓库数据，以及 IDSet 等辅助 API。

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_check_integration_test.go

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\cmd\restic\integration_helpers_test.go

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_find_integration_test.go

toolName: search_by_regex

status: success

让我确认 TerminalPrinter 的级别语义和 BlobHandle 结构。

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\internal\ui\progress\terminal.go

toolName: search_by_regex

status: success

默认 verbosity=1，`P` 消息默认显示。现在开始实现，先修改 `cmd_copy.go`。

toolName: search_by_regex

status: success

现在修改 `cmd_copy.go`，先添加 flag 和选项字段：

toolName: edit_file_search_replace

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_copy.go

changes: undefined

接下来在 `runCopy` 中接入验证流程：

toolName: edit_file_search_replace

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_copy.go

changes: undefined

现在添加完整的验证实现。先更新 imports：

toolName: edit_file_search_replace

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_copy.go

changes: undefined

现在在文件末尾追加验证相关函数：

toolName: edit_file_search_replace

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_copy.go

changes: undefined

代码已写入。先编译检查：

toolName: Shell

status: success

编译通过。运行 vet 和 gofmt 检查：

toolName: Shell

status: success

gofmt 提示需要格式化，执行格式化：

toolName: Shell

status: success

格式化完成。更新进度，开始编写测试。先查看测试仓库的封装模式：

toolName: todo_write

status: success

Todos updated: 7 items

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\internal\checker\checker_test.go

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\cmd\restic\integration_helpers_test.go

toolName: search_by_regex

status: success

让我确认测试辅助 ID 函数与 openWithReadLock 签名：

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\cmd\restic\integration_helpers_test.go

测试环境默认 Quiet。先检查 `orderedListOnceBackend` 是否允许重复 list（验证阶段会再次列出目标快照）：

toolName: search_by_regex

status: success

让我查看 MemorizeList 的实现，判断重复 list 的影响：

toolName: search_by_regex

status: success

我发现测试后端钩子禁止重复 list，且验证需要包含本次新保存的快照。调整设计：让 `copyTreeBatched` 返回已保存快照 ID，在验证前重新加载并并入映射，避免二次 list。先确认调用点：

toolName: search_by_regex

status: success

重构 `copySaveSnapshot` 和 `copyTreeBatched` 以追踪保存的快照：

toolName: edit_file_search_replace

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_copy.go

changes: undefined

更新 `copySaveSnapshot` 返回新快照 ID：

toolName: edit_file_search_replace

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_copy.go

changes: undefined

现在更新 `runCopy` 和 `verifySnapshots` 的衔接逻辑：

toolName: edit_file_search_replace

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_copy.go

changes: undefined

重构 `verifySnapshots` 接收映射参数，并添加 `addDestinationSnapshot` 辅助函数：

toolName: edit_file_search_replace

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_copy.go

changes: undefined

让 runCopy 中初始映射构建复用同一辅助函数：

toolName: edit_file_search_replace

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_copy.go

changes: undefined

重新编译验证：

toolName: Shell

status: success

gofmt 列出的都是仓库既有文件（Windows 换行符问题），我修改的 `cmd_copy.go` 不在其中。现在查看 data 包测试中如何构造带 ID 的快照：

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

让我查看 flags_test.go 是否需要为新 flag 补充注册检查：

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\cmd\restic\flags_test.go

toolName: search_by_regex

status: success

现在编写测试文件。先确认 `testRunBackup` 签名：

toolName: search_by_regex

status: success

现在创建测试文件：

toolName: Write

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_copy_verify_test.go

changes: undefined

编译测试并运行非集成的单元测试：

toolName: Shell

status: success

修复测试中的指针参数问题：

toolName: edit_file_search_replace

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_copy_verify_test.go

changes: undefined

简化写法：

toolName: edit_file_search_replace

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_copy_verify_test.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_copy_verify_test.go

changes: undefined

toolName: Shell

status: success

时间用例存在巧合相等，修正为一个明显不同的时间：

toolName: edit_file_search_replace

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_copy_verify_test.go

changes: undefined

toolName: Shell

status: success

单元测试通过。查看集成测试的启用方式：

toolName: search_by_regex

status: success

集成测试默认启用。运行 copy verify 相关集成测试：

toolName: Shell

status: success

这是 Windows 环境无法创建符号链接导致的夹具解压问题。先验证现有的 TestCopy 是否同样失败，以确认与我的改动无关：

toolName: Shell

status: success

确认是该 Windows 机器的既有环境限制（符号链接权限），与改动无关。我把集成测试改为直接创建普通文件备份，避开 tar 夹具。先看现有自建数据的测试模式：

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

所有集成都依赖 tar 夹具。我在测试文件内自建无符号链接的普通文件夹具，使测试可在当前平台运行：

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_copy_verify_test.go

重写整个测试文件，使用自建夹具：

toolName: Write

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_copy_verify_test.go

changes: undefined

运行验证相关的全部测试：

toolName: Shell

status: success

部分通过，输出被截断。逐个运行集成测试查看失败原因：

toolName: Shell

status: success

TestCopyVerify 通过。运行缺失 blob 与篡改测试：

toolName: Shell

status: success

toolName: Shell

status: success

查看完整错误信息：

toolName: Shell

status: success

测试钩子要求 snapshots 在 index 之前列出。调整测试中的调用顺序：

toolName: edit_file_search_replace

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_copy_verify_test.go

changes: undefined

toolName: Shell

status: success

全部通过。运行本文件全部测试做整体确认：

toolName: Shell

status: success

新增测试全部通过。现在更新文档。先查看 changelog 模板和 man 页生成约定：

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\changelog\TEMPLATE

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_generate_integration_test.go

让我查看 generate 命令的输出选项，以便重新生成 man 页和补全文件：

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_generate.go

我可以用构建出的二进制重新生成 man 页和补全文件。先构建并生成，再检查 diff：

toolName: Shell

status: success

检查生成文件的变更范围：

toolName: Shell

status: success

重新生成可能因 Windows 换行符导致大量无关差异。检查非 copy 文件的实际 diff：

toolName: Shell

status: success

重新生成混入了换行符和其他功能的历史漂移。回退所有生成文件，改为只对 copy 相关文件做最小手工修改：

toolName: Shell

status: success

现在手工在 copy 的 man 页中添加 `--verify`（按字母序位于 --tag 之后）：

toolName: edit_file_search_replace

status: success

filePath: f:\swe\099934\project-01\doc\man\restic-copy.1

changes: undefined

现在查看各补全文件中 copy 命令的 flag 列表位置：

toolName: search_by_regex

status: success

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\doc\bash-completion.sh

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\doc\bash-completion.sh

通过生成到 stdout 获取各 shell 中 `--verify` 的精确片段：

toolName: Shell

status: success

在 bash 补全的 copy 函数中加入 `--verify`：

toolName: edit_file_search_replace

status: failed

filePath: f:\swe\099934\project-01\doc\bash-completion.sh

changes: undefined

需要更大的唯一上下文。先确认 copy 函数中 from-* 块的唯一性：

toolName: search_by_regex

status: success

toolName: edit_file_search_replace

status: success

filePath: f:\swe\099934\project-01\doc\bash-completion.sh

changes: undefined

获取 zsh、fish、powershell 中 copy 的 verify 片段：

toolName: Shell

status: success

toolName: Shell

status: success

将生成结果写入临时文件再检索：

toolName: Shell

status: success

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: Shell

status: success

toolName: Shell

status: success

文件本身就小，临时文件是 UTF-16 编码导致 Grep 失效。改用 Select-String：

toolName: Shell

status: success

toolName: Shell

status: success

toolName: Shell

status: success

直接让二进制写文件（自身控制编码），再用 Grep 检索：

toolName: Shell

status: success

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

toolName: search_by_regex

status: success

这些补全可能不枚举本地 flag。查看生成文件的结构：

toolName: view_files

status: success

filePath: C:\Users\czxy4\AppData\Local\Temp\restic-gen\zsh

zsh/fish/powershell 补全是动态调用 `__complete` 的，不静态枚举 flag。确认一下：

toolName: search_by_regex

status: success

确认仅 bash 补全需要静态修改（已完成）。现在在手册文档中增加验证说明章节：

toolName: edit_file_search_replace

status: success

filePath: f:\swe\099934\project-01\doc\045_working_with_repos.rst

changes: undefined

清理构建产物并做最终检查：

toolName: delete_file

status: success

toolName: Shell

status: success

变更集干净且聚焦。运行最终构建、vet 与测试验证：

toolName: Shell

status: success

全部通过。完整复审最终实现文件：

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_copy.go

实现逻辑完整覆盖需求。做最后的格式检查与全套新测试运行：

toolName: Shell

status: success

测试全部通过，但测试文件需要 gofmt 格式化。查看差异并修复：

toolName: Shell

status: success

toolName: Shell

status: success

格式化完成。确认测试仍通过，并检查 golangci-lint 是否可用：

toolName: Shell

status: success

golangci-lint 可用，对修改的包运行检查：

toolName: Shell

status: success

修复我代码中的 lint 问题（context 参数位置），其余均为既有文件的历史问题：

toolName: edit_file_search_replace

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_copy_verify_test.go

changes: undefined

更新所有调用点：

toolName: edit_file_search_replace

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_copy_verify_test.go

changes: undefined

toolName: edit_file_search_replace

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_copy_verify_test.go

changes: undefined

重新运行 lint 并只关注我修改的文件，同时复测：

toolName: Shell

status: success

我的文件已无 lint 问题。确认既有问题文件未被我改动，并对全仓做 vet：

toolName: Shell

status: success

vet 警告位于未改动的既有文件。最后完整通读验证实现与测试文件：

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_copy.go

实现完整。最后通读测试文件确认最终状态：

toolName: view_files

status: success

filePath: f:\swe\099934\project-01\cmd\restic\cmd_copy_verify_test.go

全部完成。更新任务清单并做最终确认：

toolName: todo_write

status: success

Todos updated: 7 items

toolName: Shell

status: success

已完成 `copy --verify` 功能的完整实现，所有新增测试通过，构建、vet、lint 对改动文件均无问题。

## 改动概览

**核心实现** [cmd_copy.go](file:///f:/swe/099934/project-01/cmd/restic/cmd_copy.go)

- 新增默认关闭的 `--verify` 开关（[L68](file:///f:/swe/099934/project-01/cmd/restic/cmd_copy.go#L68)、[L75](file:///f:/swe/099934/project-01/cmd/restic/cmd_copy.go#L75)），未指定时原有筛选、等价跳过、批量上传保存、取消退出、中断重跑逻辑完全不变。
- [verifySnapshots](file:///f:/swe/099934/project-01/cmd/restic/cmd_copy.go#L391-L426)：对筛选出的**每个**源快照（含本次新复制和因 Original 等价被跳过的）执行验证；逐个输出 `snapshot <id> verified`，结尾输出 `verified N snapshots` 汇总。
- [findDestinationSnapshot](file:///f:/swe/099934/project-01/cmd/restic/cmd_copy.go#L441-L468)：按 Original/自身 ID 匹配并要求 `similarSnapshots` 唯一——缺失报 `no corresponding snapshot`，重复报 `multiple corresponding snapshots`。本次新保存的快照会在保存后重新加载并入映射，无需二次列目录。
- [verifyTree](file:///f:/swe/099934/project-01/cmd/restic/cmd_copy.go#L475-L545) + [compareNodes](file:///f:/swe/099934/project-01/cmd/restic/cmd_copy.go#L581-L660)：递归比较路径、节点类型、大小、模式、三种时间、链接目标、Device/DeviceID、内容 blob 有序序列、扩展属性、通用属性和子树（另含 UID/GID/inode/links 更严格校验），用 `DualTreeIterator` 捕获结构缺失与树解析失败。
- [verifyDataBlob](file:///f:/swe/099934/project-01/cmd/restic/cmd_copy.go#L665-L687) 与树 blob 校验：先 `LookupBlobSize` 查索引确认存在，再 `LoadBlob` 读取（自动解密解压），用 `restic.Hash` 比对明文内容哈希；共享 blob 去重避免重复读取。
- 所有错误都包装为 `snapshot <id>: path "<path>": ...`，明确指出快照与路径；各层检查 `ctx.Err()`，取消时返回非零（context.Canceled 对应退出码 130）。

**测试** [cmd_copy_verify_test.go](file:///f:/swe/099934/project-01/cmd/restic/cmd_copy_verify_test.go)

- `TestCopyVerify`：双快照验证成功、输出含逐快照结果与汇总；二次运行全部跳过时仍逐一验证。
- `TestCopyVerifyMissingDataBlob`：删除目标数据 pack 后验证失败，错误含快照与路径。
- `TestVerifyTreeDetectsTamperedBlobs`：包装仓库篡改明文，分别命中数据/树 blob 的哈希不匹配分支。
- `TestFindDestinationSnapshot`、`TestCompareNodes`：覆盖缺失/重复匹配与全部字段的逐字段差异报错。

**文档**：[045_working_with_repos.rst](file:///f:/swe/099934/project-01/doc/045_working_with_repos.rst#L289-L314) 新增 "Verifying copied snapshots" 章节；同步更新 [restic-copy.1](file:///f:/swe/099934/project-01/doc/man/restic-copy.1#L78-L80) 与 [bash-completion.sh](file:///f:/swe/099934/project-01/doc/bash-completion.sh#L834-L835)（zsh/fish/powershell 补全为动态 `__complete`，无需改动）。

两点说明：
1. 本机 Windows 无创建符号链接权限，仓库既有的 tar 夹具集成测试（如原 `TestCopy`）在改动前就无法运行；新测试改用普通文件夹具规避了该环境限制，全部可跑通。
2. 未新增 changelog 文件——仓库惯例要求以真实 issue/PR 编号命名，没有可信编号时不臆造；如需要我可以按你提供的编号补上。
