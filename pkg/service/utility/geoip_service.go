/*
 * @Description: IP地理位置查询服务，仅支持远程API查询。
 * @Author: 安知鱼
 * @Date: 2025-07-25 16:15:59
 * @LastEditTime: 2025-08-27 21:34:38
 * @LastEditors: 安知鱼
 */
package utility

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/anzhiyu-c/anheyu-app/pkg/constant"
	"github.com/anzhiyu-c/anheyu-app/pkg/service/setting"
)

// GeoIPService 定义了 IP 地理位置查询服务的统一接口。
type GeoIPService interface {
	Lookup(ipString string) (location string, err error)
	Close()
}

// apiResponse 定义了远程 IP API 返回的 JSON 数据的结构。
// 适配 NSUUU ipip API（全球 IPv4/IPv6 信息查询）
type apiResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		IP        string `json:"ip"`
		Country   string `json:"country"`
		Province  string `json:"province"`
		City      string `json:"city"`
		ISP       string `json:"isp"`
		Latitude  string `json:"latitude"`
		Longitude string `json:"longitude"`
		Address   string `json:"address"`
	} `json:"data"`
	RequestID string `json:"request_id"`
}

// apiKeyErrorResponse 定义了远程 IP API 密钥错误时查询返回的 JSON 数据的结构。
type apiKeyErrorResponse struct {
	Code      int    `json:"code"`
	Msg       string `json:"msg"`
	Data      string `json:"data"`
	RequestID string `json:"request_id"`
}

// smartGeoIPService 是现在唯一的服务实现，仅通过远程API查询。
type smartGeoIPService struct {
	settingSvc setting.SettingService
	httpClient *http.Client
}

// NewGeoIPService 是构造函数，注入了配置服务。
// 它不再需要数据库路径参数。
func NewGeoIPService(settingSvc setting.SettingService) (GeoIPService, error) {
	return &smartGeoIPService{
		settingSvc: settingSvc,
		httpClient: &http.Client{
			Timeout: 5 * time.Second, // 为 API 请求设置5秒超时
		},
	}, nil
}

// Lookup 是核心的查询方法，只通过 API 进行。
func (s *smartGeoIPService) Lookup(ipStr string) (string, error) {
	log.Printf("[IP属地查询] 开始查询IP地址: %s", ipStr)

	apiURL := s.settingSvc.Get(constant.KeyIPAPI.String())
	apiToken := s.settingSvc.Get(constant.KeyIPAPIToKen.String())

	// 如果 API 和 Token 未配置，则直接返回错误
	if apiURL == "" || apiToken == "" {
		log.Printf("[IP属地查询] ❌ IP属地查询失败 - IP: %s, 原因: 远程API未配置 (apiURL: %s, apiToken配置: %t)",
			ipStr, apiURL, apiToken != "")
		return "未知", fmt.Errorf("IP 查询失败：远程 API 未配置")
	}

	log.Printf("[IP属地查询] API配置检查通过 - URL: %s, Token已配置: %t", apiURL, apiToken != "")

	location, err := s.lookupViaAPI(apiURL, apiToken, ipStr)
	if err != nil {
		// 记录错误，但返回统一的"未知"给上层调用者
		log.Printf("[IP属地查询] ❌ IP属地最终结果为'未知' - IP: %s, API调用失败: %v", ipStr, err)
		return "未知", err
	}

	log.Printf("[IP属地查询]IP属地查询成功 - IP: %s, 结果: %s", ipStr, location)
	return location, nil
}

