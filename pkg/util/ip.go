// pkg/util/ip.go
package util

import (
	"net"
	"strings"

	"github.com/gin-gonic/gin"
)

// GetRealClientIP 获取客户端真实IP地址
// 优先级：X-Forwarded-For > X-Real-IP > X-Original-Forwarded-For > CF-Connecting-IP > EO-Connecting-IP > Ali-CDN-Real-IP > 其他 > RemoteAddr
// 支持的 CDN: Cloudflare, 腾讯云 EdgeOne, 阿里云 CDN/ESA 等
func GetRealClientIP(c *gin.Context) string {
	// 1. 检查 X-Forwarded-For 头部（最常用的代理头部）
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For 可能包含多个IP，格式：client, proxy1, proxy2
		// 取第一个IP（客户端真实IP）
		if ips := strings.Split(xff, ","); len(ips) > 0 {
			clientIP := strings.TrimSpace(ips[0])
			// 验证IP格式
			if ip := net.ParseIP(clientIP); ip != nil {
				return clientIP
			}
		}
	}

	// 2. 检查 X-Real-IP 头部（Nginx常用）
	if realIP := c.GetHeader("X-Real-IP"); realIP != "" {
		realIP = strings.TrimSpace(realIP)
		// 验证IP格式
		if ip := net.ParseIP(realIP); ip != nil {
			return realIP
		}
	}

	// 3. 检查 X-Original-Forwarded-For 头部（某些代理使用）
	if originalIP := c.GetHeader("X-Original-Forwarded-For"); originalIP != "" {
		originalIP = strings.TrimSpace(originalIP)
		// 验证IP格式
		if ip := net.ParseIP(originalIP); ip != nil {
			return originalIP
		}
	}

	// 4. 检查 CF-Connecting-IP 头部（Cloudflare使用）
	if cfIP := c.GetHeader("CF-Connecting-IP"); cfIP != "" {
		cfIP = strings.TrimSpace(cfIP)
		// 验证IP格式
		if ip := net.ParseIP(cfIP); ip != nil {
			return cfIP
		}
	}

	// 5. 检查 EO-Connecting-IP 头部（腾讯云 EdgeOne 使用）
	if eoIP := c.GetHeader("EO-Connecting-IP"); eoIP != "" {
		eoIP = strings.TrimSpace(eoIP)
		// 验证IP格式
		if ip := net.ParseIP(eoIP); ip != nil {
			return eoIP
		}
	}

	// 6. 检查 Ali-CDN-Real-IP 头部（阿里云 CDN/ESA 使用）
	if aliIP := c.GetHeader("Ali-CDN-Real-IP"); aliIP != "" {
		aliIP = strings.TrimSpace(aliIP)
		// 验证IP格式
		if ip := net.ParseIP(aliIP); ip != nil {
			return aliIP
		}
	}

	// 7. 检查所有可能的头部（包括非标准的）
	possibleHeaders := []string{
		"True-Client-IP",
		"X-Client-IP",
		"X-Cluster-Client-IP",
		"X-Forwarded",
		"Forwarded-For",
		"Forwarded",
	}

	for _, header := range possibleHeaders {
		if ip := c.GetHeader(header); ip != "" {
			ip = strings.TrimSpace(ip)
			// 处理可能的多IP情况
			if ips := strings.Split(ip, ","); len(ips) > 0 {
				firstIP := strings.TrimSpace(ips[0])
				if parsedIP := net.ParseIP(firstIP); parsedIP != nil {
					return firstIP
				}
			}
		}
	}

	// 8. 最后使用Gin内置的ClientIP方法（会检查RemoteAddr）
	return c.ClientIP()
}

// IsValidIP 验证IP地址是否有效
func IsValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

// IsPrivateIP 检查是否为私有IP地址
func IsPrivateIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	// IPv4私有地址范围
	privateIPRanges := []string{
		"10.0.0.0/8",     // 10.0.0.0 - 10.255.255.255
		"172.16.0.0/12",  // 172.16.0.0 - 172.31.255.255
		"192.168.0.0/16", // 192.168.0.0 - 192.168.255.255
		"127.0.0.0/8",    // 本地回环
	}

	for _, cidr := range privateIPRanges {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if ipNet.Contains(parsedIP) {
			return true
		}
	}

	return false
}
