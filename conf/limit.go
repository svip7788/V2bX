package conf

type LimitConfig struct {
	SpeedLimit              int                      `json:"SpeedLimit"`
	EnableDynamicSpeedLimit bool                     `json:"EnableDynamicSpeedLimit"`
	DynamicSpeedLimitConfig *DynamicSpeedLimitConfig `json:"DynamicSpeedLimitConfig"`
}

type DynamicSpeedLimitConfig struct {
	Periodic   int   `json:"Periodic"`
	Traffic    int64 `json:"Traffic"`
	SpeedLimit int   `json:"SpeedLimit"`
	ExpireTime int   `json:"ExpireTime"`
}
