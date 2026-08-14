# 旧 Admission 行为基线与 Webhook 配置快照

> 第 1 周交付物：固定旧实现行为，供后续新旧对照测试使用。  
> 代码来源：`cloud/pkg/admissioncontroller/`（以本仓库当前实现为准）  
> 对照文档：[Week1-Admission-Baseline.md](./Week1-Admission-Baseline.md)、[Target-Architecture-Design.md](./Target-Architecture-Design.md)

---

## 目录结构

```
admission-baseline/
  snapshots/          # WebhookConfiguration 声明式快照（由代码导出，caBundle 占位）
  cases.yaml          # 行为用例清单（输入约定 + 期望 Allowed/Message/Patch）
  fixtures/           # 固定 AdmissionReview / Object JSON
```

**使用方式（后续周）：**

1. 用同一 `fixtures/*` 分别调用旧 Handler 与新 Handler  
2. 比较 `Allowed`、`UID`、`Status.Message`、`PatchType`/`Patch`  
3. 用 `snapshots/*` diff 声明化后的 Manifest（除 caBundle 外应一致）

---

## 1. Webhook 配置快照摘要

默认假设：`namespace=kubeedge`，`service=kubeedge-admission-service`，`port=443`。  
`caBundle` 在快照中为 `Cg==`（占位，真实值由安装证书注入）。

### 1.1 ValidatingWebhookConfiguration

**Name:** `kubeedge-crds-validate-webhook-configuration`

| webhook name | path | group/version/resource | operations | failurePolicy | sideEffects | objectSelector |
|--------------|------|------------------------|------------|---------------|-------------|----------------|
| `validatedevice.kubeedge.io` | `/devices` | devices.kubeedge.io/v1beta1/devices | CREATE, UPDATE | Ignore | None | — |
| `validatedevicemodel.kubeedge.io` | `/devicemodels` | devices.kubeedge.io/v1beta1/devicemodels | CREATE, UPDATE | Ignore | None | — |
| `validatedrule.kubeedge.io` | `/rules` | rules.kubeedge.io/v1/rules | CREATE, UPDATE, DELETE | Fail | None | — |
| `validatedruleendpoint.kubeedge.io` | `/ruleendpoints` | rules.kubeedge.io/v1/ruleendpoints | CREATE, UPDATE, DELETE | Fail | None | — |
| `validatenodeupgradejob.kubeedge.io` | `/nodeupgradejobs` | operations.kubeedge.io/v1alpha1/nodeupgradejobs | CREATE, UPDATE, DELETE | Fail | None | — |

文件：`admission-baseline/snapshots/validatingwebhookconfiguration-kubeedge-crds.yaml`

### 1.2 MutatingWebhookConfiguration

**Name:** `mutate-offlinemigration`

| webhook name | path | resource | operations | failurePolicy | objectSelector |
|--------------|------|----------|------------|---------------|----------------|
| `mutateofflinemigration.kubeedge.io` | `/offlinemigration` | /v1/pods | CREATE, UPDATE | Ignore | `app-offline.kubeedge.io=autonomy` |

文件：`admission-baseline/snapshots/mutatingwebhookconfiguration-mutate-offlinemigration.yaml`

**Name:** `kubeedge-mutating-webhook`

| webhook name | path | resource | operations | failurePolicy |
|--------------|------|----------|------------|---------------|
| `mutatingnodeupgradejob.kubeedge.io` | `/mutating/nodeupgradejobs` | operations.kubeedge.io/v1alpha1/nodeupgradejobs | CREATE, UPDATE | Ignore |

文件：`admission-baseline/snapshots/mutatingwebhookconfiguration-kubeedge-mutating-webhook.yaml`

### 1.3 公共字段（全部 hooks）

- `admissionReviewVersions: ["v1"]`
- `sideEffects: None`
- `timeoutSeconds`: **未设置**（走 API 默认）
- `namespaceSelector`: 未设置
- `matchPolicy`: 未设置（默认 Equivalent）

---

## 2. 行为基线用例总表

对照字段（方案 §8.2）：`Allowed`、`UID`、`Status.Message`（语义）、`PatchType`/`Patch`。

图例：`✓` = 已有单测或夹具；`⚠` = 需 Client/集群依赖；`○` = 仅文档约定。

### 2.1 Device — `/devices` — `admitDevice`

