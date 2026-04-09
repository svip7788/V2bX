package conf

type LimitConfig struct {
	SpeedLimit              int                      `json:"SpeedLimit"`
	EnableDynamicSpeedLimit bool                     `json:"EnableDynamicSpeedLimit"`
	DynamicSpeedLimitConfig *DynamicSpeedLimitConfig `json:"DynamicSpeedLimitConfig"`
}

type DynamicSpeedLimitConfig struct {
	Periodic   int   `json:"Periodic"`   // check interval in seconds
	Traffic    int64 `json:"Traffic"`    // traffic threshold in bytes
	SpeedLimit int   `json:"SpeedLimit"` // speed limit in Mbps after trigger
	ExpireTime int   `json:"ExpireTime"` // limit duration in minutes
}
