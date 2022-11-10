package signalr

// A HubConf is a signalr config.
type HubConf struct {
	Timeout                   uint64 `json:",optional,default=3000"`
	AllowOriginPatterns       string `json:",optional,default=*"`
	KeepAliveInterval         uint64 `json:",optional,default=2000"`
	MaximumReceiveMessageSize uint64 `json:",default=32768"`
}
