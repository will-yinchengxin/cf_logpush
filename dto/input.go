package dto

type InputLogForDownLoad struct {
	ClientCountry            string            `json:"ClientCountry"`
	ClientRequestMethod      string            `json:"ClientRequestMethod"`
	ClientRequestProtocol    string            `json:"ClientRequestProtocol"`
	OriginResponseDurationMs int64             `json:"OriginResponseDurationMs"`
	ClientIP                 string            `json:"ClientIP"`
	OriginIP                 string            `json:"OriginIP"`
	ClientSrcPort            int               `json:"ClientSrcPort"`
	ClientRequestHost        string            `json:"ClientRequestHost"`
	EdgeStartTimestamp       string            `json:"EdgeStartTimestamp"`
	ClientRequestURI         string            `json:"ClientRequestURI"`
	EdgeResponseStatus       int               `json:"EdgeResponseStatus"`
	EdgeResponseBodyBytes    int               `json:"EdgeResponseBodyBytes"`
	EdgeResponseBytes        int               `json:"EdgeResponseBytes"`
	OriginResponseTime       int               `json:"OriginResponseTime"`
	ClientRequestReferer     string            `json:"ClientRequestReferer"`
	ClientRequestUserAgent   string            `json:"ClientRequestUserAgent"`
	XForwardedFor            string            `json:"XForwardedFor"`
	CacheCacheStatus         string            `json:"CacheCacheStatus"`
	RayID                    string            `json:"RayID"`
	ResponseHeaders          map[string]string `json:"ResponseHeaders"`
}

type InputLog struct {
	EdgeStartTimestamp   string            `json:"EdgeStartTimestamp"`
	ClientCountry        string            `json:"ClientCountry"`
	ClientRegionCode     string            `json:"ClientRegionCode"`
	ClientRequestHost    string            `json:"ClientRequestHost"`
	ClientRequestBytes   int               `json:"EdgeResponseBytes"`
	OriginResponseBytes  int               `json:"CacheResponseBytes"`
	CacheCacheStatus     string            `json:"CacheCacheStatus"`
	EdgeResponseStatus   int               `json:"EdgeResponseStatus"`
	OriginResponseStatus int               `json:"OriginResponseStatus"`
	ResponseHeaders      map[string]string `json:"ResponseHeaders"`
}

type OutputLog struct {
	StartTime     int64  `json:"start_time"`
	Country       string `json:"country"`
	Region        string `json:"region"`
	Domain        string `json:"domain"`
	BW            int    `json:"bw"`
	Flux          int    `json:"flux"`
	BSBW          int    `json:"bs_bw"`
	BSFlux        int    `json:"bs_flux"`
	ReqNum        int    `json:"req_num"`
	HitNum        int    `json:"hit_num"`
	BSNum         int    `json:"bs_num"`
	BSFailNum     int    `json:"bs_fail_num"`
	HitFlux       int    `json:"hit_flux"`
	HTTPCode2XX   int    `json:"http_code_2xx"`
	HTTPCode3XX   int    `json:"http_code_3xx"`
	HTTPCode4XX   int    `json:"http_code_4xx"`
	HTTPCode5XX   int    `json:"http_code_5xx"`
	BSHTTPCode2XX int    `json:"bs_http_code_2xx"`
	BSHTTPCode3XX int    `json:"bs_http_code_3xx"`
	BSHTTPCode4XX int    `json:"bs_http_code_4xx"`
	BSHTTPCode5XX int    `json:"bs_http_code_5xx"`
}
