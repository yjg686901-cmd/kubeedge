# 第 1 周基线：Controller Manager + Admission 链路梳理

> 范围：入口、7 个 Handler、动态注册、证书、部署。  
> 用途：供后续「Admission 并入 Controller Manager Webhook」设计冻结。  
> Source：本仓库 `cloud/pkg`、`build/`、`manifests/charts/cloudcore`

---

## 核心结论

Admission 与 Controller Manager 是**两套独立进程 / Deployment**。

| 组件 | 现状 |
|------|------|
| Controller Manager | 已用 controller-runtime Manager；**未开启 WebhookServer** |
| Admission | 手写 `net/http` TLS 服务 + 启动时自注册 WebhookConfiguration |

合并改造的目标形态：把 Admission Handler 迁入 CM 的 controller-runtime Webhook 框架，消灭（或过渡期双写）独立 admission Deployment。完整设计见 [Target-Architecture-Design.md](./Target-Architecture-Design.md)。行为基线与配置快照见 [Admission-Behavior-Baseline.md](./Admission-Behavior-Baseline.md)。

---

## 1. Controller Manager

### 入口

```
cloud/cmd/controllermanager
  → app.NewControllerManagerCommand
  → controllermanager.NewControllerManager
  → mgr.Start(ctx)
```

关键文件：

- `cloud/cmd/controllermanager/controllermanager.go`
- `cloud/cmd/controllermanager/app/controllermanager.go`
- `cloud/cmd/controllermanager/app/options/options.go`
- `cloud/pkg/controllermanager/controllermanager.go`

### Options

| 项 | 值 |
|----|-----|
| Scheme | corev1 + apps/v1alpha1 + operations/v1alpha2 |
| HealthProbeBindAddress | 默认 `:9001` |
| WebhookServer | **无** |
| LeaderElection / Metrics 定制 | 无 |
| FeatureGates | 可关闭 NodeTask v1alpha2（`DisableNodeTaskV1alpha2`） |

### 已注册 Controllers

1. NodeGroupController  
2. EdgeApplicationController  
3. ImagePrePullJobController（可选）  
4. ConfigUpdateJobController（可选）  
5. NodeUpgradeJobController（可选，CR 为 **v1alpha2**）

另起独立 Cache，只缓存 `corev1.Node`，供 NodeTask 控制器使用。

---

## 2. Admission 入口

```
cloud/cmd/admission
  → app.NewAdmissionCommand
  → admissioncontroller.Run(opt)
```

启动顺序：

1. 读 CA（`--ca-cert-file`）
2. `registerWebhooks`（动态注册）
3. 注册 7 条 `http.HandleFunc`
4. `ListenAndServeTLS`

关键文件：

- `cloud/cmd/admission/admission.go`
- `cloud/cmd/admission/app/server.go`
- `cloud/cmd/admission/app/options/options.go`
- `cloud/pkg/admissioncontroller/admission.go`
- `cloud/pkg/admissioncontroller/common.go`

### 默认 Flags

| Flag | 默认 |
|------|------|
| `--port` | `443` |
| `--webhook-namespace` | `kubeedge` |
| `--webhook-service-name` | `kubeedge-admission-service` |
| `--tls-cert-file` | （部署时指向 Secret 挂载路径） |
| `--tls-private-key-file` | 同上 |
| `--ca-cert-file` | 同上 |

---

## 3. 七个 Handler

全部经 `common.go` 的 `serve()` 解码 `AdmissionReview`（v1）。