// lookupViaAPI 封装了调用远程 API 的逻辑。
// 使用 NSUUU ipv1 API，支持 Bearer Token 认证方式
func (s *smartGeoIPService) lookupViaAPI(apiURL, apiToken, ipStr string) (string, error) {
	// 构建请求URL，只包含ip参数，key通过Header传递
	reqURL := fmt.Sprintf("%s?ip=%s", apiURL, ipStr)

	log.Printf("[IP属地查询] 准备调用第三方API - URL: %s", reqURL)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		log.Printf("[IP属地查询] ❌ 创建HTTP请求失败 - IP: %s, 目标: %s", ipStr, reqURL)
		return "", fmt.Errorf("创建 API 请求失败: %w", err)
	}

	// 使用 Bearer Token 方式传递 API Key（推荐方式，更安全）
	req.Header.Set("Authorization", "Bearer "+apiToken)

	log.Printf("[IP属地查询] 发送HTTP请求到第三方API（使用Bearer Token认证）...")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Printf("[IP属地查询] ❌ HTTP请求失败 - IP: %s, 目标: %s, 错误类型: %T", ipStr, reqURL, err)
		return "", fmt.Errorf("API 请求网络错误: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("[IP属地查询] 收到HTTP响应 - IP: %s, 状态码: %d", ipStr, resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		log.Printf("[IP属地查询] ❌ API返回非200状态码 - IP: %s, 状态: %s", ipStr, resp.Status)
		return "", fmt.Errorf("API 返回非 200 状态码: %s", resp.Status)
	}

	// 读取整个响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应体失败: %w", err)
	}

	// 1. 尝试解析为apiKeyErrorResponse
	var keyErrorResult apiKeyErrorResponse
	if err := json.Unmarshal(body, &keyErrorResult); err == nil {
		// 如果能解析成功，说明是API KEY错误
		log.Printf("[IP属地查询] ❌ API KEY错误 - IP: %s", ipStr)
		return "", fmt.Errorf("API KEY配置错误")
	}

	// 2. 上述错误结构无法解析，尝试解析为正常响应
	var result apiResponse
	if err := json.Unmarshal(body, &result); err != nil {
		// 如果这里也解析失败，才报告JSON解析失败
		log.Printf("[IP属地查询] ❌ 解析API响应JSON失败 - IP: %s, 错误: %v", ipStr, err)
		return "", fmt.Errorf("解析API响应JSON失败: %w", err)
	}

	log.Printf("[IP属地查询] API响应解析成功 - IP: %s, 业务码: %d, 国家: %s, 省份: %s, 城市: %s",
		ipStr, result.Code, result.Data.Country, result.Data.Province, result.Data.City)

	if result.Code != 200 {
		log.Printf("[IP属地查询] ❌ API返回业务错误 - IP: %s, 错误码: %d, 错误信息: %s", ipStr, result.Code, result.Message)
		return "", fmt.Errorf("API 返回业务错误码: %d, 信息: %s", result.Code, result.Message)
	}

	province := result.Data.Province
	city := result.Data.City

	// 根据优先级组装位置信息
	var finalLocation string
	if province != "" && city != "" && province != city {
		finalLocation = fmt.Sprintf("%s %s", province, city)
		log.Printf("[IP属地查询] 使用省+市格式 - IP: %s, 结果: %s", ipStr, finalLocation)
	} else if city != "" {
		finalLocation = city
		log.Printf("[IP属地查询] 使用城市格式 - IP: %s, 结果: %s", ipStr, finalLocation)
	} else if province != "" {
		finalLocation = province
		log.Printf("[IP属地查询] 使用省份格式 - IP: %s, 结果: %s", ipStr, finalLocation)
	} else if result.Data.Country != "" {
		finalLocation = result.Data.Country
		log.Printf("[IP属地查询] 使用国家格式 - IP: %s, 结果: %s", ipStr, finalLocation)
	} else {
		log.Printf("[IP属地查询] ❌ API响应中无有效位置信息 - IP: %s, API返回的数据: 国家=%s, 省份=%s, 城市=%s",
			ipStr, result.Data.Country, result.Data.Province, result.Data.City)
		return "", fmt.Errorf("API 响应中未包含位置信息")
	}

	return finalLocation, nil
}

// Close 在这个实现中不需要做任何事，但为了满足接口要求而保留。
func (s *smartGeoIPService) Close() {
	// httpClient 不需要显式关闭
}
