package conf

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const MachineAssetDir = "/etc/V2bX/"

// MachineConfig 机器模式：整台机器以 machine_id + token 连接 Xboard，节点列表由面板下发
type MachineConfig struct {
	APIHost   string `json:"ApiHost"`
	APISendIP string `json:"ApiSendIP"`
	MachineID int    `json:"MachineID"`
	Token     string `json:"Token"`
	// auto | sing | xray
	Core    string `json:"Core"`
	Timeout int    `json:"Timeout"`
	// 覆盖每个节点的默认 Options，字段与 Nodes 单项相同
	NodeDefaults json.RawMessage `json:"NodeDefaults"`
}

func (m *MachineConfig) Enabled() bool {
	return m != nil && m.MachineID > 0
}

// auto 模式下优先 sing 的协议（QUIC 系）；AnyTLS 两个内核都支持，auto 下走 xray
var singPreferredTypes = map[string]struct{}{
	"hysteria":  {},
	"hysteria2": {},
	"tuic":      {},
}

// 必须带 TLS 证书的协议，未配置证书时用自签
var selfCertTypes = map[string]struct{}{
	"hysteria":  {},
	"hysteria2": {},
	"tuic":      {},
	"anytls":    {},
}

var machineNodeTypes = map[string]struct{}{
	"vmess":       {},
	"vless":       {},
	"trojan":      {},
	"shadowsocks": {},
	"hysteria":    {},
	"hysteria2":   {},
	"tuic":        {},
	"anytls":      {},
}

// NormalizeMachineNodeType 返回 V2bX 支持的节点类型，不支持时返回空串
func NormalizeMachineNodeType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	if t == "v2ray" {
		t = "vmess"
	}
	if _, ok := machineNodeTypes[t]; !ok {
		return ""
	}
	return t
}

func (m *MachineConfig) coreMode() string {
	switch strings.ToLower(strings.TrimSpace(m.Core)) {
	case "sing":
		return "sing"
	case "xray":
		return "xray"
	default:
		return "auto"
	}
}

// CoreTypeFor 按 Core 策略为节点类型选择内核，available 为实际可用的内核类型
func (m *MachineConfig) CoreTypeFor(nodeType string, available []string) string {
	preferred := "xray"
	_, singPreferred := singPreferredTypes[nodeType]
	switch m.coreMode() {
	case "sing":
		preferred = "sing"
	case "auto":
		if singPreferred {
			preferred = "sing"
		}
	}
	has := func(t string) bool {
		for _, a := range available {
			if strings.EqualFold(a, t) {
				return true
			}
		}
		return false
	}
	if has(preferred) || len(available) == 0 {
		return preferred
	}
	if preferred == "xray" {
		return "sing"
	}
	return "xray"
}

// CoreConfigs 已手写 Cores 时原样使用，否则按 Core 策略生成默认内核配置
func (m *MachineConfig) CoreConfigs(existing []CoreConfig) []CoreConfig {
	if len(existing) > 0 {
		return existing
	}
	cores := make([]CoreConfig, 0, 2)
	if m.coreMode() != "sing" {
		cores = append(cores, CoreConfig{Type: "xray", XrayConfig: defaultMachineXrayConfig()})
	}
	cores = append(cores, CoreConfig{Type: "sing", SingConfig: defaultMachineSingConfig()})
	return cores
}

func defaultMachineXrayConfig() *XrayConfig {
	x := NewXrayConfig()
	x.AssetPath = MachineAssetDir
	x.LogConfig.Level = "error"
	x.LogConfig.ErrorPath = ""
	// dns/route/outbound 由 resolveOptionalConfigPath 在文件存在时自动使用
	if p := machineAsset("custom_inbound.json"); p != "" {
		x.InboundConfigPath = p
	}
	return x
}

func defaultMachineSingConfig() *SingConfig {
	s := NewSingConfig()
	s.OriginalPath = machineAsset("sing_origin.json")
	return s
}

func machineAsset(name string) string {
	p := filepath.Join(MachineAssetDir, name)
	info, err := os.Stat(p)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return ""
	}
	return p
}

// NodeConfig 生成单个面板节点的运行配置
func (m *MachineConfig) NodeConfig(nodeID int, nodeType string, coreType string) (NodeConfig, error) {
	timeout := m.Timeout
	if timeout <= 0 {
		timeout = 30
	}
	nc := NodeConfig{
		ApiConfig: ApiConfig{
			APIHost:   strings.TrimRight(m.APIHost, "/"),
			APISendIP: m.APISendIP,
			Key:       m.Token,
			NodeType:  nodeType,
			Timeout:   timeout,
		},
	}
	nc.ApiConfig.SetNodeIDs([]int{nodeID})

	opts, err := m.nodeOptionsJSON(nodeID, nodeType, coreType)
	if err != nil {
		return NodeConfig{}, err
	}
	nc.Options = Options{
		ListenIP:   "0.0.0.0",
		SendIP:     "0.0.0.0",
		CertConfig: NewCertConfig(),
	}
	if err := json.Unmarshal(opts, &nc.Options); err != nil {
		return NodeConfig{}, fmt.Errorf("unmarshal node %d options error: %w", nodeID, err)
	}
	return nc, nil
}

func (m *MachineConfig) nodeOptionsJSON(nodeID int, nodeType, coreType string) ([]byte, error) {
	_, needCert := selfCertTypes[nodeType]
	listenIP := "0.0.0.0"
	if coreType == "sing" {
		listenIP = "::"
	}
	certMode := "none"
	if needCert {
		certMode = "self"
	}
	opts := map[string]any{
		"Core":                   coreType,
		"ListenIP":               listenIP,
		"SendIP":                 "0.0.0.0",
		"DeviceOnlineMinTraffic": 200,
		"CertConfig": map[string]any{
			"CertMode":   certMode,
			"CertDomain": "",
			"CertFile":   fmt.Sprintf("%scert/node%d.crt", MachineAssetDir, nodeID),
			"KeyFile":    fmt.Sprintf("%scert/node%d.key", MachineAssetDir, nodeID),
		},
	}
	if coreType == "sing" {
		opts["EnableSniff"] = true
		opts["SniffOverrideDestination"] = true
		if needCert {
			opts["EnableTFO"] = false
		}
	}
	if len(m.NodeDefaults) > 0 {
		var overrides map[string]any
		if err := json.Unmarshal(m.NodeDefaults, &overrides); err != nil {
			return nil, fmt.Errorf("unmarshal Machine.NodeDefaults error: %w", err)
		}
		for k, v := range overrides {
			if k == "CertConfig" {
				if base, ok := opts["CertConfig"].(map[string]any); ok {
					if over, ok := v.(map[string]any); ok {
						for ck, cv := range over {
							base[ck] = cv
						}
						continue
					}
				}
			}
			opts[k] = v
		}
		opts["Core"] = coreType
	}
	return json.Marshal(opts)
}