| ID | 场景 | Op | 期望 Allowed | 期望 Message / 备注 | 夹具 | 现有测试 |
|----|------|----|--------------|---------------------|------|----------|
| DEV-01 | 合法创建，属性名唯一 | CREATE | true | — | `fixtures/device-create-valid.json` | ✓ |
| DEV-02 | 重复属性名 | CREATE | false | `property names must be unique.` | `fixtures/device-create-dup-props.json` | ✓ |
| DEV-03 | 合法更新 | UPDATE | true | 同 DEV-01 结构 | cases.yaml | ✓ 逻辑同 Create |
| DEV-04 | DELETE | DELETE | true | 无校验 | cases.yaml | ✓ |
| DEV-05 | 未知 Operation | OTHER | false | `Unsupported webhook operation!` | cases.yaml | ✓ |

### 2.2 DeviceModel — `/devicemodels` — `admitDeviceModel`

| ID | 场景 | Op | Allowed | Message | 夹具 | 测试 |
|----|------|----|---------|---------|------|------|
| DM-01 | 合法创建 | CREATE | true | — | `fixtures/devicemodel-create-valid.json` | ✓ |
| DM-02 | 重复属性名 | CREATE | false | `property names must be unique.` | `fixtures/devicemodel-create-dup-props.json` | ✓ |
| DM-03 | DELETE | DELETE | true | — | cases.yaml | ✓ |
| DM-04 | 未知 Operation | OTHER | false | `Unsupported webhook operation!` | cases.yaml | ✓ |

### 2.3 RuleEndpoint — `/ruleendpoints` — `admitRuleEndpoint`

| ID | 场景 | Op | Allowed | Message（含） | 夹具 | 测试 |
|----|------|----|---------|---------------|------|------|
| RE-01 | servicebus + 合法 port | CREATE | true | — | `fixtures/ruleendpoint-servicebus-valid.json` | ✓ |
| RE-02 | servicebus 缺 service_port | CREATE | false | `service_port` property missed | cases.yaml | ✓ |
| RE-03 | port 非整数 | CREATE | false | `port should be integer` | cases.yaml | ✓ |
| RE-04 | port 越界 | CREATE | false | `port must be in range 1-65535` | `fixtures/ruleendpoint-servicebus-bad-port.json` | ✓ |
| RE-05 | UPDATE | UPDATE | false | `unsupported webhook operation Update` | cases.yaml | ✓ |
| RE-06 | DELETE | DELETE | true | — | cases.yaml | ✓ |
| RE-07 | 非法 JSON | CREATE | false | decode error | cases.yaml | ✓ |

### 2.4 Rule — `/rules` — `admitRule`（⚠ 依赖 CrdClient）

| ID | 场景 | Op | Allowed | Message（语义） | 依赖 | 测试 |
|----|------|----|---------|-----------------|------|------|
| RULE-01 | source/target 存在且类型配对合法 | CREATE | true | — | 预置 RuleEndpoint | ○ 无单测 |
| RULE-02 | source RuleEndpoint 不存在 | CREATE | false | can't get source / has not been created | Client | ○ |
| RULE-03 | 类型配对不在允许表 | CREATE | false | not validate（source→target 类型） | Client | ○ |
| RULE-04 | rest source 缺 path | CREATE | false | `"path" property missed` | Client | ○ |
| RULE-05 | DELETE | DELETE | true | — | — | ○ 代码放行 |
| RULE-06 | UPDATE | UPDATE | false | unsupported webhook operation | — | ○ |

允许的 source→target 类型对（代码硬编码）：

- rest → eventbus  
- rest → servicebus  
- eventbus → rest  

### 2.5 NodeUpgradeJob Validating — `/nodeupgradejobs` — `admitNodeUpgradeJob`

| ID | 场景 | Op | Allowed | Message（含） | 夹具 | 测试 |
|----|------|----|---------|---------------|------|------|
| NU-V-01 | 合法 Create（version+nodeNames） | CREATE | true | — | `fixtures/nodeupgrade-create-valid.json` | ✓ |
| NU-V-02 | version 无 v 前缀 | CREATE | false | `invalid version` | cases.yaml | ✓ |
| NU-V-03 | 非法 semver | CREATE | false | `invalid version` | cases.yaml | ✓ |
| NU-V-04 | 非法 image | CREATE | false | `invalid image repo` | cases.yaml | ✓ |
| NU-V-05 | 无 NodeNames 且无 LabelSelector | CREATE | false | `both NodeNames and LabelSelector are NOT specified` | cases.yaml | ✓ |
| NU-V-06 | 同时指定两者 | CREATE | false | `both NodeNames and LabelSelector are specified` | cases.yaml | ✓ |
| NU-V-07 | Update 不改 Spec | UPDATE | true | — | cases.yaml | ✓ |
| NU-V-08 | Update 改 Spec | UPDATE | false | `spec fields are not allowed to update once it's created` | `fixtures/nodeupgrade-update-spec-change.json` | ✓ |
| NU-V-09 | DELETE | DELETE | true | — | cases.yaml | ✓ |
| NU-V-10 | 空 image 允许 | CREATE | true | — | cases.yaml | ✓ |
| NU-V-11 | 合法 image repo | CREATE | true | — | cases.yaml | ✓ |

