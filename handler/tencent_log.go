package handler

import (
	"encoding/json"
	"fmt"
	cdn "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cdn/v20180606"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

var (
	metrics       = []string{"flux", "hitFlux", "request", "hitRequest", "bandwidth", "requestHitRate", "statusCode", "fluxHitRate"}
	originMetrics = []string{"flux", "request", "bandwidth", "failRequest", "statusCode", "failRate"}
	dimensions    = []string{"overseas", "mainland"}
)

type reqForTencentLog struct {
	SecretID  string `json:"secretId"`
	SecretKey string `json:"secretKey"`
	Domains   string `json:"domains"`
	StartTime int64  `json:"startTime"`
	EndTime   int64  `json:"endTime"`
	EndPoint  string `json:"endPoint"`
}

type CDNDataResult struct {
	Domain     string
	DataType   string
	Location   string
	Metric     string
	Timestamps []string
	Values     []int64
}

func HandleTencentOnTimeLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "仅支持 POST 方法", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	defer r.Body.Close()
	var reader io.Reader = r.Body
	body, err := io.ReadAll(reader)
	if err != nil {
		log.Printf("读取请求体时出错: %v\n", err)
		http.Error(w, "读取请求体时出错", http.StatusBadRequest)
		return
	}
	req := reqForTencentLog{}
	err = json.Unmarshal(body, &req)
	if err != nil {
		log.Printf("无效的 JSON 数据: %s\n", err.Error())
		http.Error(w, "无效的 JSON 数据", http.StatusBadRequest)
		return
	}

	m := GetMetric(req)
	marshal, _ := json.Marshal(m)
	w.Write(marshal)
	w.WriteHeader(http.StatusOK)
}

func HandleTencentZipLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "仅支持 POST 方法", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	defer r.Body.Close()
	var reader io.Reader = r.Body
	body, err := io.ReadAll(reader)
	if err != nil {
		log.Printf("读取请求体时出错: %v\n", err)
		http.Error(w, "读取请求体时出错", http.StatusBadRequest)
		return
	}
	req := reqForTencentLog{}
	err = json.Unmarshal(body, &req)
	if err != nil {
		log.Printf("无效的 JSON 数据: %s\n", err.Error())
		http.Error(w, "无效的 JSON 数据", http.StatusBadRequest)
	}

	m := GetLog(req)
	marshal, _ := json.Marshal(m)
	w.Write(marshal)
	w.WriteHeader(http.StatusOK)
}

func GetMetric(req reqForTencentLog) interface{} {
	secretId := req.SecretID
	secretKey := req.SecretKey
	domains := SplitDomains(req.Domains)

	credential := common.NewCredential(secretId, secretKey)
	cpf := profile.NewClientProfile()
	//cpf.HttpProfile.Endpoint = "cdn.tencentcloudapi.com"
	cpf.HttpProfile.Endpoint = req.EndPoint
	//client, _ := cdn.NewClient(credential, "", cpf)
	client, _ := cdn.NewClient(credential, "ap-shanghai", cpf)

	results := make(chan CDNDataResult, len(domains)*len(dimensions)*len(metrics))
	originResults := make(chan CDNDataResult, len(domains)*len(dimensions)*len(originMetrics))
	defer close(results)
	defer close(originResults)

	for _, domain := range domains {
		for _, metric := range metrics {
			for _, dim := range dimensions {
				go queryDimensionData(req, client, metric, domain, dim, results)
				time.Sleep(50 * time.Millisecond)
			}
		}
		for _, metric := range originMetrics {
			for _, dim := range dimensions {
				go queryDimensionOriginData(req, client, metric, domain, dim, originResults)
				time.Sleep(50 * time.Millisecond)
			}
		}
	}

	var dataResults, dataOriginResults []CDNDataResult
	for i := 0; i < len(domains)*len(dimensions)*len(metrics); i++ {
		if res := <-results; res.Metric != "" {
			dataResults = append(dataResults, res)
		}
	}
	for i := 0; i < len(domains)*len(dimensions)*len(originMetrics); i++ {
		if res := <-originResults; res.Metric != "" {
			dataOriginResults = append(dataOriginResults, res)
		}
	}

	//printReport(dataResults)
	//printReport(dataOriginResults)

	var (
		res = make(map[string]interface{})
		tmp = make(map[string]interface{})
	)
	res["code"] = 200
	res["status"] = "success"
	tmp["reqData"] = dataResults
	tmp["backToResData"] = dataOriginResults
	res["data"] = tmp
	return res
}

