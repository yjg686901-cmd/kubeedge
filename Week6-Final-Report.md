# 第六周最终验证报告

> 日期：2026-09-23
> 范围：旧组件清理、文档收口、CI/生成检查、全量回归

## 0. 项目版本与目录说明

### 0.1 版本矩阵

| 项目 | 当前版本或状态 |
| --- | --- |
| Go | 1.25.13 |
| Kubernetes Go 依赖 | v0.32.10；KubeEdge replace 模块为 v1.32.10-kubeedge1 |
| Kubernetes 主模块 | v1.32.10 |
| controller-runtime | v0.20.4 |
| controller-gen | v0.17.3 |
| Envtest | Kubernetes 1.32.0 |
| Helm 客户端 | v3.17.3 |
| 实际测试集群 | Kubernetes v1.28.13 |
| CloudCore 镜像 | `docker.io/kubeedge/cloudcore:v1.19.2-runtimeclass-20260812` |
| Controller Manager 镜像 | `docker.io/kubeedge/controller-manager:week5-go1.25.13` |
| Git 分支 | `week6/final-cleanup` |
| Git 提交 | `aacc684fd21d1d9ec001008c5d64f4f3decce8cd` |

### 0.2 主要目录内容

| 路径 | 内容 |
| --- | --- |
| `cloud/cmd/controllermanager` | 统一 Controller Manager 程序入口、选项和 Manager 启动逻辑 |
| `cloud/pkg/admissioncontroller` | 5 个 Validating、2 个 Mutating Handler 及统一注册适配层 |
| `cloud/pkg/controllermanager` | Reconcile Controller、Leader Election 和业务控制器 |
| `cloud/test/integration/controllermanager` | Envtest、TLS 路由、API Server Admission 集成测试 |
| `build/controllermanager` | Controller Manager 镜像和最小 RBAC 构建资源 |
| `manifests/charts/cloudcore` | Deployment、Service、WebhookConfiguration、证书挂载及 Helm values |
| `hack` | 证书生成、Chart 验证、E2E、代码生成和发布脚本 |
| `.github/workflows` | Controller Manager Webhook 和发布 CI 工作流 |
| `admission-baseline` | 旧/新行为对照用 fixtures、cases 和配置快照，不是运行组件 |
| `week5-test-evidence` | 第五周 Envtest、Helm 等原始测试日志 |
| `week6-test-evidence` | 第六周本地回归、集群状态和 Helm 历史证据 |
| `docs` | 架构、安装、升级、回滚、故障排查和贡献者指南 |

独立运行入口 `cloud/cmd/admission` 和部署目录 `build/admission` 已删除。业务 Handler 目录 `cloud/pkg/admissioncontroller` 必须保留，因为统一 Controller Manager 仍调用其中的七个 Handler。

## 1. 当前结论

第六周代码清理、文档和本地/实际集群回归已完成。统一组件已完成升级、回滚和再次升级。代码已推送到 `week6/final-cleanup`；远端 CI 已触发，但因 GitHub 账户 Billing 锁定而未启动任务，不影响当前本地与集群验收结论。

## 2. 已完成的清理

| 项目 | 状态 | 证据 |
| --- | --- | --- |
| 独立 Admission Command | 已删除 | `cloud/cmd/admission` 不存在 |
| 独立 TLS HTTP Server 与参数 | 已删除 | 旧 Command、Server、Options 文件删除 |
| 进程动态注册 Webhook | 已删除 | `registerValidateWebhook`、`registerMutatingWebhook` 不存在 |
| 独立 Admission 镜像 | 已删除 | Makefile、镜像矩阵、release 脚本不再构建 |
| 独立 Deployment/RBAC | 已删除 | `build/admission` 与 Chart 旧模板删除 |
| 旧 Helm 参数与回滚文件 | 已删除 | `admission.*` 与 legacy rollback values 删除 |
| 重复 Client | 已收口 | Handler 复用 Controller Manager 创建的 KubeEdge client |
| 声明式注册 | 已完成 | Helm 管理 5 个 Validating 与 2 个 Mutating 路径 |

保留 `cloud/pkg/admissioncontroller`，因为其中是七个仍在使用的业务 Handler；保留 `admission-baseline`，因为它是行为兼容验证数据，不是旧运行组件。

## 3. 文档交付

