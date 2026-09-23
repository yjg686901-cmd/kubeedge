# 第五周集群验收命令（由测试人员在主节点执行）

本页针对现有两节点集群的预览 Service。命令只作用于带 `week5.kubeedge.io/business-test=true` 标签的测试对象；NodeUpgradeJob 仅做 server-side dry run，避免触发真实节点升级。每个编号的终端输出可分别截图。执行前确认当前 kubeconfig context 是预期的测试集群。

## A. 前置检查

```bash
kubectl config current-context
kubectl -n kubeedge get deployment kubeedge-controller-manager
kubectl -n kubeedge get endpoints kubeedge-controller-manager-webhook-preview
test -s /home/ubuntu/week5-transfer/preview-certs/ca.crt && echo 'preview CA exists'
openssl x509 -in /home/ubuntu/week5-transfer/preview-certs/tls.crt -checkend 3600 -noout
kubectl get crd devices.devices.kubeedge.io devicemodels.devices.kubeedge.io rules.rules.kubeedge.io ruleendpoints.rules.kubeedge.io nodeupgradejobs.operations.kubeedge.io
```

仅当 Deployment 为 2/2 Ready、Service 有两个 Endpoint、CRD 全部存在时继续。以下注册操作由测试人员执行，会暂时修改集群的 Admission 配置。先保留本页 D 节清理命令。

## B. 注册仅匹配测试标签的七条 Webhook

在 **Ubuntu 4090 主节点** 的普通 Bash 终端执行下面两行。复制时只选中命令本身，不要复制 Markdown 的 ` ```bash ` 和 ` ``` `。脚本先检查当前 context、证书、双副本、Endpoint 和五个 CRD；检查不通过时不会注册 Webhook。

```bash
cd /home/ubuntu/kubeedge-week4-test
bash hack/week5-register-business-webhooks.sh
```

预期最后输出 `Registered 5 validating and 2 mutating webhooks`，随后列出 5 条 Validating 和 2 条 Mutating 名称。截图应保留命令、这行结果和提示符。如果出现错误，先将完整输出发给维护者，不要继续 C 节。

## C. E01–E07 业务检查

2026-09-23 实际集群执行结果：**E01–E07 全部通过**。期间补齐 Operations v1alpha2 CRD 与 RBAC，并修复 OfflineMigration 对 API Server 临时 toleration 的误判；以下命令保留用于复现和验收归档。

### E01、E02：Device 合法创建和非法创建

```bash
kubectl -n week5-business-test create -f - <<'YAML'
apiVersion: devices.kubeedge.io/v1beta1
kind: Device
metadata:
  name: week5-e01-valid
  labels: {week5.kubeedge.io/business-test: "true"}
spec:
  properties: []
YAML

kubectl -n week5-business-test create --dry-run=server -f - <<'YAML'
apiVersion: devices.kubeedge.io/v1beta1
kind: Device
metadata:
  name: week5-e02-invalid
  labels: {week5.kubeedge.io/business-test: "true"}
spec:
  properties:
    - {name: duplicate}
    - {name: duplicate}
YAML
echo "E02 exit=$? (期望非 0，错误包含 property names must be unique)"
```

### E03：DeviceModel 非法更新

```bash
kubectl -n week5-business-test create -f - <<'YAML'
apiVersion: devices.kubeedge.io/v1beta1
kind: DeviceModel
metadata:
  name: week5-e03-model
  labels: {week5.kubeedge.io/business-test: "true"}
spec:
  properties: []
YAML
kubectl -n week5-business-test patch devicemodel week5-e03-model --type=merge -p '{"spec":{"properties":[{"name":"duplicate"},{"name":"duplicate"}]}}'
echo "E03 exit=$? (期望非 0，错误包含 property names must be unique)"
```

### E04：Rule/RuleEndpoint 校验与删除行为

```bash
kubectl -n week5-business-test create -f - <<'YAML'
apiVersion: rules.kubeedge.io/v1
kind: RuleEndpoint
metadata:
  name: week5-e04-source
  labels: {week5.kubeedge.io/business-test: "true"}
spec:
  ruleEndpointType: rest
---
apiVersion: rules.kubeedge.io/v1
kind: RuleEndpoint
metadata:
  name: week5-e04-target
  labels: {week5.kubeedge.io/business-test: "true"}
spec:
  ruleEndpointType: eventbus
YAML
sleep 5  # 等待 Controller Manager 的 RuleEndpoint informer 缓存同步
kubectl -n week5-business-test create -f - <<'YAML'
apiVersion: rules.kubeedge.io/v1
kind: Rule
metadata:
  name: week5-e04-rule
  labels: {week5.kubeedge.io/business-test: "true"}
spec:
  source: week5-e04-source
  sourceResource: {path: /week5-e04}
  target: week5-e04-target
  targetResource: {topic: week5-e04}
YAML
kubectl -n week5-business-test delete ruleendpoint week5-e04-source
echo "E04 endpoint delete exit=$?"
kubectl -n week5-business-test delete rule week5-e04-rule
echo "E04 rule delete exit=$?"
```

记录两个删除结果。要判定“与旧行为一致”，还需在正式 Helm 回滚后停用本页的预览配置，让相同请求由旧 Admission 的 WebhookConfiguration 处理，再使用新的测试资源名重复测试；不能把仍指向预览 Service 的结果算作旧组件证据。

### E05：非法 NodeUpgradeJob（仅 dry run）

