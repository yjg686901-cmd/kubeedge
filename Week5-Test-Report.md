# 第五周 Controller Manager Webhook 测试报告

日期：2026-09-23
代码基线：`week5/deployment-integration`，`c99ad43dd` 加测试补充文件
测试范围：Deployment/Service/Helm、Envtest、集群 E2E、CI 安全与生成检查

## 结论

**E01–E14 全部通过。** Helm 渲染、Envtest、单元测试、Race Test、多架构编译及 govulncheck 已通过；E01–E07 完成实际 API Server 验证，E08–E11 完成重启、多副本及故障恢复，E12–E14 完成正式升级、回滚、卸载重装和业务回归。最终 Helm 状态为 deployed，CloudCore 1/1、Controller Manager 2/2，三个 Pod 重启数为 0。远端 CI 记录仍待补充。

| 编号 | 检查项 | 执行位置 | 实际结果 | 证据截图 |
| --- | --- | --- | --- | --- |
| W5-01 | Deployment/Service/Helm 渲染与非法开关校验 | 从节点 WSL | **通过**：1 chart linted，0 failed；7 条路由、证书 Secret、Probe 和 Service 端口均通过脚本断言 | `S01-helm.png` |
| W5-02 | Kubernetes 1.32 Envtest 完整套件 | 从节点 WSL；主节点新增测试复核 | **通过**：原 45/45；新增 API Server 测试后主节点 52/52，exit=0 | `S02-envtest-full.png`；`/tmp/week5-envtest-full-retest.log` |
| W5-03 | 七条 TLS Webhook 路由及无效 Device | 从节点 WSL | **通过**：8/8 Specs，exit=0；Context 退出由 AfterSuite 检查 | `S03-webhook-routes.png` |
| W5-03A | API Server 经 WebhookConfiguration 触发 Admission | 主节点 Envtest | **通过**：7/7 路由；5 条 Validating 的拒绝场景、2 条 Mutating 的 Patch 场景 | `S03A-apiserver-envtest.png`；`/tmp/week5-apiserver-envtest.log` |
| W5-04 | Controller Manager 多架构编译 | 从节点 WSL | 主节点已验证 amd64/arm64 编译通过；从节点截图待补 | `S04-multiarch.png` |
| W5-05 | govulncheck | 主节点 | **通过**：Go 1.25.13 与依赖升级后复扫为 0 个可达漏洞 | `/home/ubuntu/week5-transfer/govulncheck-retest.log` |
| W5-06 | Vendor、生成代码、CRD | 主节点 | **通过**：已使校验与 Go workspace 布局一致；Vendor 和代码生成重新执行通过；CRD 检查此前通过 | `S06-ci-checks.png` |
| W5-07 | 实际集群预览 Deployment/Service/TLS/Probe | `k8s-master` | **通过**：补齐 Operations v1alpha2 CRD、RBAC 并部署新二进制后为 2/2 Ready、两个 Endpoint，新 Pod 重启数为 0 | 最新主节点截图 |
| W5-10 | E08–E11 重启、Leader 切换、错误 CABundle、Service 无 Endpoint | `k8s-master` | **通过预览验证**：非 Leader 与 Leader 均提供 Webhook；删 Leader 后 10/10 次请求成功且 Lease 转移；错误 CABundle 被 API Server 拒绝；无 Pod/Endpoint 时按 Fail 策略拒绝，恢复后成功 | `S10-fault-injection.png` |
| W5-11 | E01–E07 全量集群业务、E12–E14 Helm 升级/回滚/卸载重装 | 现有集群 | **通过**：E01–E07 全部通过；E12 切换到双副本 Controller Manager，E13 回滚旧 Admission 并恢复新组件，E14 卸载清理及按 values 重装，最终 API Server 拒绝与 Mutation 回归均通过 | 主节点执行记录与 `/home/ubuntu/week5-e12-e14-backup` |
| W5-08 | Controller/Admission 单元测试 | 主节点 | **通过**：`go test ./cloud/pkg/controllermanager/... ./cloud/pkg/admissioncontroller/...`，exit=0 | `/tmp/week5-unit.log` |
| W5-09 | Race Test 与 go vet | 主节点 | **通过**：Race Test 首次发现 `TimeoutJob` 并发停止与测试数据竞争，修复后复测通过；`go vet` 通过 | `/tmp/week5-race-retest.log`；`/tmp/week5-vet.log` |

## 截图命令

所有截图应同时包含命令、关键输出和机器提示符。执行位置不能混用。

