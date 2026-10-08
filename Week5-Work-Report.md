# 第五周工作报告：Controller Manager 与 Admission Webhook 部署合并及系统测试

日期：2026-09-23
项目基线：`week5/deployment-integration`，`c99ad43dd`，叠加当前工作区改动
计划依据：技术方案第 5 周“Deployment 合并与系统测试”及 E01–E14 验收清单

## 当前结论

第五周的 Deployment/Service/Helm **配置实现与模板验证**、Envtest、单元测试、Race Test、Vendor/生成文件检查和多架构编译已完成。集群预览测试验证了 Webhook 连通性、双副本选主、重启和两类故障恢复；修复 CRD、RBAC 和 OfflineMigration 后，E01–E07 也已全部通过。

第五周 E01–E14 已全部完成本地或实际集群验证。安全修复已将工具链升级到 Go 1.25.13，`govulncheck` 复扫为 **0 个可达漏洞**；Operations CRD、RBAC、OfflineMigration、旧 Helm values 兼容和主节点调度问题均已修复并复测。最终集群为 Helm `deployed`，CloudCore 1/1、Controller Manager 2/2，正式 Service 有两个 Endpoint，业务拒绝与 Mutation 均通过。当前仅缺远端 CI 运行记录和代码提交记录。

## 已完成工作

| 计划任务 | 已完成内容 | 验证结果 |
| --- | --- | --- |
| 统一 Deployment/Service/Helm | Controller Manager 暴露 9443 Webhook 端口，挂载 TLS Secret，设置 Liveness/Readiness Probe；Admission Service 可在旧组件与新组件之间切换；回滚配置保留 WebhookConfiguration 的 Helm 归属；双副本启用 Leader Election。 | Chart 验证与 lint 通过；E12 升级、E13 回滚/恢复、E14 卸载重装全部通过。最终 Controller Manager **2/2 Ready**，正式 Service 有两个 Endpoint。 |
| Envtest | 安装 Apps、Devices、Operations、Router CRD；启动 Kubernetes 1.32 API Server、etcd、Manager 和 TLS Webhook；用真实 Kubernetes Client 经 WebhookConfiguration 触发 5 个 Validating 与 2 个 Mutating 路由；检查 Context 退出和端口释放。 | 完整套件 **52/52 通过**；七条 API Server Admission 用例 **7/7 通过**；直接 TLS 路由用例 **8/8 通过**。 |
| 集群预览测试 | 在现有 `k8s-master` 上启动双副本预览 Deployment；经 API Server 验证 5 个 Validating 与 2 个 Mutating Webhook，并验证滚动重启、Leader 切换、错误 CABundle、无 Endpoint 故障与恢复。 | **E01–E07 全部通过**：合法资源成功，非法 Device、DeviceModel 与 NodeUpgradeJob 按预期拒绝，Rule/RuleEndpoint 创建删除成功，OfflineMigration Patch 与 NodeUpgradeJob 默认值和幂等通过。E08–E11 此前完成预览验证。 |
| 工程检查 | 修复 Go workspace 的 Vendor/代码生成校验；修复 `TimeoutJob` 的并发停止竞争；增加包含单元测试、Race Test、Envtest、Helm、Vendor、CRD、漏洞扫描和双架构构建的 CI 工作流。 | 单元测试、选定包 Race Test、`go vet`、Vendor、代码生成和 CRD 检查通过；Linux amd64/arm64 构建通过。CI 工作流文件已写入仓库，但尚无远端 CI 通过记录。 |

## 问题处理状态

