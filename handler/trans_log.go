package handler

import (
	"log_format/dto"
	"time"
)

const (
	tdAgentURL    = "http://localhost:9881/cftest.log"
	maxRetry      = 3
	retryInterval = 2 * time.Second
)

var (
	cfRegionField = "x-client-region"
)

func TransformLog(input dto.InputLog) dto.OutputLog {
	output := dto.OutputLog{
		ReqNum:    1,
		BSFailNum: 0,
	}

	t, err := time.Parse(time.RFC3339, input.EdgeStartTimestamp)
	if err == nil {
		output.StartTime = t.UnixMilli()
	}

	output.Region = input.ResponseHeaders[cfRegionField]
	//if input.ClientCountry != "" {
	//	if input.ClientCountry == "India" {
	//		output.Country = "in"
	//	} else if input.ClientCountry == "Singapore" {
	//		output.Country = "sg"
	//	} else {
	//		output.Country = input.ClientCountry
	//	}
	//}
	output.Country = input.ClientCountry
	output.Domain = input.ClientRequestHost
	output.BW = input.ClientRequestBytes * 8
	output.Flux = input.ClientRequestBytes

	// MISS  / EXPIRED / BYPASS  / DYNAMIC
	if input.OriginResponseStatus != 0 && input.OriginResponseStatus != 304 &&
		(input.CacheCacheStatus == "miss" || input.CacheCacheStatus == "expired" || input.CacheCacheStatus == "bypass" || input.CacheCacheStatus == "dynamic") {
		output.BSBW = input.OriginResponseBytes * 8
		output.BSFlux = input.OriginResponseBytes
	}

	if input.CacheCacheStatus == "hit" {
		output.HitNum = 1
		output.HitFlux = input.ClientRequestBytes
	}

	if input.CacheCacheStatus == "miss" || input.CacheCacheStatus == "expired" || input.CacheCacheStatus == "bypass" || input.CacheCacheStatus == "dynamic" {
		output.BSNum = 1
	}

	// 根据 EdgeResponseStatus 计算 HTTPCodeXXX
	if input.EdgeResponseStatus >= 200 && input.EdgeResponseStatus <= 299 {
		output.HTTPCode2XX = 1
	} else if input.EdgeResponseStatus >= 300 && input.EdgeResponseStatus <= 399 {
		output.HTTPCode3XX = 1
	} else if input.EdgeResponseStatus >= 400 && input.EdgeResponseStatus <= 499 {
		output.HTTPCode4XX = 1
	} else if input.EdgeResponseStatus >= 500 && input.EdgeResponseStatus <= 599 {
		output.HTTPCode5XX = 1
	}

	// 根据 OriginResponseStatus 计算 BSHTTPCodeXXX
	if input.OriginResponseStatus >= 200 && input.OriginResponseStatus <= 299 {
		output.BSHTTPCode2XX = 1
	} else if input.OriginResponseStatus >= 300 && input.OriginResponseStatus <= 399 {
		output.BSHTTPCode3XX = 1
	} else if input.OriginResponseStatus >= 400 && input.OriginResponseStatus <= 499 {
		output.BSHTTPCode4XX = 1
	} else if input.OriginResponseStatus >= 500 && input.OriginResponseStatus <= 599 {
		output.BSHTTPCode5XX = 1
	}

	return output
}
