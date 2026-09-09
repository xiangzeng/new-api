package constant

var StreamingTimeout int

// StreamingTimeoutDataOnly restricts what resets the idle StreamingTimeout to
// real upstream data lines. Off by default: the historical behaviour lets any
// line, keep-alive included, postpone the deadline forever.
var StreamingTimeoutDataOnly bool

// ZeroResponseNoCharge skips billing a streaming relay that received no
// upstream data at all. The user got nothing, so locally estimated prompt
// tokens must not be charged.
var ZeroResponseNoCharge bool
var DifyDebug bool
var MaxFileDownloadMB int
var StreamScannerMaxBufferMB int
var ForceStreamOption bool
var CountToken bool
var GetMediaToken bool
var GetMediaTokenNotStream bool
var UpdateTask bool
var MaxRequestBodyMB int
var AnonymousRequestBodyLimitKB int
var AzureDefaultAPIVersion string
var NotifyLimitCount int
var NotificationLimitDurationMinute int
var GenerateDefaultToken bool
var ErrorLogEnabled bool
var TaskQueryLimit int
var TaskTimeoutMinutes int

// temporary variable for sora patch, will be removed in future
var TaskPricePatches []string

// TrustedRedirectDomains is a list of trusted domains for redirect URL validation.
// Domains support subdomain matching (e.g., "example.com" matches "sub.example.com").
var TrustedRedirectDomains []string
