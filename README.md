# V2bX v1.0.2

一个基于多种内核的 V2board/Xboard 节点服务端，支持 VMess/VLess、Trojan、Shadowsocks、Hysteria1/2、Tuic、AnyTLS 协议。

**搭配 [Xboard](https://github.com/cedar2025/Xboard) 面板使用**

## 特点

* 永久开源且免费
* 支持 VMess/VLess、Trojan、Shadowsocks、Hysteria1/2、Tuic、AnyTLS 多种协议
* 支持 VLess + XTLS/Reality 等新特性
* 支持单实例对接多节点，无需重复启动
* 支持 WebSocket 实时推送（配合 Xboard 面板，需在面板服务器执行 `php artisan ws-server start --d` 启动 WS 服务）
* 支持限制在线 IP、TCP 连接数
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
| 连接数限制 | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| 跨节点 IP 数限制 | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| 按用户限速 | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| 动态限速 | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| WebSocket 实时推送 | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

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
GOEXPERIMENT=jsonv2 go build -v -o V2bX -tags "sing xray with_quic with_grpc with_utls with_wireguard with_acme with_gvisor" -trimpath -ldflags "-X 'github.com/InazumaV/V2bX/cmd.version=1.0.2' -s -w -buildid="
```

## 免责声明

* 本项目仅供学习交流使用，使用本项目造成的任何后果由使用者自行承担。

## Thanks

* [Project X](https://github.com/XTLS/)
* [V2Fly](https://github.com/v2fly)
* [XrayR](https://github.com/XrayR/XrayR)
* [sing-box](https://github.com/SagerNet/sing-box)
* [Xboard](https://github.com/cedar2025/Xboard)