- 运维文档：`docs/controller-manager-webhook.md`
- 贡献者指南/技术说明：`docs/controller-manager-webhook-contributor-guide.md`
- 清理 PR 说明：`Week6-Cleanup-PR.md`
- 最终报告：本文档
- 本地回归原始记录：`week6-test-evidence/local-validation.log`
- 集群与 Helm 原始记录：`week6-test-evidence/cluster-final.log`

## 4. 验证记录

| 检查 | 结果 | 说明 |
| --- | --- | --- |
| Admission/Controller Manager 单元测试 | 通过 | Go 1.25.13；相关包全部通过 |
| Race Test | 通过 | Admission、NodeGroup、NodeTask |
| go vet | 通过 | Admission 与 Controller Manager |
| Envtest | 通过 | Kubernetes 1.32；52/52 specs |
| Helm lint/模板约束 | 通过 | 1 chart linted；controller-manager-only 校验通过 |
| vendor/codegen/CRD | 通过 | Vendor、client 代码、CRD 均为最新 |
| AMD64/ARM64 构建 | 通过 | 两个静态 Controller Manager ELF |
| govulncheck | 通过 | 可达漏洞 0；导入包中 4 项不可达，依赖模块中另有 4 项不可达 |
| 旧入口静态扫描 | 通过 | Command、build 目录、旧 values、动态注册函数均不存在 |
| 实际集群升级/回滚/再升级 | 通过 | revision 6 升级、7 回滚、8 再升级；最终 revision 8 deployed |

### 4.1 实际集群结果

- CloudCore：`1/1` Ready。
- Controller Manager：`2/2` Ready。
- `kubeedge-admission-service`：两个端点，均监听 `9443`。
- 非法 Device：被 `validatedevice.kubeedge.io` 拒绝，错误包含 `property names must be unique`。
- 独立 `kubeedge-admission` Deployment 与 ServiceAccount：不存在。

首次使用 `helm upgrade --atomic --timeout 5m` 时，revision 3 因 Chart 内无关的 Mosquitto DaemonSet 在边缘节点 ImagePullBackOff 而超时；自动回滚也因同一全 Chart 等待超时，留下 revision 3、4 的失败记录。统一组件全过程保持健康。随后先恢复 revision 5，再使用 `--wait=false` 并分别对 CloudCore、Controller Manager、Service 和业务请求做严格检查，完成 revision 6 → 7 → 8 的升级/回滚闭环。该处理避免用无关边缘工作负载状态代替本项目验收结果。

### 4.2 可复现命令

```bash
go test ./cloud/pkg/admissioncontroller/... ./cloud/pkg/controllermanager/...
go test -race ./cloud/pkg/admissioncontroller/... \
  ./cloud/pkg/controllermanager/nodegroup/... \
  ./cloud/pkg/controllermanager/nodetask/...
go vet ./cloud/pkg/admissioncontroller/... ./cloud/pkg/controllermanager/...
hack/verify-controller-manager-webhook-chart.sh
make verify-vendor verify-codegen verify-crds
govulncheck ./cloud/cmd/controllermanager/... \
  ./cloud/pkg/admissioncontroller/... ./cloud/pkg/controllermanager/...

helm upgrade cloudcore ./manifests/charts/cloudcore -n kubeedge \
  --reuse-values --wait=false
kubectl -n kubeedge rollout status deployment/kubeedge-controller-manager --timeout=5m
kubectl -n kubeedge rollout status deployment/cloudcore --timeout=5m
kubectl -n kubeedge get endpoints kubeedge-admission-service
helm -n kubeedge rollback cloudcore REVISION --wait=false
```

## 5. 最终验收门槛

- [x] 仓库无独立 Admission 命令、镜像、Deployment、RBAC 和动态注册入口。
- [x] 七个 Handler 由统一 Controller Manager Webhook Server 提供。
- [x] Helm 是 WebhookConfiguration 的唯一管理者。
- [x] 本轮本地与生成检查全部通过。
- [x] 实际集群统一版本升级、回滚、再次升级通过。
- [ ] 远端 CI 有成功运行链接；当前运行 `35838530513` 因 GitHub Billing 锁定未启动。

远端 CI 记录：https://github.com/yjg686901-cmd/kubeedge/actions/runs/35838530513 。解除账户 Billing 限制后需重新运行。本地同等检查结果与远端 CI 状态分别记录，不能用本地结果替代远端记录。
