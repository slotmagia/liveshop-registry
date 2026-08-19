# LiveShop Registry 工程规则

Registry 不是业务模块，不建 `模块开发规范.md`。通用边界见仓库根 [`docs/开发规范.md`](../docs/开发规范.md)。

- 本仓库拥有模块不可变发布、激活、路由快照与活动能力快照。
- 后端根目录为 `backend/internal`；`controlplane/provisioning` 是唯一对外表面。
- 进程配置只能来自 `-config` 指定的一份完整 YAML。
- 不发布新的线协议包名；实现 `liveshop.platform.v1.PlatformRegistryService`。
- 禁止拥有 IAM、通知投递、业务编排或浏览器 UI。
- gRPC 证书卷 `liveshop-grpc-certs` 由本仓 `grpccerts` 生产，Platform / Identity 只读挂载。
