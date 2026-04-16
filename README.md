# V2bX v1.0.4

一个基于多种内核的 V2board/Xboard 节点服务端，支持 VMess/VLess、Trojan、Shadowsocks、Hysteria1/2、Tuic、AnyTLS 协议。

**搭配 [Xboard](https://github.com/cedar2025/Xboard) 面板使用**

## 特点

* 永久开源且免费
* 支持 VMess/VLess、Trojan、Shadowsocks、Hysteria1/2、Tuic、AnyTLS 多种协议
* 支持 VLess + XTLS/Reality 等新特性
* 支持单实例对接多节点，无需重复启动
* 支持按 `Nodes` 配置对象自动拆分子进程运行
* 支持单个 `Nodes` 对象使用逗号 `NodeID`，一个对象对应一个子进程
* 支持 WebSocket 实时推送（配合 Xboard 面板，需在面板服务器执行 `php artisan ws-server start --d` 启动 WS 服务）
* 支持限制在线 IP / 设备数
* 支持节点端口级别、用户级别限速
* 支持动态限速（按时间段、速率阈值自动触发）
* 支持面板驱动证书配置
* 配置简单，修改配置自动重启
* 支持多内核（Xray / Sing-box），易扩展
* 支持条件编译，可仅编译需要的内核
* 高并发优化，万级用户无锁竞争

## 功能介绍

| 功能 | VMess/VLess | Trojan | Shadowsocks | Hysteria1/2 | Tuic | AnyTLS |
|------|:-----------:|:------:|:-----------:|:-----------:|:----:|:------:|
| 自动申请/续签 TLS 证书 | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| 在线人数统计 | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| 审计规则 | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| 在线 IP 数限制 | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| 设备数限制 | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| 跨节点 IP 数限制 | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| 按用户限速 | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| 动态限速 | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| WebSocket 实时推送 | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

## v1.0.4 更新日志

### 稳定性与并发一致性
* 修复用户列表长期命中 `304 Not Modified` 时，到期/流量耗尽用户禁用不及时的问题
* 流量上报改为成功后再提交计数，避免面板异常时本地流量被提前清空
* 补齐 sing 路径下的 UDP 用户级限速，并在删用户、删节点时主动关闭关联连接
* 重构设备数限制与在线设备上报状态，减少高并发下的锁竞争和并发判定偏差

### 配置与诊断
* 统一 `dns.json`、`route.json`、`custom_outbound.json`、`custom_inbound.json` 的路径解析与 watcher 监听
* 新增可选 `pprof` 监听参数和 `collect_pprof.sh`，方便线上排查 CPU / 内存问题
* 新增 `99-v2bx-performance.conf`，便于高并发场景快速应用推荐内核参数

### 内核与兼容性
* 升级 `wyx2685/xray-core` 到最新提交，并修复对应配置接口兼容问题
* sing 配置同时兼容 `EnableTFO` / `EnableSniff` 与旧字段写法，便于平滑升级

## v1.0.3 更新日志

### 节点运行模型
* 新增 `master-worker` 多进程运行模式，主进程会按 `Nodes` 配置对象自动拉起子进程
* 支持单个 `Nodes` 对象配置多个 `NodeID`，例如 `"NodeID": "1167,1168"`
* 同类节点可合并到一个配置对象内运行，同时保留按对象隔离的多进程能力
* 多节点场景下为 sing-box 子进程隔离独立工作目录与 `cache.db`，避免缓存锁冲突

### 兼容性改进
* 兼容 Xboard 直接下发顶层 `decryption` 字段，用于 Xray VLESS 协议加密配置
* 保留旧版 `encryption + encryption_settings` 解析逻辑，兼容旧面板返回结构

## v1.0.2 更新日志

### 内存优化
* 修复 MarkOnlineDeviceReported 在上报失败时不调用导致 userOnlineIP 持续累积内存泄漏
* ConnCounter 避免对已实现 ExtendedConn 的连接重复包装，减少每连接内存开销
* 新增可选 pprof HTTP 端点（配置 `PprofListen`），支持在线内存诊断

## v1.0.1 更新日志

### Bug 修复
* 修复 GetNodeInfo 空指针 panic 及 ETag 缓存污染
* 修复审计规则并发读写无锁导致的数据竞争
* 修复自签证书生成时文件句柄泄漏及 PEM 类型错误
* 修复固定节点名称时重载 limiter 用户不同步
* 修复用户流量耗尽后节点仍可使用（空用户列表解析为 nil）
* 修复 Task.Restart 在回调内调用导致死锁
* 修复 Interval=0 时 NewTicker panic
* 修复 WS 关闭与重连竞态产生孤儿连接
* 修复 WS 事件丢弃后用户状态不一致
* 修复 lego DecodePrivate 空 PEM panic 及 Save 文件句柄泄漏
* 修复远程 Include 配置加载逻辑错误
* 修复流量上报重复 UID 覆盖导致统计偏少
* 修复 applyUserDelta 重复推送导致用户列表膨胀
* 修复跨午夜时段动态限速不生效

### 性能优化
* limiter 核心数据结构改为 sync.Map，万级用户连接零锁竞争
* UserTag 字符串拼接优化，减少每连接堆分配
* 缓存 uidToUUID 映射，流量上报不再全量遍历用户列表
* nodeReportMinTrafficBytes 改为 sync.Map，消除并发数据竞争

## 安装

### 一键安装

```bash
wget -N https://raw.githubusercontent.com/svip7788/V2bX/dev_new/install/install.sh && bash install.sh
```

### 手动安装

从 [Releases](https://github.com/svip7788/V2bX/releases) 下载对应平台的 zip 包。

## 构建

```bash
GOEXPERIMENT=jsonv2 go build -v -o V2bX -tags "sing xray with_quic with_grpc with_utls with_wireguard with_acme with_gvisor" -trimpath -ldflags "-X 'github.com/InazumaV/V2bX/cmd.version=1.0.4' -s -w -buildid="
```

## 免责声明

* 本项目仅供学习交流使用，使用本项目造成的任何后果由使用者自行承担。

## Thanks

* [Project X](https://github.com/XTLS/)
* [V2Fly](https://github.com/v2fly)
* [XrayR](https://github.com/XrayR/XrayR)
* [sing-box](https://github.com/SagerNet/sing-box)
* [Xboard](https://github.com/cedar2025/Xboard)
