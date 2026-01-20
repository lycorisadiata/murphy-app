/*
 * @Description: 简单搜索器实现（用于 Redis 不可用时的降级方案）
 * @Author: 安知鱼
 * @Date: 2025-10-05 00:00:00
 * @LastEditTime: 2025-10-05 00:00:00
 * @LastEditors: 安知鱼
 */

package search

import (
	"context"
	"log"
	"strings"
	"sync"

	"github.com/anzhiyu-c/anheyu-app/pkg/constant"
	"github.com/anzhiyu-c/anheyu-app/pkg/domain/model"
	"github.com/anzhiyu-c/anheyu-app/pkg/idgen"
	"github.com/anzhiyu-c/anheyu-app/pkg/service/setting"
)

// SimpleSearcher 简单的内存搜索器实现（降级方案）
type SimpleSearcher struct {
	articles   sync.Map // map[string]*model.Article
	settingSvc setting.SettingService
}

// NewSimpleSearcher 创建简单搜索器
func NewSimpleSearcher(settingSvc setting.SettingService) *SimpleSearcher {
	log.Println("🔄 使用简单搜索模式（Simple Search）- 基于内存的关键词匹配")
	return &SimpleSearcher{
		settingSvc: settingSvc,
	}
}

// Search 执行搜索（简单的关键词匹配）
func (s *SimpleSearcher) Search(ctx context.Context, query string, page int, size int) (*model.SearchResult, error) {
	if query == "" {
		return &model.SearchResult{
			Pagination: &model.SearchPagination{Total: 0, Page: page, Size: size, TotalPages: 0},
			Hits:       []*model.SearchHit{},
		}, nil
	}

	query = strings.ToLower(strings.TrimSpace(query))
	var allHits []*model.SearchHit

	// 遍历所有文章进行匹配
	s.articles.Range(func(key, value interface{}) bool {
		article := value.(*model.Article)

		// 简单的关键词匹配
		title := strings.ToLower(article.Title)
		content := strings.ToLower(article.ContentHTML)

		// 计算相关度分数
		score := 0.0
		if strings.Contains(title, query) {
			score += 10.0 // 标题匹配权重更高
		}
		if strings.Contains(content, query) {
			score += 1.0 // 内容匹配基础权重
		}

		// 如果匹配，添加到结果中
		if score > 0 {
			hit := s.articleToSearchHit(article)
			allHits = append(allHits, hit)
		}

		return true
	})

	// 按相关度排序（这里简化处理，可以根据需要实现更复杂的排序）
	// TODO: 实现排序逻辑

	// 分页
	total := int64(len(allHits))
	start := (page - 1) * size
	end := start + size

	if start >= len(allHits) {
		allHits = []*model.SearchHit{}
	} else {
		if end > len(allHits) {
			end = len(allHits)
		}
		allHits = allHits[start:end]
	}

	totalPages := (int(total) + size - 1) / size
	return &model.SearchResult{
		Pagination: &model.SearchPagination{
			Total:      total,
			Page:       page,
			Size:       size,
			TotalPages: totalPages,
		},
		Hits: allHits,
	}, nil
}

// IndexArticle 索引文章
func (s *SimpleSearcher) IndexArticle(ctx context.Context, article *model.Article) error {
	s.articles.Store(article.ID, article)
	return nil
}

// DeleteArticle 删除文章索引
func (s *SimpleSearcher) DeleteArticle(ctx context.Context, articleID string) error {
	s.articles.Delete(articleID)
	return nil
}

// articleToSearchHit 将文章转换为搜索结果
func (s *SimpleSearcher) articleToSearchHit(article *model.Article) *model.SearchHit {
	// 获取作者名称：优先使用文章的版权作者，其次使用站点所有者名称
	author := article.CopyrightAuthor
	if author == "" {
		author = s.settingSvc.Get(constant.KeyFrontDeskSiteOwnerName.String())
	}

	hit := &model.SearchHit{
		ID:          article.ID,
		Title:       article.Title,
		Author:      author,
		CoverURL:    article.CoverURL,
		Abbrlink:    article.Abbrlink,
		PublishDate: article.CreatedAt,
		ViewCount:   article.ViewCount,
		WordCount:   article.WordCount,
		ReadingTime: article.ReadingTime,
		IsDoc:       article.IsDoc,
	}

	// 转换文档系列ID
	if article.DocSeriesID != nil {
		docSeriesPublicID, err := idgen.GeneratePublicID(*article.DocSeriesID, idgen.EntityTypeDocSeries)
		if err == nil {
			hit.DocSeriesID = docSeriesPublicID
		}
	}

	// 提取分类
	if len(article.PostCategories) > 0 {
		hit.Category = article.PostCategories[0].Name
	}

	// 提取标签
	tags := make([]string, len(article.PostTags))
	for i, tag := range article.PostTags {
		tags[i] = tag.Name
	}
	hit.Tags = tags

	// 生成摘要（简化处理）
	content := reHTMLTags.ReplaceAllString(article.ContentHTML, " ")
	content = strings.TrimSpace(content)
	contentRunes := []rune(content)
	if len(contentRunes) > 150 {
		hit.Snippet = string(contentRunes[:150]) + "..."
	} else {
		hit.Snippet = string(contentRunes)
	}

	return hit
}

// HealthCheck 健康检查
func (s *SimpleSearcher) HealthCheck(ctx context.Context) error {
	return nil // 内存搜索器总是健康的
}