### S01：Helm 渲染（从节点 WSL）

```bash
cd ~/projects/kubeedge
bash hack/verify-controller-manager-webhook-chart.sh
```

截图保留 `1 chart(s) linted, 0 chart(s) failed` 和 `controller-manager webhook chart verification passed`。此测试证明模板可正确渲染及拒绝冲突配置，不证明 Pod 已部署。

### S02：完整 Envtest（从节点 WSL）

已运行，日志为 `/tmp/week5-envtest.log`。可用以下命令重新显示结果后截图：

```bash
tail -n 8 /tmp/week5-envtest.log
```

原截图保留 `Ran 45 of 45 Specs`、`45 Passed`、`PASS`。新增七个 API Server 用例后的主节点完整复测使用同一套 Envtest assets，运行结果为 52/52，exit=0；可另行截图新增结果。

### S03：七条路由 Envtest（从节点 WSL）

已运行，日志为 `/tmp/week5-webhook-routes.log`。可用以下命令重新显示结果后截图：

```bash
tail -n 8 /tmp/week5-webhook-routes.log
```

截图保留 `Ran 8 of 45 Specs`、`8 Passed`、`0 Failed` 和 `PASS`。七条路由通过 TLS 直接请求 Webhook；这并非真实集群 API Server 的 admission 调用。

### S03A：真实 API Server Admission（主节点）

新增测试位于 `cloud/test/integration/controllermanager/webhook_apiserver_test.go`，安装 Devices/Operations/Router CRD 与临时 WebhookConfiguration，使用 Kubernetes Client 创建资源。五种非法 CRD 请求由对应 Validating Handler 拒绝；Pod 和 NodeUpgradeJob 的创建经过 Mutating Handler 并带回默认 Patch。聚焦复测为 7/7，exit=0。端点 URL 为 Envtest 本机地址，尚未覆盖集群 Service DNS。命令：

```bash
cd /home/ubuntu/kubeedge-week4-test
PATH=/home/ubuntu/.local/go1.23.12/bin:$PATH go test -v ./cloud/test/integration/controllermanager -run TestAppsAPIs -ginkgo.focus='Kubernetes API Server admission' -count=1 -timeout=5m -ldflags '-X github.com/kubeedge/kubeedge/cloud/test/integration/controllermanager.appsCRDDirectoryPath=/home/ubuntu/kubeedge-week4-test/build/crds/apps -X github.com/kubeedge/kubeedge/cloud/test/integration/controllermanager.envtestBinDir=/tmp/envtest/bin/k8s/1.32.0-linux-amd64'
```

### S04：多架构编译（从节点 WSL）

```bash
cd ~/projects/kubeedge
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/week5-controller-manager-amd64 ./cloud/cmd/controllermanager
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /tmp/week5-controller-manager-arm64 ./cloud/cmd/controllermanager
file /tmp/week5-controller-manager-amd64 /tmp/week5-controller-manager-arm64
```

截图保留 `x86-64` 与 `ARM aarch64`（或等价架构标识）。这证明两个架构的二进制可构建，不证明 ARM 设备上已运行。

### S05：漏洞扫描（主节点）

依赖与 Go 工具链升级后，使用 Go 1.25.13 和 `govulncheck v1.1.4` 完成复扫，结果为 0 个可达漏洞。工具同时提示导入包和依赖模块中存在不可达发现，不影响本项目的可达漏洞门禁。完整日志保存为 `/home/ubuntu/week5-transfer/govulncheck-retest.log`。在主节点显示结论并截图：

```bash
tail -n 6 /home/ubuntu/week5-transfer/govulncheck-retest.log
```

截图保留 `No vulnerabilities found` 和 `affected by 0 vulnerabilities`。漏洞数据库可能更新，后续重跑以当天输出为准。

### S06：Vendor、生成文件和 CRD（主节点）

修复后 `make verify-vendor` 和 `make verify-codegen` 均通过，`make verify-crds` 此前通过。现在的 Vendor 校验在临时目录重建并比较，不修改共享工作区。可在主节点执行并截图：

```bash
cd /home/ubuntu/kubeedge-week4-test
PATH=/home/ubuntu/.local/go1.23.12/bin:$PATH make verify-vendor verify-codegen verify-crds
```

### S07：集群预览状态（主节点）

```bash
kubectl -n kubeedge get deployment kubeedge-controller-manager
kubectl -n kubeedge get service kubeedge-controller-manager-webhook-preview
kubectl -n kubeedge get endpoints kubeedge-controller-manager-webhook-preview
kubectl -n kubeedge get lease kubeedge-controller-manager
```

