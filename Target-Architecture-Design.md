# 目标架构与迁移设计（第 1 周冻结）

> 依据：`KubeEdge_Controller_Admission_统一改造技术方案.docx` §4 / §5 / §9  
> 对齐基线：[Week1-Admission-Baseline.md](./Week1-Admission-Baseline.md)  
> 对齐版本：[Version-Compatibility-Matrix.md](./Version-Compatibility-Matrix.md)  
> 状态：**设计冻结候选**（评审后不再随意扩大范围）

---

## 0. 一句话目标

把独立 Admission（手写 TLS HTTP + 启动时动态注册）迁入现有 **Controller Manager** 的 **controller-runtime WebhookServer**，由**同一进程**承载 Reconcile 与 7 个 Admission Handler；WebhookConfiguration **声明化**；证书/Service/RBAC/Helm 统一；业务规则**等价迁移**。

```
现状：
  [Controller Manager] ── reconcile only
  [Admission] ── TLS:443 + 动态注册 WebhookConfiguration

目标：
  [Controller Manager]
      ├── Controllers (reconcile, 可 Leader)
      └── WebhookServer (7 paths, 每副本可服务)
  [声明式 Validating/MutatingWebhookConfiguration]
      └── clientConfig.service → kubeedge-admission-service
            └── selector → Controller Manager Pods
  [旧 Admission] 仅过渡期保留，验证通过后删除
```

---

## 1. 目标架构

### 1.1 Manager 职责边界

| 能力 | 目标实现 | 冻结约束 |
|------|----------|----------|
| Controllers | 继续 `SetupWithManager` | 不因 Webhook 迁移重写业务 reconcile |
| Webhook Server | Manager 注册 7 个 Handler | **每 Ready 副本都可服务**，不依赖成为 Leader |
| Scheme | core + devices + rules + operations（含 admission API） | Decode Object/OldObject 前必须完整 |
| Client | 优先 `mgr.GetClient()` | 旧 typed/versioned client 可先 Adapter，禁止同 PR 大重写 |
| Cache | Manager Cache 优先；现有独立 Node Cache **先保留** | 未验证禁止删除 |
| Health / Ready | 存活 ≠ 就绪；Ready 含证书可读 + Webhook 已监听 | FailurePolicy=Fail 时 Ready 尤其关键 |
| Lifecycle | Manager 统一启动 / Context 取消 / 优雅退出 | 禁止再挂独立不可控 `http.Server` |
| Metrics / Log | 统一 Logger；补 Webhook 计数/延迟 | 禁止打敏感对象全文 |

### 1.2 请求处理链路（目标）

```
API Server
  → Validating/MutatingWebhookConfiguration
  → Service (kubeedge-admission-service)
  → Controller Manager Pod :webhook-port (TLS)
  → controller-runtime WebhookServer
  → handlers/* (解码 AdmissionReview)
  → validation/* 或 mutation/*（纯业务）
  → AdmissionResponse（Allowed / Denied / Errored / Patch）
```

路径**保持兼容**（与现状一致）：

| Path | 类型 |
|------|------|
| `/devices` | Validating |
| `/devicemodels` | Validating |
| `/rules` | Validating |
| `/ruleendpoints` | Validating |
| `/nodeupgradejobs` | Validating |
| `/offlinemigration` | Mutating |
| `/mutating/nodeupgradejobs` | Mutating |

### 1.3 WebhookConfiguration 声明化

- **禁止**进程启动时 Create/Update WebhookConfiguration（删除 `registerWebhooks` 运行时逻辑）
- 改为安装期可审查 YAML（优先 controller-tools Marker；表达不全则手写显式 YAML）
- 必须保持字段：name、webhook name、rules、service、failurePolicy、sideEffects、objectSelector（OfflineMigration）、admissionReviewVersions=`v1`、caBundle（安装注入）

现有三个对象名称建议过渡期保留，避免升级时大范围 rename：

1. `kubeedge-crds-validate-webhook-configuration`
2. `mutate-offlinemigration`
3. `kubeedge-mutating-webhook`

---

## 2. 包结构（目标）

原则：**传输层适配** 与 **纯业务校验/Mutation** 分离；禁止继续把 TLS、注册、7 个 Handler 堆在单个 `admission.go`。

```
cloud/pkg/controllermanager/
  controllermanager.go     # NewManager：Scheme、Health、Controllers、setupWebhooks
  webhook.go               # WebhookServer 端口/CertDir、Register 7 路径

cloud/pkg/admissioncontroller/
  handlers/                # Admission 适配层（解码 Request → 调业务 → 构造 Response）
    device.go
    devicemodel.go
    rule.go
    ruleendpoint.go
    nodeupgradejob.go      # validating + mutating 可同文件或拆分
    offlinemigration.go
  validation/              # 无 HTTP 依赖的纯校验（从旧 admit_* 迁出）
  mutation/                # 无 HTTP 依赖的 Patch 逻辑
  response.go              # Allowed / Denied / Errored 辅助
  scheme.go                # Webhook 解码所需 Scheme

cloud/cmd/controllermanager/   # 唯一生产入口（最终）
cloud/cmd/admission/           # 过渡期保留；清理阶段删除
```