| # | Path | 类型 | 资源 | Operations | FailurePolicy | Selector | 业务函数 | 做什么 | 文件 |
|---|------|------|------|------------|---------------|----------|----------|--------|------|
| 1 | `/devices` | Validating | `devices.kubeedge.io/v1beta1/devices` | CREATE, UPDATE | Ignore | — | `admitDevice` / `validateDevice` | Properties[].Name 唯一 | `admit_device.go` |
| 2 | `/devicemodels` | Validating | `devices.kubeedge.io/v1beta1/devicemodels` | CREATE, UPDATE | Ignore | — | `admitDeviceModel` / `validateDeviceModel` | Spec.Properties 名称唯一 | `admit_devicemodel.go` |
| 3 | `/rules` | Validating | `rules.kubeedge.io/v1/rules` | CREATE, UPDATE, DELETE | Fail | — | `admitRule` / `validateRule` | 校验 source/target RuleEndpoint 存在与类型配对；业务主要校 CREATE | `admit_rule.go` |
| 4 | `/ruleendpoints` | Validating | `rules.kubeedge.io/v1/ruleendpoints` | CREATE, UPDATE, DELETE | Fail | — | `admitRuleEndpoint` / `validateRuleEndpoint` | servicebus 须有合法 `service_port`；业务主要校 CREATE | `admit_ruleendpoint.go` |
| 5 | `/nodeupgradejobs` | Validating | `operations.kubeedge.io/v1alpha1/nodeupgradejobs` | CREATE, UPDATE, DELETE | Fail | — | `admitNodeUpgradeJob` / `validateNodeUpgradeJob` | Version/Image 校验；NodeNames 与 LabelSelector 互斥；UPDATE 禁止改 Spec | `admit_nodeupgradejob.go` |
| 6 | `/offlinemigration` | Mutating | `core/v1/pods` | CREATE, UPDATE | Ignore | ObjectSelector: `app-offline.kubeedge.io=autonomy` | `mutateOfflineMigration` / `generatePatch` | 注入 `node.kubernetes.io/unreachable` Exists toleration | `mutate_offlinemigration.go` |
| 7 | `/mutating/nodeupgradejobs` | Mutating | `operations.kubeedge.io/v1alpha1/nodeupgradejobs` | CREATE, UPDATE | Ignore | — | `mutatingNodeUpgradeJob` / `generateNodeUpgradeJobPatch` | 默认 concurrency=1、timeoutSeconds=300 | `admit_nodeupgradejob.go` |

### 注册 vs 业务不一致（设计冻结时注意）

- Rule / RuleEndpoint 的 Webhook Rules 含 **UPDATE/DELETE**，但 admit 函数对 UPDATE 返回 unsupported；DELETE 直接放行。
- NodeUpgradeJob Admission 仍绑 **v1alpha1**，而 CM 控制器已用 **v1alpha2**。

---

## 4. 动态注册

机制：进程启动时用 client-go 对 Admissionregistration API 做 Get → 存在则 `Update.Webhooks`，否则 `Create`。

函数：

- `registerValidateWebhook`（`common.go`）
- `registerMutatingWebhook`（`common.go`）
- `AdmissionController.registerWebhooks`（`admission.go`）

**不是** cert-manager，也**不是** controller-runtime 的 Webhook 自动注册。

### 三个 WebhookConfiguration 对象

| Kind | Name | 内含 Hooks |
|------|------|-----------|
| ValidatingWebhookConfiguration | `kubeedge-crds-validate-webhook-configuration` | 5（Device / DeviceModel / Rule / RuleEndpoint / NodeUpgradeJob） |
| MutatingWebhookConfiguration | `mutate-offlinemigration` | 1（OfflineMigration → Pod） |
| MutatingWebhookConfiguration | `kubeedge-mutating-webhook` | 1（Mutating NodeUpgradeJob） |

### ClientConfig 公共字段

| 字段 | 值 |
|------|-----|
| Service | ns=`AdmissionServiceNamespace`，name=`AdmissionServiceName`，port=`Port` |
| CABundle | 读 `CaCertFile` 字节写入 |
| SideEffects | None |
| AdmissionReviewVersions | `["v1"]` |
| TimeoutSeconds | **代码未设置**（走 API 默认） |
| NamespaceSelector | 均未设置 |
| ObjectSelector | 仅 OfflineMigration 有 |

---

## 5. 证书链路

```
openssl 生成（gen-admission-secret.sh）
  → Secret kubeedge-admission-secret（tls.key / tls.crt / ca.crt）
  → Volume 挂载 /admission.local.config/certificates
  → configTLS 加载 server 证书
  → ca.crt 写入各 Webhook CABundle
```

