/*
 * @Description: 访问统计仓储实现
 * @Author: 安知鱼
 * @Date: 2025-01-20 15:30:00
 * @LastEditTime: 2025-08-21 11:10:03
 * @LastEditors: 安知鱼
 */
package ent

import (
	"context"
	"time"

	"github.com/anzhiyu-c/anheyu-app/ent"
	"github.com/anzhiyu-c/anheyu-app/ent/visitorstat"
	"github.com/anzhiyu-c/anheyu-app/pkg/domain/model"
	"github.com/anzhiyu-c/anheyu-app/pkg/domain/repository"

	"entgo.io/ent/dialect/sql"
)

type entVisitorStatRepository struct {
	client *ent.Client
}

// NewVisitorStatRepository 创建访问统计仓储实例
func NewVisitorStatRepository(client *ent.Client) repository.VisitorStatRepository {
	return &entVisitorStatRepository{
		client: client,
	}
}

func (r *entVisitorStatRepository) GetLatestDate(ctx context.Context) (*time.Time, error) {
	stat, err := r.client.VisitorStat.
		Query().
		Order(ent.Desc(visitorstat.FieldDate)).
		First(ctx)
	if err != nil {
		return nil, err
	}
	return &stat.Date, nil
}

func (r *entVisitorStatRepository) GetByDate(ctx context.Context, date time.Time) (*ent.VisitorStat, error) {
	// 截取到日期，忽略时分秒，并转换为UTC时区
	dateOnly := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)

	return r.client.VisitorStat.Query().
		Where(visitorstat.DateEQ(dateOnly)).
		Only(ctx)
}

func (r *entVisitorStatRepository) CreateOrUpdate(ctx context.Context, stat *ent.VisitorStat) error {
	// 截取到日期，忽略时分秒，统一使用 UTC 时区存储以确保与查询一致
	dateOnly := time.Date(stat.Date.Year(), stat.Date.Month(), stat.Date.Day(), 0, 0, 0, 0, time.UTC)

	return r.client.VisitorStat.Create().
		SetDate(dateOnly).
		SetUniqueVisitors(stat.UniqueVisitors).
		SetTotalViews(stat.TotalViews).
		SetPageViews(stat.PageViews).
		SetBounceCount(stat.BounceCount).
		OnConflict(
			// 明确指定冲突列为 date 字段
			sql.ConflictColumns(visitorstat.FieldDate),
		).
		UpdateNewValues().
		Exec(ctx)
}

func (r *entVisitorStatRepository) GetByDateRange(ctx context.Context, startDate, endDate time.Time) ([]*ent.VisitorStat, error) {
	// 使用本地时区来匹配数据库中存储的时间
	startOnly := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, time.Local)
	endOnly := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 999999999, time.Local)

	return r.client.VisitorStat.Query().
		Where(
			visitorstat.DateGTE(startOnly),
			visitorstat.DateLTE(endOnly),
		).
		Order(ent.Desc(visitorstat.FieldDate)).
		All(ctx)
}

func (r *entVisitorStatRepository) GetRecentDays(ctx context.Context, days int) ([]*ent.VisitorStat, error) {
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -days)

	return r.GetByDateRange(ctx, startDate, endDate)
}

func (r *entVisitorStatRepository) GetBasicStatistics(ctx context.Context) (*model.VisitorStatistics, error) {
	// 使用本地时区来匹配数据库中存储的时间（数据库使用服务器本地时间）
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	yesterday := today.AddDate(0, 0, -1)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
	yearStart := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.Local)

	stats := &model.VisitorStatistics{}

	// 今日数据
	if todayData, err := r.GetByDate(ctx, today); err == nil {
		stats.TodayVisitors = todayData.UniqueVisitors
		stats.TodayViews = todayData.TotalViews
	}

	// 昨日数据
	if yesterdayData, err := r.GetByDate(ctx, yesterday); err == nil {
		stats.YesterdayVisitors = yesterdayData.UniqueVisitors
		stats.YesterdayViews = yesterdayData.TotalViews
	}

	// 本月数据
	monthData, err := r.GetByDateRange(ctx, monthStart, now)
	if err == nil {
		for _, data := range monthData {
			stats.MonthViews += data.TotalViews
		}
	}

	// 本年数据
	yearData, err := r.GetByDateRange(ctx, yearStart, now)
	if err == nil {
		for _, data := range yearData {
			stats.YearViews += data.TotalViews
		}
	}

	return stats, nil
}