```bash
kubectl create --dry-run=server -f - <<'YAML'
apiVersion: operations.kubeedge.io/v1alpha1
kind: NodeUpgradeJob
metadata:
  name: week5-e05-invalid
  labels: {week5.kubeedge.io/business-test: "true"}
spec:
  version: 1.0.0
  nodeNames: [week5-nonexistent]
YAML
echo "E05 exit=$? (期望非 0，错误包含 invalid version)"
```

### E06：OfflineMigration Pod Patch（仅 dry run）

```bash
kubectl -n week5-business-test create --dry-run=server -f - -o json <<'YAML' | jq -e '.spec.tolerations | any(.key == "node.kubernetes.io/unreachable" and .operator == "Exists" and ((.effect // "") == "") and .tolerationSeconds == null)'
apiVersion: v1
kind: Pod
metadata:
  name: week5-e06-pod
  labels:
    week5.kubeedge.io/business-test: "true"
    app-offline.kubeedge.io: autonomy
spec:
  containers:
    - {name: test, image: busybox}
YAML
echo "E06 exit=$? (期望 0 且输出 true)"
```

### E07：NodeUpgradeJob 默认值和幂等（仅 dry run）

```bash
job_json=$(kubectl create --dry-run=server -f - -o json <<'YAML'
apiVersion: operations.kubeedge.io/v1alpha1
kind: NodeUpgradeJob
metadata:
  name: week5-e07-defaults
  labels: {week5.kubeedge.io/business-test: "true"}
spec:
  version: v1.0.0
  nodeNames: [week5-nonexistent]
YAML
)
printf '%s\n' "$job_json" | jq -e '.spec.concurrency == 1 and .spec.timeoutSeconds == 300'
job_json_second=$(printf '%s\n' "$job_json" | kubectl create --dry-run=server -f - -o json)
printf '%s\n' "$job_json_second" | jq -e '.spec.concurrency == 1 and .spec.timeoutSeconds == 300'
diff <(printf '%s\n' "$job_json" | jq -S '.spec') <(printf '%s\n' "$job_json_second" | jq -S '.spec')
echo "E07 exit=$? (期望 0、两次输出 true，且 spec 无差异)"
```

如果收到 `no matches for kind`，说明目标 CRD 未安装；如果收到 `x509`、`service not found` 或 `no endpoints`，应先修复预览 Service/TLS。E01–E07 的通过证据必须是上述 API Server 响应，不能用直接 `curl` 的路由检查替代。

## D. 清理

先删除临时 WebhookConfiguration，避免在删除测试对象时被故障 Webhook 拦住；再删除测试资源。

```bash
kubectl delete validatingwebhookconfiguration week5-business-validation --ignore-not-found
kubectl delete mutatingwebhookconfiguration week5-business-mutation --ignore-not-found
kubectl delete nodeupgradejob week5-e07-defaults week5-e05-invalid --ignore-not-found
kubectl delete namespace week5-business-test --ignore-not-found
```

E01–E07 通过后，临时业务 WebhookConfiguration 和 namespace 已清理；正式环境由 Helm 管理。

## E. E12–E14 正式 Helm 验收前置检查

截至 2026-09-23，以下前置工作已经完成：

- `/home/ubuntu/week5-e12-e14-backup` 已保存 9 个 Helm 和在线资源备份文件，`backup_exit=0`；
- 在线 CloudCore 镜像确认为 `docker.io/kubeedge/cloudcore:v1.19.2-runtimeclass-20260812`，与历史 Helm values 中的镜像不同；
- 使用在线镜像覆盖参数执行 Helm dry-run，`dry_run_exit=0`；
- Controller Manager OCI 包已生成，路径为 `/home/ubuntu/week5-transfer/controller-manager-week5-oci.tar`，镜像名为 `docker.io/kubeedge/controller-manager:week5-go1.25.13`；
- 旧 Admission OCI 包已生成，路径为 `/home/ubuntu/week5-transfer/admission-week5-oci.tar`，镜像名为 `docker.io/kubeedge/admission:week5-go1.25.13`；
- E12 upgrade、E13 rollback/恢复和 E14 uninstall/install 均已执行并通过。

最终状态：`cloudcore` release revision 2 为 `deployed`；CloudCore 1/1，Controller Manager 2/2，正式 Service 有两个 `:9443` Endpoint；三项 WebhookConfiguration 均归属 `cloudcore`。预览 Service 与证书已经清理。

以下命令用于复核最终 Helm release、实际 Deployment、镜像和 WebhookConfiguration 归属。

```bash
kubectl config current-context
helm -n kubeedge status cloudcore
helm -n kubeedge history cloudcore
kubectl -n kubeedge get deployment cloudcore kubeedge-controller-manager -o wide
kubectl -n kubeedge get service kubeedge-admission-service kubeedge-controller-manager-webhook-preview -o wide
kubectl get validatingwebhookconfiguration kubeedge-crds-validate-webhook-configuration -o yaml | grep -E 'name:|meta.helm.sh/release-name|path:|namespace:'
kubectl get mutatingwebhookconfiguration mutate-offlinemigration kubeedge-mutating-webhook -o yaml | grep -E 'name:|meta.helm.sh/release-name|path:|namespace:'
```

本次执行中已对旧 Admission 和 Controller Manager 镜像、Service DNS 证书、CA、历史清单及资源所有权完成核对。预览阶段创建且内容与 Chart 完全一致的三个 Controller Manager RBAC 对象在备份后添加 Helm 所有权；未使用全局 `--take-ownership`。

`hack/test-controller-manager-webhook-e2e.sh` 可用于后续回归七条路由、重启、多副本、错误 CA、无 Endpoint和双向切换。E01–E07 的正式证据仍以 Kubernetes API Server 业务用例为准。
