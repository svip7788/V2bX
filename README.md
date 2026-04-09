# V2bX

一个基于多种内核的 V2board/Xboard 节点服务端，支持 VMess/VLess、Trojan、Shadowsocks、Hysteria1/2、Tuic、AnyTLS 协议。

**搭配 [Xboard](https://github.com/cedar2025/Xboard) 面板使用**

## 特点

* 永久开源且免费
* 支持 VMess/VLess、Trojan、Shadowsocks、Hysteria1/2、Tuic、AnyTLS 多种协议
* 支持 VLess + XTLS/Reality 等新特性
* 支持单实例对接多节点，无需重复启动
* 支持 WebSocket 实时推送（配合 Xboard 面板）
* 支持限制在线 IP、TCP 连接数
* 支持节点端口级别、用户级别限速
* 支持面板驱动证书配置
* 配置简单，修改配置自动重启
* 支持多内核（Xray / Sing-box），易扩展
* 支持条件编译，可仅编译需要的内核

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

## 安装

从 [Releases](https://github.com/svip7788/V2bX/releases) 下载对应平台的 zip 包，解压后将 `V2bX` 放到 `/usr/local/bin/` 即可：

```bash
# 自动检测架构并安装最新版
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH_NAME="64" ;;
  aarch64|arm64) ARCH_NAME="arm64-v8a" ;;
  armv7*) ARCH_NAME="arm32-v7a" ;;
esac
VERSION=$(curl -sL https://api.github.com/repos/svip7788/V2bX/releases/latest | grep '"tag_name"' | head -1 | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
wget -O /tmp/v2bx.zip "https://github.com/svip7788/V2bX/releases/download/${VERSION}/V2bX-linux-${ARCH_NAME}.zip"
unzip -o /tmp/v2bx.zip -d /tmp/v2bx && cp /tmp/v2bx/V2bX /usr/local/bin/v2bx && chmod +x /usr/local/bin/v2bx
rm -rf /tmp/v2bx /tmp/v2bx.zip
```

## 构建

```bash
GOEXPERIMENT=jsonv2 go build -v -o V2bX -tags "sing xray with_quic with_grpc with_utls with_wireguard with_acme with_gvisor" -trimpath -ldflags "-X 'github.com/InazumaV/V2bX/cmd.version=$version' -s -w -buildid="
```

## 免责声明

* 本项目仅供学习交流使用，使用本项目造成的任何后果由使用者自行承担。

## Thanks

* [Project X](https://github.com/XTLS/)
* [V2Fly](https://github.com/v2fly)
* [XrayR](https://github.com/XrayR/XrayR)
* [sing-box](https://github.com/SagerNet/sing-box)
* [Xboard](https://github.com/cedar2025/Xboard)