func queryDimensionOriginData(param reqForTencentLog, client *cdn.Client, metric, domain, dataType string, ch chan<- CDNDataResult) {
	req := cdn.NewDescribeOriginDataRequest()
	req.StartTime = common.StringPtr(GetTimeStr(param.StartTime))
	req.EndTime = common.StringPtr(GetTimeStr(param.EndTime))
	req.Metric = common.StringPtr(metric)
	req.Domains = []*string{&domain}
	req.Interval = common.StringPtr("min")

	if dataType == "overseas" {
		req.Area = common.StringPtr("overseas")
		// 查询中国境外CDN数据时，可指定地区类型查询，不填充表示查询服务地区数据（仅在 Area 为 overseas 时可用）
		// server：指定查询服务地区（腾讯云 CDN 节点服务器所在地区）数据
		// client：指定查询客户端地区（用户请求终端所在地区）数据
		//req.AreaType = common.StringPtr("server")
		// https://cloud.tencent.com/document/product/228/6316#.E5.8C.BA.E5.9F.9F-.2F-.E8.BF.90.E8.90.A5.E5.95.86.E6.98.A0.E5.B0.84.E8.A1.A8
		//req.District = common.StringPtr(district)
	} else {
		req.Area = common.StringPtr("mainland")
	}

	resp, err := client.DescribeOriginData(req)
	//fmt.Println(*resp.Response.RequestId)
	if err != nil {
		if sdkErr, ok := err.(*errors.TencentCloudSDKError); ok {
			fmt.Printf("queryDimensionOriginData API Error[%s]: %s\n", sdkErr.GetCode(), sdkErr.GetMessage())
		}
		return
	}

	// 解析数据
	for _, item := range resp.Response.Data {
		result := CDNDataResult{
			Domain:   domain,
			DataType: dataType,
		}

		for _, metricData := range item.OriginData {
			result.Metric = *metricData.Metric
			for _, detail := range metricData.DetailData {
				result.Timestamps = append(result.Timestamps, *detail.Time)
				result.Values = append(result.Values, int64(*detail.Value))
			}
		}
		ch <- result
	}
}

func queryDimensionData(param reqForTencentLog, client *cdn.Client, metric, domain, dataType string, ch chan<- CDNDataResult) {
	req := cdn.NewDescribeCdnDataRequest()
	req.StartTime = common.StringPtr(GetTimeStr(param.StartTime))
	req.EndTime = common.StringPtr(GetTimeStr(param.EndTime))
	req.Metric = common.StringPtr(metric)
	req.Domains = []*string{&domain}
	req.Interval = common.StringPtr("min")

	// 设置维度参数
	if dataType == "overseas" {
		req.Area = common.StringPtr("overseas")
		// 查询中国境外CDN数据时，可指定地区类型查询，不填充表示查询服务地区数据（仅在 Area 为 overseas 时可用）
		// server：指定查询服务地区（腾讯云 CDN 节点服务器所在地区）数据
		// client：指定查询客户端地区（用户请求终端所在地区）数据
		//req.AreaType = common.StringPtr("server")
		// https://cloud.tencent.com/document/product/228/6316#.E5.8C.BA.E5.9F.9F-.2F-.E8.BF.90.E8.90.A5.E5.95.86.E6.98.A0.E5.B0.84.E8.A1.A8
		//req.District = common.StringPtr(district)
	} else {
		req.Area = common.StringPtr("mainland")
	}

	resp, err := client.DescribeCdnData(req)
	if err != nil {
		if sdkErr, ok := err.(*errors.TencentCloudSDKError); ok {
			fmt.Printf("queryDimensionData API Error[%s]: %s\n", sdkErr.GetCode(), sdkErr.GetMessage())
		}
		return
	}

	for _, item := range resp.Response.Data {
		result := CDNDataResult{
			Domain:   domain,
			DataType: dataType,
		}

		for _, metricData := range item.CdnData {
			result.Metric = *metricData.Metric
			for _, detail := range metricData.DetailData {
				result.Timestamps = append(result.Timestamps, *detail.Time)
				result.Values = append(result.Values, int64(*detail.Value))
			}
		}
		ch <- result
	}
}