| 问题 | 当前状态 | 处理结果或剩余动作 |
| --- | --- | --- |
| Go 1.23.12 标准库及依赖可达漏洞 | **已解决并复测通过** | 升级到 Go 1.25.13 和安全依赖版本；govulncheck 为 0 个可达漏洞 |
| Operations v1alpha2 CRD 缺失导致 Controller Manager 退出 255 | **已解决并复测通过** | 补齐三项 Operations CRD，预览 Deployment 恢复 2/2 Ready |
| Controller Manager 缺少 Events 和业务资源 RBAC | **已解决并复测通过** | RBAC 已补齐，Rule/RuleEndpoint 等实际集群操作通过 |
| OfflineMigration 把 300 秒临时 toleration 误判为永久 toleration | **已解决并复测通过** | 修正判断逻辑，E06 输出 `true` 且退出 0 |
| Admission Chart 把 `2>&1` 当成进程参数 | **已解决，模板验证通过** | 已删除该参数，Chart 验证和 lint 通过 |
| 旧 release 缺少新增 labels、Leader Election、Webhook 端口和证书目录 | **已解决并复测通过** | 模板提供兼容默认值；旧 Admission 基线和 E12 正式切换通过 |
| Admission/CloudCore 缺少 control-plane toleration | **已解决并复测通过** | 增加默认及旧 values 回退；两个组件均成功调度到 `k8s-master` |
| 预览 RBAC 无 Helm 所有权 | **已解决并复测通过** | 比对规则并备份后，仅对三个完全一致的 RBAC 对象添加 Helm 所有权 |
| E12–E14 正式升级、回滚、卸载重装 | **已完成并通过** | E12 切换、E13 双向切换、E14 清理与重装后的 API Server 回归全部通过 |
| 远端 CI 记录 | **尚未完成** | 当前只有本地门禁结果，仍需提交并取得远端运行结果 |

1. **漏洞扫描已经修复并通过本地复扫。** 工具链、`go.mod`、`go.work` 和 Controller Manager Dockerfile 已升级到 Go 1.25.13；升级 gRPC、x/net、x/text、OpenTelemetry 等依赖，移除 Controller Manager 对旧 `distribution/v3/reference` 的调用，并以兼容 Kubernetes 0.32 的 `otelgrpc` 版本约束解决编译冲突。`govulncheck` 结果为 **0 个可达漏洞**；另有 4 个导入包和 4 个依赖模块的不可达发现，不触发本项目可达漏洞门禁。升级后单元测试、Race、Vet、Vendor、Envtest 52/52、Helm 检查及 amd64/arm64 编译通过。
2. **E01–E07 已完成实际集群验证。** 测试前修复了 Operations CRD 版本缺失、Controller Manager 业务资源与 Events RBAC，以及 OfflineMigration 将 API Server 自动加入的 300 秒临时 toleration 误判为永久 toleration 的问题。部署 Go 1.25.13 新二进制后，Deployment 为 2/2 Ready、Service 有两个 Endpoint；E01、E04、E06、E07 成功路径退出码为 0，E02、E03、E05 的非法输入均被对应 Webhook 按预期拒绝，E04 三项删除退出码均为 0。
3. **E12–E14 已完成。** 旧 Admission 基线为 deployed 且能拒绝非法 Device；E12 切换到 Controller Manager 后为 2/2 Ready，非法 Device 拒绝和 OfflineMigration Mutation 通过；E13 回滚到旧组件及恢复新组件均通过；E14 卸载时 release、工作负载和三项 WebhookConfiguration 清理，CRD 与外置证书保留，按保存 values 重装后全部恢复。最终 Helm revision 2 deployed，CloudCore 1/1，Controller Manager 2/2，三个 Pod 重启数均为 0。
4. **远端证据尚待补充。** 本地与集群验收已通过；CI 工作流尚未提交或在远端运行，因此报告保留“远端 CI 记录待补”一项。

## 正式环境现状

`cloudcore` Helm release 为 revision 2、状态 `deployed`。CloudCore 使用 `docker.io/kubeedge/cloudcore:v1.19.2-runtimeclass-20260812`，1/1 Ready；Controller Manager 使用 `docker.io/kubeedge/controller-manager:week5-go1.25.13`，2/2 Ready；`kubeedge-admission-service` 有两个 `:9443` Endpoint。三项正式 WebhookConfiguration 均归属 `cloudcore`。预览 Service、预览证书、临时 WebhookConfiguration 和业务测试 namespace 已清理。

## 漏洞修复的版本依据

安全修复采用 Go `1.25.13`、`google.golang.org/grpc v1.83.1`、`golang.org/x/text v0.39.0`、`golang.org/x/net v0.56.0` 和 `go.opentelemetry.io/otel/sdk v1.44.0`。由于 Kubernetes 0.32 仍使用旧 `otelgrpc` 拦截器 API，根模块通过 `replace` 固定兼容版本；实际编译、单元测试、Race、Vet、Envtest 和双架构构建均已通过。`govulncheck` 复扫为 0 个可达漏洞。

## 后续验收顺序

1. 归档 E01–E14 的命令、退出码、关键响应和截图。
2. 提交当前代码与文档并运行远端 CI。
3. 取得远端 CI 全绿记录后完成第五周最终签署。

详细命令、已执行输出和截图点见 [第五周测试报告](Week5-Test-Report.md)。