预览 Deployment 的验收条件为稳定 2/2 Ready、两个 Endpoint、一个 Lease holder；此前曾满足，但最新截图只有 1/2 Ready、一个 Endpoint，须修复并复测。此前两个 `NotFound` 的前置检查曾由预览部署解决，历史日志在 `/home/ubuntu/week5-transfer/e2e-preflight-retest.log`。`kubeedge-admission-service` 和正式 Helm release 仍未切换。

### S10：实际集群故障与恢复（主节点）

预览资源在 `kubeedge` 命名空间；临时 `week5-preview-device-validation` WebhookConfiguration 只匹配带 `week5.kubeedge.io/admission-test=true` 标签的 `week5-admission-test` 命名空间。可用以下只读或 server-side dry run 命令截图恢复状态：

```bash
kubectl -n kubeedge get deployment kubeedge-controller-manager
kubectl -n kubeedge get endpoints kubeedge-controller-manager-webhook-preview
kubectl -n kubeedge get lease kubeedge-controller-manager -o jsonpath='{.spec.holderIdentity}{"\n"}'
kubectl create --dry-run=server -f /home/ubuntu/week5-transfer/preview-valid-device.yaml -o name
kubectl create --dry-run=server -f /home/ubuntu/week5-transfer/preview-invalid-device.yaml -o name
```

最后一条应因 `property names must be unique` 退出 1；这是预期拒绝。故障注入时实际观察到错误 CABundle 导致 `x509: certificate signed by unknown authority`；预览 Deployment 缩容到 0 后，API Server 报 `connect: connection refused`，恢复到 2 副本后合法 Device dry run 再次成功。单独将 Service Endpoint 置 0 时，请求仍可通过已有 TLS 连接成功；因此无 Endpoint 的拒绝结论以同时终止预览 Pod 的复测为准。

## 集群 E2E 当前范围

现有集群已完成 E01–E14。E12 建立旧 Admission deployed 基线后升级到双副本 Controller Manager，非法 Device 拒绝和 OfflineMigration Mutation 通过；E13 回滚旧组件及恢复新组件均通过；E14 卸载后确认 release 与三项 WebhookConfiguration 消失、CRD 和外置证书保留，随后按保存 values 重装。重装后因旧 CloudCore values 缺少 control-plane toleration 出现 Pending，补齐兼容回退后恢复为 1/1。

Controller Manager 的 Go 1.25.13 OCI 包位于 `/home/ubuntu/week5-transfer/controller-manager-week5-oci.tar`，镜像名为 `docker.io/kubeedge/controller-manager:week5-go1.25.13`，SHA-256 为 `9d185008bba551540d3b4d66afcf9c9f3e74965ff25b3474a2416b25f84e7ad1`。旧 Admission OCI 包位于 `/home/ubuntu/week5-transfer/admission-week5-oci.tar`，镜像名为 `docker.io/kubeedge/admission:week5-go1.25.13`，SHA-256 为 `aae25a7ff9b25ebfb2890442ef572ee0d7cb3ce7011b1649471a366bd411f96c`。两个镜像均为 linux/amd64，下一步导入主节点 containerd 并核对 Helm 资源所有权。

两个 OCI 包已经导入主节点 containerd，两个导入命令退出码均为 0。正式资源名当前均未占用，预览资源无 Helm 所有权标记。旧 Admission Deployment 模板中误传给进程的字面参数 `2>&1` 已删除，Chart 验证和 `helm lint` 通过。

测试过程中依次发现并修复了旧 values 缺少 Admission labels、toleration、Leader Election、Webhook 端口与证书目录，以及 CloudCore 重装后缺少 control-plane toleration的问题；同时删除 Admission 模板中的无效 `2>&1` 参数。每项修复均通过 Chart/lint 或兼容渲染测试，并在实际集群完成最终复测。

E01–E07 的主节点命令、七条临时 Webhook 注册和清理命令见 [第五周集群验收命令](Week5-E2E-Runbook.md)，对应执行截图已经取得。现有 Helm E2E 脚本带有 `deployed` release 检查；建立安全基线后用于 E12–E14。

## 验收限制

- Envtest 使用 Kubernetes 1.32 临时 API Server；实际主节点集群运行 Kubernetes 1.28.13 和旧 CloudCore。
- 本地安全扫描已通过；远端 CI 尚无运行记录。
- 远端 CI 尚无运行记录；当前结论基于本地门禁及实际集群 E01–E14。