| 步骤 | 说明 |
|------|------|
| 1. openssl | `build/admission/gen-admission-secret.sh`：CA + server 证书；SAN = `service` / `service.ns` / `service.ns.svc` |
| 2. Secret | 默认名 `kubeedge-admission-secret` |
| 3. Volume | 只读挂载到 `/admission.local.config/certificates` |
| 4. 运行时 | `--tls-cert-file` / `--tls-private-key-file` / `--ca-cert-file` |

备选：若未传 tls 文件，则尝试用 kubeconfig 里的 CertData/KeyData。  
ClusterRole 含 CSR 权限，但当前脚本走本地 openssl，未必走 CSR 审批路径。

---

## 6. 部署链路

| 组件 | 裸 YAML | Helm 模板 | 默认开关 | SA | 对外入口 |
|------|---------|-----------|----------|-----|----------|
| Admission | `build/admission/*.yaml` | `deployment_admission` / `service_admission` / `rbac_admission` | `enable: false` | `kubeedge-admission` | `kubeedge-admission-service:443` |
| Controller Manager | `build/controllermanager/*.yaml` | `deployment_controllermanager` / `rbac_controllermanager` | `enable: false` | `controller-manager` | 无 Webhook Service（仅 health `:9001`） |

### Admission 部署步骤（`build/admission/README.md`）

1. `make admissionimage`
2. `cd build/admission && ./gen-admission-secret.sh`
3. `kubectl create -f` 各 yaml
4. `kubectl get pods -n kubeedge` 确认运行

### RBAC 要点（Admission）

- 可 get/list/watch/create/update：`mutatingwebhookconfigurations`、`validatingwebhookconfigurations`
- 可读 rules / devices / operations 相关 CR（业务校验用）
- 含 CSR、secrets 相关权限（证书路径相关）

### 合并后需要补齐

- WebhookServer
- 共用 / 迁入证书策略
- 合并 RBAC
- 单一 Service 路径

---

## 7. 现有单测

| 文件 | 覆盖 |
|------|------|
| `admit_device_test.go` | Device |
| `admit_devicemodel_test.go` | DeviceModel |
| `admit_ruleendpoint_test.go` | RuleEndpoint |
| `admit_nodeupgradejob_test.go` | NodeUpgradeJob |
| `common_test.go` | 注册 Create/Update |

缺口：

- 未见 `admit_rule` / `mutate_offlinemigration` 独立单测
- 未见 envtest 级 Webhook 集成测试

---

## 8. 设计冻结建议记下的结论

1. **目标形态**：Admission Handler 迁入 CM 的 controller-runtime WebhookServer，消灭独立 admission Deployment（或过渡期双写）。
2. **必须保留的业务语义**：上表 7 条路径的校验 / 默认值 / OfflineMigration toleration 与 ObjectSelector。
3. **版本债**：NodeUpgradeJob Webhook 仍为 v1alpha1，与 CM v1alpha2 控制器不一致——迁移时要明确是否一并升级。
4. **证书**：今天靠离线 openssl Secret；迁入后是否改用 cert-manager / webhook 证书轮换需单独决策。
5. **工具链旁注**（并行项）：详见 [Version-Compatibility-Matrix.md](./Version-Compatibility-Matrix.md)。冻结候选：`go 1.23.12` + K8s `v0.32.10` + **controller-runtime `v0.20.4`** + **controller-gen `v0.17.3`** + **setup-envtest `@release-0.20`**（Envtest binaries `1.32.x`）。

---

## 关键路径速查

```
cloud/cmd/admission/
cloud/cmd/controllermanager/
cloud/pkg/admissioncontroller/
cloud/pkg/controllermanager/
build/admission/
build/controllermanager/
manifests/charts/cloudcore/templates/deployment_admission.yaml
manifests/charts/cloudcore/templates/service_admission.yaml
manifests/charts/cloudcore/templates/rbac_admission.yaml
manifests/charts/cloudcore/templates/deployment_controllermanager.yaml
manifests/charts/cloudcore/templates/rbac_controllermanager.yaml
manifests/charts/cloudcore/values.yaml
```
