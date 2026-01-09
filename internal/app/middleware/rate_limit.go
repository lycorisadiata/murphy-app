/*
 * @Description: 频率限制中间件
 * @Author: 安知鱼
 * @Date: 2025-11-08 00:00:00
 * @LastEditTime: 2025-11-08 15:59:28
 * @LastEditors: 安知鱼
 */
package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/anzhiyu-c/anheyu-app/pkg/response"
	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// ipRateLimiter 用于存储每个IP地址的限流器
type ipRateLimiter struct {
	limiters map[string]*limiterInfo
	mu       sync.RWMutex
	// 每个IP每分钟允许的请求数
	requestsPerMinute int
	// 突发请求数（允许短时间内的突发流量）
	burst int
	// 清理过期限流器的时间间隔
	cleanupInterval time.Duration
}

// limiterInfo 存储限流器及其最后访问时间
type limiterInfo struct {
	limiter      *rate.Limiter
	lastAccessed time.Time
}

// newIPRateLimiter 创建一个新的IP限流器
func newIPRateLimiter(requestsPerMinute, burst int) *ipRateLimiter {
	limiter := &ipRateLimiter{
		limiters:          make(map[string]*limiterInfo),
		requestsPerMinute: requestsPerMinute,
		burst:             burst,
		cleanupInterval:   5 * time.Minute,
	}

	// 启动定期清理协程
	go limiter.cleanupStaleEntries()

	return limiter
}

// getLimiter 获取指定IP的限流器
func (i *ipRateLimiter) getLimiter(ip string) *rate.Limiter {
	i.mu.Lock()
	defer i.mu.Unlock()

	info, exists := i.limiters[ip]
	if !exists {
		// 创建新的限流器
		// rate.Every(time.Minute / time.Duration(i.requestsPerMinute)) 表示每分钟允许 i.requestsPerMinute 个请求
		limiter := rate.NewLimiter(rate.Every(time.Minute/time.Duration(i.requestsPerMinute)), i.burst)
		info = &limiterInfo{
			limiter:      limiter,
			lastAccessed: time.Now(),
		}
		i.limiters[ip] = info
	} else {
		// 更新最后访问时间
		info.lastAccessed = time.Now()
	}

	return info.limiter
}

// cleanupStaleEntries 定期清理超过一定时间未使用的限流器
func (i *ipRateLimiter) cleanupStaleEntries() {
	ticker := time.NewTicker(i.cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		i.mu.Lock()
		for ip, info := range i.limiters {
			// 如果超过10分钟未访问，则删除该限流器
			if time.Since(info.lastAccessed) > 10*time.Minute {
				delete(i.limiters, ip)
			}
		}
		i.mu.Unlock()
	}
}

// 全局的友链申请限流器实例
var linkApplyLimiter *ipRateLimiter

func init() {
	// 每个IP每分钟最多3次请求，突发允许6次
	// 这意味着用户可以连续提交6次，但之后需要等待1分钟才能再次提交
	linkApplyLimiter = newIPRateLimiter(3, 6)
}

// LinkApplyRateLimit 友链申请频率限制中间件
// 限制每个IP地址的友链申请频率
func LinkApplyRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取客户端IP地址
		ip := getClientIP(c)

		// 获取该IP的限流器
		limiter := linkApplyLimiter.getLimiter(ip)

		// 检查是否允许请求
		if !limiter.Allow() {
			response.Fail(c, http.StatusTooManyRequests, "提交过于频繁，请稍后再试")
			c.Abort()
			return
		}

		c.Next()
	}
}

// getClientIP 获取客户端真实IP地址
func getClientIP(c *gin.Context) string {
	// 优先从 X-Real-IP 获取
	clientIP := c.GetHeader("X-Real-IP")
	if clientIP != "" {
		return clientIP
	}

	// 其次从 X-Forwarded-For 获取（可能包含多个IP，取第一个）
	clientIP = c.GetHeader("X-Forwarded-For")
	if clientIP != "" {
		// X-Forwarded-For 可能包含多个IP，格式为：client, proxy1, proxy2
		// 取第一个IP
		if ip, _, err := net.SplitHostPort(clientIP); err == nil {
			return ip
		}
		// 如果没有端口，直接返回
		return clientIP
	}

	// 最后从 RemoteAddr 获取
	if ip, _, err := net.SplitHostPort(c.Request.RemoteAddr); err == nil {
		return ip
	}

	return c.Request.RemoteAddr
}

// CustomRateLimit 创建一个自定义的频率限制中间件
// requestsPerMinute: 每分钟允许的请求数
// burst: 突发请求数
func CustomRateLimit(requestsPerMinute, burst int) gin.HandlerFunc {
	limiter := newIPRateLimiter(requestsPerMinute, burst)

	return func(c *gin.Context) {
		ip := getClientIP(c)
		ipLimiter := limiter.getLimiter(ip)

		if !ipLimiter.Allow() {
			response.Fail(c, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
			c.Abort()
			return
		}

		c.Next()
	}
}
