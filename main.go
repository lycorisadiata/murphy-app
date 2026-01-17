/*
 * @Description:
 * @Author: 安知鱼
 * @Date: 2025-06-28 00:21:55
 * @LastEditTime: 2025-12-01 12:19:06
 * @LastEditors: 安知鱼
 */
package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/anzhiyu-c/anheyu-app/cmd/server"
)

//go:embed all:assets/dist
var content embed.FS

// @title           Anheyu App API
// @version         1.0
// @description     Anheyu App 应用接口文档
// @termsOfService  http://swagger.io/terms/

// @contact.name   安知鱼
// @contact.url    https://github.com/anzhiyu-c/anheyu-app
// @contact.email  support@anheyu.com

// @license.name  MIT
// @license.url   https://opensource.org/licenses/MIT

// @host      localhost:8080
// @BasePath  /api

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description 在请求头中添加 Bearer Token，格式为: Bearer {token}

// @externalDocs.description  OpenAPI
// @externalDocs.url          https://swagger.io/resources/open-api/
func main() {
	// 解析命令行参数
	var exportAssetsDir string
	flag.StringVar(&exportAssetsDir, "export-assets", "", "导出静态资源到指定目录（用于自定义静态资源）")
	flag.Parse()

	// 如果指定了导出静态资源的目录，则导出并退出
	if exportAssetsDir != "" {
		if err := exportAssets(exportAssetsDir); err != nil {
			log.Fatalf("导出静态资源失败: %v", err)
		}
		log.Printf("✅ 静态资源已成功导出到: %s", exportAssetsDir)
		log.Println("提示：您可以修改导出的静态资源，然后将其挂载到容器的 /app/static 目录或配置外部静态资源路径")
		return
	}

	// 调用位于 cmd/server 包中的 NewApp 函数来构建整个应用
	app, cleanup, err := server.NewApp(content)
	if err != nil {
		log.Fatalf("应用初始化失败: %v", err)
	}

	// 使用 defer 来确保 cleanup 函数在 main 退出时被调用
	defer cleanup()

	// 确保后台任务在程序退出时被停止
	defer app.Stop()

	if os.Getenv("ANHEYU_LICENSE_KEY") == "" {
		app.PrintBanner()
	}

	// 启动应用
	if err := app.Run(); err != nil {
		log.Fatalf("应用运行失败: %v", err)
	}
}

// exportAssets 将嵌入的静态资源导出到指定目录
func exportAssets(outputDir string) error {
	// 创建输出目录
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 获取嵌入的 assets/dist 子文件系统
	subFS, err := fs.Sub(content, "assets/dist")
	if err != nil {
		return fmt.Errorf("获取嵌入文件系统失败: %w", err)
	}

	// 统计导出的文件数量
	var fileCount int

	// 遍历并复制所有文件
	err = fs.WalkDir(subFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// 计算目标路径
		targetPath := filepath.Join(outputDir, path)

		if d.IsDir() {
			// 创建目录
			return os.MkdirAll(targetPath, 0755)
		}

		// 读取文件内容
		data, err := fs.ReadFile(subFS, path)
		if err != nil {
			return fmt.Errorf("读取文件 %s 失败: %w", path, err)
		}

		// 确保父目录存在
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return fmt.Errorf("创建父目录失败: %w", err)
		}

		// 写入文件
		if err := os.WriteFile(targetPath, data, 0644); err != nil {
			return fmt.Errorf("写入文件 %s 失败: %w", targetPath, err)
		}

		fileCount++
		return nil
	})

	if err != nil {
		return err
	}

	log.Printf("📦 共导出 %d 个文件", fileCount)
	return nil
}