func SplitDomains(input string) []string {
	if input == "" {
		return []string{}
	}
	return strings.Split(input, ",")
}

func GetTimeStr(t int64) string {
	return time.Unix(t, 0).Format("2006-01-02 15:04:05")
}

func GetLog(req reqForTencentLog) interface{} {
	secretId := req.SecretID
	secretKey := req.SecretKey
	domains := SplitDomains(req.Domains)
	startTime := GetTimeStr(req.StartTime)
	endTime := GetTimeStr(req.EndTime)

	credential := common.NewCredential(secretId, secretKey)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = req.EndPoint
	client, _ := cdn.NewClient(credential, "", cpf)

	var (
		res = make(map[string]interface{})
		tmp = make(map[string][]interface{})
	)
	failRet := func() interface{} {
		res["code"] = 500
		res["status"] = "fail"
		res["data"] = "查询日志失败"
		return res
	}

	for _, domain := range domains {
		tmpMainLand := make(map[string]interface{})
		tmpOverSea := make(map[string]interface{})
		for _, dim := range dimensions {
			logs, err := queryDomainLogs(client, domain, startTime, endTime, dim)
			if err != nil {
				fmt.Printf("查询日志失败: %v\n", err)
				return failRet()
			}
			fmt.Printf("\n=== 日志下载链接 ===\n")
			for _, log := range logs {
				fmt.Printf("日志路径: %s\n区域: %s\n下载地址: %s\n\n",
					*log.LogPath, *log.Area, *log.LogPath)
			}
			if dim == "mainland" {
				tmpMainLand["mainland"] = logs
				continue
			}
			tmpOverSea["oversea"] = logs
		}
		tmp[domain] = append(tmp[domain], tmpMainLand, tmpOverSea)
	}

	res["code"] = 200
	res["status"] = "success"
	res["data"] = tmp
	return res
}

func queryDomainLogs(client *cdn.Client, domain, startTime, endTime, dim string) ([]*cdn.DomainLog, error) {
	req := cdn.NewDescribeCdnDomainLogsRequest()
	req.Domain = &domain
	req.StartTime = &startTime
	req.EndTime = &endTime
	req.Limit = common.Int64Ptr(1000)
	req.Area = &dim // mainland-境内 overseas-境外

	resp, err := client.DescribeCdnDomainLogs(req)
	//fmt.Println("resp", *resp.Response.RequestId)
	if err != nil {
		if sdkErr, ok := err.(*errors.TencentCloudSDKError); ok {
			return nil, fmt.Errorf("API Error[%s] %s", sdkErr.GetCode(), sdkErr.GetMessage())
		}
		return nil, err
	}

	return resp.Response.DomainLogs, nil
}

func printReport(results []CDNDataResult) {
	fmt.Println("\n=== CDN 多维数据报告 ===")
	for _, res := range results {
		fmt.Printf("\n域名: %s\n类型: %s\n地区: %s\n指标: %s\n",
			res.Domain, res.DataType, res.Location, res.Metric)

		for i := range res.Timestamps {
			fmt.Printf("时间: %s => 值: %d\n",
				res.Timestamps[i], res.Values[i])
		}
	}
}