### 2.6 OfflineMigration Mutating — `/offlinemigration` — `mutateOfflineMigration`

| ID | 场景 | Op | Allowed | Patch | 夹具 | 测试 |
|----|------|----|---------|-------|------|------|
| OM-01 | 无 tolerations | CREATE | true | replace `/spec/tolerations`，追加 unreachable Exists | `fixtures/offlinemigration-pod-no-toleration.json` | ○ 无独立单测 |
| OM-02 | 已有其他 toleration | CREATE | true | 保留其他 + 追加/规范化 unreachable | cases.yaml | ○ |
| OM-03 | 已有 unreachable（任意形式） | CREATE | true | 去掉旧 unreachable 再追加 Exists（幂等目标） | cases.yaml | ○ |
| OM-04 | ObjectSelector 未命中 | — | API Server 不调用 | — | 配置层 | ○ |

ObjectSelector（配置层）：`app-offline.kubeedge.io=autonomy`

### 2.7 NodeUpgradeJob Mutating — `/mutating/nodeupgradejobs` — `mutatingNodeUpgradeJob`

| ID | 场景 | Allowed | Patch | 夹具 | 测试 |
|----|------|---------|-------|------|------|
| NU-M-01 | concurrency/timeout 均未设 | true | add `/spec/concurrency`=1；add `/spec/timeoutSeconds`=300 | `fixtures/nodeupgrade-mutate-defaults.json` | ✓ |
| NU-M-02 | 两者均已设 | true | 空 Patch（无 Patch 字段） | cases.yaml | ✓ |
| NU-M-03 | 仅缺 concurrency | true | 只 add concurrency=1 | cases.yaml | ✓ |
| NU-M-04 | 仅缺 timeoutSeconds | true | 只 add timeoutSeconds=300 | cases.yaml | ✓ |

---

## 3. 已知「注册 vs 业务」差异（对照时勿当回归）

这些是**旧实现既有行为**，冻结进基线；迁移时默认保持等价，除非单独 Issue 批准改语义。

| 项 | 配置 Rules | Handler 实际 |
|----|------------|--------------|
| Rule UPDATE | 已注册 | 返回 unsupported（Allowed 语义上拒绝/错误） |
| RuleEndpoint UPDATE | 已注册 | 同上 |
| Rule/RuleEndpoint DELETE | 已注册 | 直接放行 |
| Device/DeviceModel DELETE | 未在 Rules 中 | Handler 若被调则放行 |
| NodeUpgradeJob API | v1alpha1 | CM 控制器已用 v1alpha2（版本债，不在本基线改） |

---

## 4. 对照测试最小断言模板

```text
给定 fixture F、旧函数 old()、新函数 new()：
  oldResp = old(F)
  newResp = new(F)
  assert oldResp.Allowed == newResp.Allowed
  assert newResp.UID == F.Request.UID          # 框架应回填
  if !Allowed:
    assert message 语义等价（允许标点/包装差异时在 PR 说明）
  if Mutating && needsPatch:
    assert PatchType == JSONPatch
    assert Patch 应用到原对象后 DeepEqual 期望对象
  if Mutating && !needsPatch:
    assert Patch 为空或不存在
```

机器可读清单：`admission-baseline/cases.yaml`

---

## 5. 快照刷新方式

代码变更 `registerWebhooks` 后：

1. 更新 `admission-baseline/snapshots/*.yaml`  
2. 更新本文件摘要表  
3. 在迁移 PR 中解释 diff（除 caBundle / 生成注解外）

导出参考命令（集群已部署旧 Admission 时）：

```bash
kubectl get validatingwebhookconfiguration kubeedge-crds-validate-webhook-configuration -o yaml
kubectl get mutatingwebhookconfiguration mutate-offlinemigration -o yaml
kubectl get mutatingwebhookconfiguration kubeedge-mutating-webhook -o yaml
```

无集群时以本目录快照（从源码导出）为准。

---

## 6. 第 1 周完成勾选

- [x] WebhookConfiguration 三份快照  
- [x] 7 类 Handler 行为用例清单  
- [x] 关键路径固定 JSON 夹具  
- [x] 记录注册/业务差异与 Client 依赖用例  
- [ ] （后续）Envtest/对照测试加载 `cases.yaml` 自动化执行  
