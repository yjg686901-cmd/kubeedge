# 版本兼容矩阵（第 1 周）

> 依据：`KubeEdge_Controller_Admission_统一改造技术方案.docx` §3.1 / §5.1.1  
> 与仓库现状：`go.mod`、`hack/generate-crds.sh`、`cloud/test/integration/scripts/execute.sh`  
> 上游兼容表：controller-runtime / controller-tools README（按 k8s.io/* 次版本一一对应）

---

## 选择规则（方案原文）

目标不是「最新」，而是：

**与 KubeEdge 当前 Kubernetes 依赖兼容、可构建、可生成、可测试、可发布。**

硬约束：

- **不升级** Kubernetes 大版本（保持 **v0.32.x**）
- Go 基线保持 **1.23.12**（本阶段不升 1.24+）
- 最终小版本以 `go build` / `go test` / 代码生成 / Envtest / Vendor 通过为准

---

## 1. 仓库现状 vs 候选

| 组件 | 现状 | 候选（对齐 K8s v0.32） | 结论 |
|------|------|------------------------|------|
| Go | **1.23.12** | 1.23.12 | **冻结，保持** |
| Kubernetes (`k8s.io/*`) | **v0.32.10**（replace → kubeedge staging `v1.32.10-kubeedge1`） | v0.32.x | **冻结，不升大版本** |
| controller-runtime | **v0.19.7**（上游测的是 k8s **v0.31**） | **v0.20.x**，首选 **v0.20.4** | **应升级**（落后一整条兼容线） |
| controller-tools / controller-gen | 脚本已 pin **v0.17.3**（CRD 注解同） | **v0.17.x**，保持 **v0.17.3** | **已对齐，保持** |
| setup-envtest（工具） | `@latest`（未锁定） | **`@release-0.20`**（与 CR 0.20 同分支） | **应锁定** |
| Envtest K8s 二进制 | **`1.29.0`**（过旧） | **`1.32.x`**（与主仓 k8s 次版本一致，如 `1.32.0`） | **应升级** |

---

## 2. 上游兼容矩阵（参考）

### 2.1 controller-runtime ↔ client-go / Go

来源：https://pkg.go.dev/sigs.k8s.io/controller-runtime（Compatibility 表）

| controller-runtime | k8s.io/* / client-go | 最低 Go |
|--------------------|----------------------|---------|
| v0.21 | v0.33 | 1.24 |
| **v0.20** | **v0.32** | **1.23** |
| v0.19 | v0.31 | 1.22 |
| v0.18 | v0.30 | 1.22 |

实测 go.mod：

- `controller-runtime@v0.20.4` → `k8s.io/client-go v0.32.1`，`go 1.23.0`
- `controller-runtime@v0.19.7` → `k8s.io/client-go v0.31.0`，`go 1.22.0`

### 2.2 controller-tools ↔ client-go / Go

来源：https://github.com/kubernetes-sigs/controller-tools README

| controller-tools | k8s.io/* / client-go | 最低 Go |
|------------------|----------------------|---------|
| v0.18+ | ≥ v0.33（勿选） | ≥ 1.24 |
| **v0.17** | **v0.32** | **1.23** |
| v0.16 | v0.31 | 1.22 |

实测：`controller-tools@v0.17.3` → `k8s.io/api v0.32.2`，`go 1.23.0`。

### 2.3 setup-envtest

| 项 | 建议 |
|----|------|
| 安装源 | `go install sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.20` |
| 下载的 apiserver/etcd 版本 | `setup-envtest use 1.32.x`（与主仓次版本一致） |
| 禁止 | `@latest` 无 pin；继续用 `1.29.0` |

说明：Envtest 二进制现由 controller-tools releases 提供；`release-0.20` 的 setup-envtest 与 CR v0.20 配套。

---

## 3. 截图里旧矩阵怎么理解

文档附图里的 **K8s 1.23–1.26 ↔ CR 0.11–0.14** 是历史对照，**不是本项目目标**。

同一份技术方案正文已写明：

- 基线：Go 1.23.12 / K8s **v0.32.10** / CR **v0.19.7**
- 候选：CR **v0.20.x**（因 K8s 为 v0.32.x）

本仓库执行时以正文 §3.1 / §5.1 与上表为准，忽略附图旧矩阵。

---

## 4. 本项目冻结候选（可写入设计冻结）

| 组件 | 冻结候选 | 备注 |
|------|----------|------|
| Go | **1.23.12** | 已统一；不追 1.24+ |
| k8s.io/* | **v0.32.10** | 不升大版本 |
| controller-runtime | **v0.20.4**（线：v0.20.x） | 需 PR：依赖 + 兼容 API 编译修复 |
| controller-gen | **v0.17.3** | `hack/generate-crds.sh` 已正确；禁止升到 v0.18+ |
| setup-envtest | **release-0.20** | 改掉 `@latest` |
| Envtest binaries | **1.32.x** | 改掉 `execute.sh` 里的 `1.29.0` |

**不选：**

| 版本 | 原因 |
|------|------|
| CR v0.21+ | 对 k8s v0.33+，会逼升 K8s 依赖线 |
| CT v0.18+ | 对 k8s ≥ v0.33，同上 |
| 维持 CR v0.19.7 | 与主仓 K8s v0.32 **官方不匹配**（上游测的是 v0.31） |

---

## 5. 缺口与待改入口

| 位置 | 问题 | 建议动作 |
|------|------|----------|
| `go.mod` | CR `v0.19.7` | 升到 `v0.20.4`，tidy + vendor |
| `hack/generate-crds.sh` | 已是 `controller-gen@v0.17.3` | 保持；文档写死禁止追新 |
| `cloud/test/integration/scripts/execute.sh` | `setup-envtest@latest` + `use 1.29.0` | 改为 `@release-0.20` + `use 1.32.0`（或与 patch 对齐的 1.32.x） |
| CRD YAML 注解 | `controller-gen.kubebuilder.io/version: v0.17.3` | 与工具一致，无需因 CR 升级而改 |

---

## 6. 升级顺序（方案 §5.1.2）

1. 锁定 Kubernetes **v0.32.x** 不变  
2. 升级 **controller-runtime → v0.20.4** 及必要直接依赖  
3. 处理编译 API 变化（不做业务重构）  
4. 确认 **controller-gen 保持 v0.17.3**  
5. 锁定 **setup-envtest@release-0.20**，Envtest 二进制 **1.32.x**，跑最小 Webhook Envtest  
6. `go mod tidy` / vendor / 生成文件 / 相关测试

---

## 7. 验收命令（可勾选）

```bash
go version
go list -m sigs.k8s.io/controller-runtime
go list -m k8s.io/client-go

# 工具版本（安装后）
controller-gen --version   # 期望 v0.17.3
setup-envtest version      # 来自 release-0.20

# 构建与测试
go build ./cloud/cmd/controllermanager
go build ./cloud/cmd/admission
go test ./cloud/pkg/controllermanager/...
go test ./cloud/pkg/admissioncontroller/...
```

通过标准：

- [ ] `controller-runtime` 为 v0.20.x，且未把 `k8s.io/*` 拉到 v0.33+
- [ ] `controller-gen` 仍为 v0.17.3
- [ ] Envtest 使用 1.32.x 二进制可启动
- [ ] Controllermanager / Admission 相关包编译与既有单测通过

---

## 8. 一句话结论

**本阶段兼容线冻结为：Go 1.23.12 + K8s v0.32.10 + controller-runtime v0.20.4 + controller-gen v0.17.3 + setup-envtest@release-0.20（Envtest binaries 1.32.x）。**

其中 controller-gen 已对齐；真正要动的是 **升 CR 到 0.20.4**，以及 **锁死 setup-envtest / 升 Envtest 二进制**。
