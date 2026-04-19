package conf

import (
	"encoding/json"
	"strings"

	"github.com/sagernet/sing-box/option"
)

type SingConfig struct {
	LogConfig    SingLogConfig `json:"Log"`
	NtpConfig    SingNtpConfig `json:"NTP"`
	OriginalPath string        `json:"OriginalPath"`
}

type SingLogConfig struct {
	Disabled   bool   `json:"Disable"`
	Level      string `json:"Level"`
	Output     string `json:"Output"`
	Timestamp  bool   `json:"Timestamp"`
	AccessPath string `json:"AccessPath,omitempty"`
	ErrorPath  string `json:"ErrorPath,omitempty"`
}

func NewSingConfig() *SingConfig {
	return &SingConfig{
		LogConfig: SingLogConfig{
			Level:     "error",
			Timestamp: true,
		},
		NtpConfig: SingNtpConfig{
			Enable:     false,
			Server:     "time.apple.com",
			ServerPort: 0,
		},
	}
}

func (c SingLogConfig) Normalized() SingLogConfig {
	normalized := c
	normalized.Level = strings.ToLower(strings.TrimSpace(normalized.Level))
	normalized.Output = strings.TrimSpace(normalized.Output)
	normalized.AccessPath = strings.TrimSpace(normalized.AccessPath)
	normalized.ErrorPath = strings.TrimSpace(normalized.ErrorPath)

	if normalized.Level == "warning" {
		normalized.Level = "warn"
	}

	if normalized.Output == "none" {
		normalized.Output = ""
	}

	if normalized.Level == "" {
		normalized.Level = "error"
	}

	if normalized.Level == "none" {
		normalized.Disabled = true
		normalized.Level = "error"
	}

	if normalized.Disabled {
		normalized.Output = ""
		return normalized
	}

	if normalized.Output == "" {
		switch {
		case normalized.ErrorPath != "" && normalized.ErrorPath != "none":
			normalized.Output = normalized.ErrorPath
		case normalized.AccessPath != "" && normalized.AccessPath != "none":
			normalized.Output = normalized.AccessPath
		}
	}

	return normalized
}

type SingOptions struct {
	TCPFastOpen              bool                   `json:"EnableTFO"`
	SniffEnabled             bool                   `json:"EnableSniff"`
	SniffOverrideDestination bool                   `json:"SniffOverrideDestination"`
	EnableDNS                bool                   `json:"EnableDNS"`
	DomainStrategy           option.DomainStrategy  `json:"DomainStrategy"`
	FallBackConfigs          *FallBackConfigForSing `json:"FallBackConfigs"`
	Multiplex                *MultiplexConfig       `json:"MultiplexConfig"`
}

type SingNtpConfig struct {
	Enable     bool   `json:"Enable"`
	Server     string `json:"Server"`
	ServerPort uint16 `json:"ServerPort"`
}

type FallBackConfigForSing struct {
	// sing-box
	FallBack        FallBack            `json:"FallBack"`
	FallBackForALPN map[string]FallBack `json:"FallBackForALPN"`
}

type FallBack struct {
	Server     string `json:"Server"`
	ServerPort string `json:"ServerPort"`
}

type MultiplexConfig struct {
	Enabled bool          `json:"Enable"`
	Padding bool          `json:"Padding"`
	Brutal  BrutalOptions `json:"Brutal"`
}

type BrutalOptions struct {
	Enabled  bool `json:"Enable"`
	UpMbps   int  `json:"UpMbps"`
	DownMbps int  `json:"DownMbps"`
}

func NewSingOptions() *SingOptions {
	return &SingOptions{
		EnableDNS:                false,
		TCPFastOpen:              false,
		SniffEnabled:             true,
		SniffOverrideDestination: true,
		FallBackConfigs:          &FallBackConfigForSing{},
		Multiplex:                &MultiplexConfig{},
	}
}

func (o *SingOptions) UnmarshalJSON(data []byte) error {
	type alias SingOptions
	if err := json.Unmarshal(data, (*alias)(o)); err != nil {
		return err
	}

	raw := struct {
		EnableTFO    *bool `json:"EnableTFO"`
		TCPFastOpen  *bool `json:"TCPFastOpen"`
		EnableSniff  *bool `json:"EnableSniff"`
		SniffEnabled *bool `json:"SniffEnabled"`
	}{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if raw.EnableTFO == nil && raw.TCPFastOpen != nil {
		o.TCPFastOpen = *raw.TCPFastOpen
	}
	if raw.EnableSniff == nil && raw.SniffEnabled != nil {
		o.SniffEnabled = *raw.SniffEnabled
	}
	return nil
}