目录名可随 Maintainer 评审微调，但**分层义务**不变。

### Manager Webhook 配置冻结值

| 配置 | 建议值 | 说明 |
|------|--------|------|
| Webhook Port | 容器内 **9443**（或与 Service targetPort 一致） | 现状 Service 是 443→443；合并后推荐 Service `443 → targetPort 9443` |
| CertDir | `/admission.local.config/certificates`（可复用现路径） | 只读挂载 |
| CertName / KeyName | `tls.crt` / `tls.key` | 与 Secret key 对齐 |
| CA 文件 | `ca.crt`（用于安装期注入 caBundle，不必进 Server 读） | |
| HealthProbe | 沿用 `:9001` | Ready 增加 webhook/cert 检查 |
| LeaderElection | Controllers 可用；Webhook **不受锁阻塞** | 见 §4 |

伪代码形态（以 CR v0.20 API 为准）：

```go
func setupWebhooks(mgr manager.Manager) error {
    server := mgr.GetWebhookServer()
    server.Register("/devices", &webhook.Admission{Handler: newDeviceValidator(mgr.GetClient())})
    // ... 其余 6 路径
    return nil
}
```

初始化失败必须在 `mgr.Start` 前返回错误。

---

## 3. 证书设计

### 3.1 要求

| 要求 | 说明 |
|------|------|
| SAN | 覆盖 Service DNS：`svc`、`svc.ns`、`svc.ns.svc` |
| 挂载 | Secret 只读挂载到 CertDir |
| CABundle | 与签发 Server 证书的 CA **一致** |
| Ready | 证书缺失/过期/不可读 → Ready 失败 + 明确日志 |
| 轮换 | 本阶段：**更新 Secret → 滚动重启 Pod** 生效（不强制热加载） |

### 3.2 方案选择（冻结）

| 方案 | 结论 |
|------|------|
| 沿用现有 `gen-admission-secret.sh` / openssl → Secret | **基础方案（必须支持）** |
| cert-manager | **可选**，不作为硬依赖 |
| 进程内动态签发 / CSR 运行时注册 | **不做**（与声明化目标冲突） |

Secret 建议名：过渡期可继续 `kubeedge-admission-secret`；最终可改名为 `kubeedge-controller-manager-webhook-certs`，但切换时必须同步 caBundle。

### 3.3 高风险

`FailurePolicy=Fail` 的 Webhook（Rule / RuleEndpoint / NodeUpgradeJob validating）在证书或 Service 不可用时会**阻塞**对应资源写操作 → 必须具备一键回滚（§6）。

---

## 4. Service 设计

### 4.1 目标形态

| 项 | 值 |
|----|-----|
| Service 名 | **保持** `kubeedge-admission-service`（减少 WebhookConfiguration 大改） |
| Namespace | `kubeedge`（或 Release Namespace，与 Chart 一致） |
| Port | `443` |
| TargetPort | Webhook 容器端口（建议 `9443`） |
| Selector | **指向 Controller Manager Pod**（不再指向旧 admission Deployment） |

### 4.2 切换约束

1. 切换阶段**禁止**旧、新后端同时被同一 WebhookConfiguration 命中（重复校验/Mutation）。
2. Endpoint 必须 Ready；DNS 与证书 SAN 一致。
3. Helm Values 增加：`webhook.enable`、端口、`certsSecretName`、是否保留旧 admission。

### 4.3 RBAC（方向）

- 从旧 Admission ClusterRole **按实际 Client 调用最小化搬迁**到 `controller-manager` SA
- 需要的只读能力示例：`rules`、`ruleendpoints`、`devices`/`devicemodels`、`nodeupgradejobs` 等
- **不再需要**运行时写 `validatingwebhookconfigurations` / `mutatingwebhookconfigurations`（声明化后）
- 禁止“整份 Admission ClusterRole 原样复制”

---

## 5. Leader Election 设计

### 5.1 原则（硬性）

| 组件 | Leader Election |
|------|-----------------|
| Reconcile Controllers | **可以**只在 Leader 跑（若启用） |
| Webhook Server | **必须**在每个 Ready 副本上跑，**不得**因未获锁而不监听 |

### 5.2 实现注意（CR v0.20）

- 接入 LeaderElection 时，验证 WebhookServer 作为 Runnable **不** `NeedLeaderElection()==true`
- Service 只转发到 **Ready** 副本
- 验收：双副本 + Leader 切换期间，Admission 请求持续成功（E09）

### 5.3 现状说明

当前 CM **未**显式开 LeaderElection；合并 Webhook 后若开启，必须满足 §5.1，否则禁止在生产开多副本。

---

## 6. 升级与回滚设计

### 6.1 推荐切换顺序（双轨过渡）

| 步骤 | 动作 | 旧 Admission |
|------|------|--------------|
| 1 | 部署带 WebhookServer 的新 CM（挂证书、暴露端口） | **仍服务**；WebhookConfiguration **仍指向旧** |
| 2 | 用独立测试入口 / 临时 Service 验证新 Server TLS、Path、Handler | 不动 |
| 3 | 创建或切换 Service Selector → CM Pods；核对 Endpoint / SAN | 暂时并存但**不要**被 Configuration 指向 |
| 4 | 更新 WebhookConfiguration `clientConfig` → 新 Service；确认无重复命中 | 保留但流量已切走 |
| 5 | 跑 7 类 Webhook 回归 + 业务冒烟 | 待命回滚 |
| 6 | 停止旧 Admission Deployment；Manifest 留作快速恢复 | 停止 |
| 7 | 稳定观察 + E2E 通过后，删除旧代码/镜像/Chart | 清理 |

**硬性门槛：** 未完成行为对照、Envtest、E2E、升级与回滚演练前，**不得**删除旧 Admission。

### 6.2 回滚触发条件

满足任一即回滚：

1. API Server 调 Webhook 持续 TLS / 超时 / 5xx  
2. 核心资源出现与旧版不一致的误拒绝或误放行  
3. Mutating Patch 不可应用、非幂等或覆盖用户显式值  
4. 多副本 / Leader 切换 / 滚动升级造成明显不可用窗口  
5. Helm 升级后 Service、Secret、RBAC、CABundle 状态不一致  

### 6.3 回滚动作（按序）

1. 恢复旧 Admission Deployment 与 Service Endpoint  
2. 恢复旧 Validating/MutatingWebhookConfiguration（或改回指向旧 Service）  
3. 确认旧证书与 CABundle 匹配  
4. 暂停或缩容新 Webhook Server 副本  
5. 对核心资源做创建/更新冒烟  
6. 保留失败日志、Admission UID、配置快照复盘  

**紧急恢复：** 预先准备并测试「一键恢复」Manifest/脚本，随升级文档交付；禁止故障现场临时拼装。

### 6.4 Helm 过渡开关（建议 Values）

```yaml
controllerManager:
  enable: true
  webhook:
    enable: true
    port: 9443
    certsSecretName: kubeedge-admission-secret

admission:
  enable: true   # 过渡期 true；切换稳定后 false；清理阶段删除该段
```

升级文档必须写清：先开 CM webhook 能力 → 再切 Configuration → 再关旧 admission。

---

## 7. 与现状差异速查（评审用）

| 维度 | 现状 | 目标 |
|------|------|------|
| 进程 | CM + Admission 两套 | 仅 CM（过渡期双轨） |
| HTTP | 手写 `ListenAndServeTLS` | controller-runtime WebhookServer |
| 注册 | 启动时动态 Create/Update | 声明式 Manifest |
| Service | 指向 admission Pod | 指向 CM Pod |
| 证书 | admission Secret | 同 Secret 挂到 CM（方案 A） |
| Leader | 未用 / 或仅 Controller | Webhook 永不绑 Leader |
| 端口 | 443 | 建议容器 9443，Service 仍对外 443 |
| 业务规则 | 7 Handler | **等价**，不改 FailurePolicy/语义 |
| NodeUpgradeJob API | Admission v1alpha1 | 过渡期保持；是否升 v1alpha2 **另开议题，不塞本冻结** |

---

## 8. 明确不纳入本设计冻结

- 升 Kubernetes 大版本  
- 改 Admission 业务规则 / FailurePolicy  
- 强制 cert-manager  
- 立刻删除独立 Node Cache  
- 无关全仓目录重构 / 无关依赖升级  

---

## 9. 设计冻结验收清单

- [ ] 目标进程模型与包结构已评审  
- [ ] 7 Path / Service 名 / Configuration 名保留策略已确认  
- [ ] 证书方案 A（现有 openssl Secret）确认为必选  
- [ ] Leader Election 与 Webhook 解耦原则已确认  
- [ ] 升级 7 步 + 回滚触发/动作已确认  
- [ ] 旧组件删除门槛已确认（测试通过后才清理）  

评审通过后，下一阶段按 PR 拆分进入第 2 周：升 controller-runtime + 接入空 WebhookServer。
