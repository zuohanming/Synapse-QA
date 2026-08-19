package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"synapseqa/backend/internal/model"
)

type DashboardRepository interface {
	ListProjects(ctx context.Context, userID int64, admin bool, projectID *int64) ([]model.DashboardProject, error)
	ListUIExecutions(ctx context.Context, userID int64, admin bool, projectID *int64, from, to time.Time) (model.DashboardSourceData, error)
	ListAPIExecutions(ctx context.Context, userID int64, admin bool, projectID *int64, from, to time.Time) (model.DashboardSourceData, error)
	ListPerfExecutions(ctx context.Context, userID int64, admin bool, projectID *int64, from, to time.Time) (model.DashboardSourceData, error)
}

type DashboardService struct {
	repo DashboardRepository
}

func NewDashboardService(repo DashboardRepository) *DashboardService {
	return &DashboardService{repo: repo}
}

type DashboardRequest struct {
	ProjectID *int64
	Range     string
}

func (s *DashboardService) Overview(ctx context.Context, claims model.Claims, req DashboardRequest) (model.DashboardOverview, error) {
	if req.ProjectID != nil && *req.ProjectID <= 0 {
		return model.DashboardOverview{}, model.NewDomainError(model.ErrValidation, "项目 ID 无效")
	}
	if req.Range == "" {
		req.Range = "7d"
	}
	now := time.Now().UTC()
	from, to, err := dashboardRange(req.Range, now)
	if err != nil {
		return model.DashboardOverview{}, err
	}

	admin := claims.RoleCode == "admin"
	projects, err := s.repo.ListProjects(ctx, claims.UserID, admin, req.ProjectID)
	if err != nil {
		return model.DashboardOverview{}, err
	}
	if req.ProjectID != nil && !hasProject(projects, *req.ProjectID) {
		return model.DashboardOverview{}, model.NewDomainError(model.ErrNotFound, "项目不存在")
	}

	capabilities := dashboardCapabilities(claims)
	sourceStates := model.DashboardSourceStates{}
	recentRecords := make([]model.DashboardExecutionRecord, 0, 15)
	failedRecords := make([]model.DashboardExecutionRecord, 0, 9)
	counts := model.DashboardExecutionCounts{}
	partial := false

	if capabilities.UIRuns {
		data, sourceErr := s.repo.ListUIExecutions(ctx, claims.UserID, admin, req.ProjectID, from, to)
		if sourceErr != nil {
			sourceStates.UI = "error"
			partial = true
		} else {
			sourceStates.UI = "ok"
			counts = addDashboardCounts(counts, data.Counts)
			recentRecords = append(recentRecords, data.Recent...)
			failedRecords = append(failedRecords, data.Failed...)
		}
	} else {
		sourceStates.UI = "forbidden"
	}

	if capabilities.APIRuns {
		data, sourceErr := s.repo.ListAPIExecutions(ctx, claims.UserID, admin, req.ProjectID, from, to)
		if sourceErr != nil {
			sourceStates.API = "error"
			partial = true
		} else {
			sourceStates.API = "ok"
			counts = addDashboardCounts(counts, data.Counts)
			recentRecords = append(recentRecords, data.Recent...)
			failedRecords = append(failedRecords, data.Failed...)
		}
	} else {
		sourceStates.API = "forbidden"
	}

	if capabilities.PerfRuns {
		data, sourceErr := s.repo.ListPerfExecutions(ctx, claims.UserID, admin, req.ProjectID, from, to)
		if sourceErr != nil {
			sourceStates.Perf = "error"
			partial = true
		} else {
			sourceStates.Perf = "ok"
			counts = addDashboardCounts(counts, data.Counts)
			recentRecords = append(recentRecords, data.Recent...)
			failedRecords = append(failedRecords, data.Failed...)
		}
	} else {
		sourceStates.Perf = "forbidden"
	}

	sortDashboardRecords(recentRecords)
	if len(recentRecords) > 5 {
		recentRecords = recentRecords[:5]
	}
	recent := make([]model.DashboardRecentItem, 0, len(recentRecords))
	for _, record := range recentRecords {
		recent = append(recent, dashboardRecentItem(record))
	}

	sortDashboardRecords(failedRecords)
	attention := dashboardAttentionWithTotal(failedRecords, partial, counts.Failed)
	state := "ok"
	if partial {
		state = "partial"
	}
	return model.DashboardOverview{
		GeneratedAt:  to,
		Filter:       model.DashboardFilter{ProjectID: req.ProjectID, Range: req.Range, From: from, To: to},
		Projects:     projects,
		Capabilities: capabilities,
		Executions: model.DashboardExecutions{
			State: state, SourceStates: sourceStates, Counts: counts, Recent: recent,
		},
		Attention: attention,
	}, nil
}

func dashboardRange(value string, now time.Time) (time.Time, time.Time, error) {
	now = now.UTC()
	var from time.Time
	switch value {
	case "":
		from = now.AddDate(0, 0, -7)
	case "24h":
		from = now.Add(-24 * time.Hour)
	case "7d":
		from = now.AddDate(0, 0, -7)
	case "30d":
		from = now.AddDate(0, 0, -30)
	default:
		return time.Time{}, time.Time{}, model.NewDomainError(model.ErrValidation, "range 仅支持 24h、7d 或 30d")
	}
	return from, now, nil
}

func dashboardCapabilities(claims model.Claims) model.DashboardCapabilities {
	if claims.RoleCode == "admin" {
		return model.DashboardCapabilities{UIRuns: true, APIRuns: true, PerfRuns: true}
	}
	return model.DashboardCapabilities{
		UIRuns:   false,
		APIRuns:  hasPermission(claims, "menu.api_automation.read") || hasPermission(claims, "api.interface.read"),
		PerfRuns: false,
	}
}

func hasPermission(claims model.Claims, code string) bool {
	for _, permission := range claims.Permissions {
		if permission == code {
			return true
		}
	}
	return false
}

func hasProject(projects []model.DashboardProject, id int64) bool {
	for _, project := range projects {
		if project.ID == id {
			return true
		}
	}
	return false
}

func sortDashboardRecords(records []model.DashboardExecutionRecord) {
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].CreatedAt.Equal(records[j].CreatedAt) {
			if records[i].Type == records[j].Type {
				return records[i].ID > records[j].ID
			}
			return records[i].Type < records[j].Type
		}
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})
}

func addDashboardCounts(total, source model.DashboardExecutionCounts) model.DashboardExecutionCounts {
	total.Total += source.Total
	total.Success += source.Success
	total.Failed += source.Failed
	total.Running += source.Running
	total.Canceled += source.Canceled
	total.Unknown += source.Unknown
	return total
}

func dashboardRecentItem(record model.DashboardExecutionRecord) model.DashboardRecentItem {
	item := model.DashboardRecentItem{
		UID: dashboardUID(record.Type, record.ID), Type: record.Type, ID: record.ID, Title: record.Title, ProjectID: record.ProjectID,
		ProjectName: record.ProjectName, Status: mapDashboardStatus(record.Status), CreatedAt: record.CreatedAt,
		StartedAt: record.StartedAt, FinishedAt: record.FinishedAt, DurationMS: record.DurationMS,
		TargetURL: dashboardTargetURL(record.Type, record.ID),
	}
	if item.DurationMS == nil && item.StartedAt != nil && item.FinishedAt != nil {
		value := item.FinishedAt.Sub(*item.StartedAt).Milliseconds()
		if value >= 0 {
			item.DurationMS = &value
		}
	}
	return item
}

func dashboardAttention(records []model.DashboardExecutionRecord, partial bool) model.DashboardAttention {
	return dashboardAttentionWithTotal(records, partial, dashboardFailedCandidateCount(records))
}

func dashboardAttentionWithTotal(records []model.DashboardExecutionRecord, partial bool, total int64) model.DashboardAttention {
	items := make([]model.DashboardAttentionItem, 0, 3)
	for _, record := range records {
		if mapDashboardStatus(record.Status) != "failed" {
			continue
		}
		if len(items) >= 3 {
			continue
		}
		description := strings.TrimSpace(record.Description)
		if description == "" {
			description = fmt.Sprintf("%s 执行失败", dashboardTypeLabel(record.Type))
		}
		items = append(items, model.DashboardAttentionItem{
			UID: dashboardUID(record.Type, record.ID), ID: record.ID, Type: "failed", Source: record.Type,
			Title: record.Title, Description: description,
			ProjectID: record.ProjectID, ProjectName: record.ProjectName, CreatedAt: record.CreatedAt,
			TargetURL: dashboardTargetURL(record.Type, record.ID),
		})
	}
	state := "ok"
	if partial {
		state = "partial"
	}
	return model.DashboardAttention{State: state, Total: total, Items: items}
}

func dashboardFailedCandidateCount(records []model.DashboardExecutionRecord) int64 {
	var total int64
	for _, record := range records {
		if mapDashboardStatus(record.Status) == "failed" {
			total++
		}
	}
	return total
}

func dashboardUID(source, id string) string {
	return source + ":" + id
}

func dashboardTypeLabel(source string) string {
	switch source {
	case "ui":
		return "UI"
	case "api":
		return "API"
	case "perf":
		return "性能测试"
	default:
		return "执行"
	}
}

func dashboardTargetURL(source, id string) string {
	switch source {
	case "perf":
		return "#/performance/runs/" + id
	case "api":
		return "#/接口自动化/测试用例"
	default:
		return "#/执行中心/执行记录"
	}
}

func mapDashboardStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active", "pending", "queued", "dispatching", "dispatched", "running", "stopping":
		return "running"
	case "completed", "success":
		return "success"
	case "failed", "execution_failed", "threshold_failed", "timed_out":
		return "failed"
	case "canceled":
		return "canceled"
	default:
		return "unknown"
	}
}

// MapDashboardStatus 供其他后端模块复用统一状态语义。
func MapDashboardStatus(status string) string {
	return mapDashboardStatus(status)
}

// DashboardRange 计算 UTC 半开时间范围 [from,to)。
func DashboardRange(value string, now time.Time) (time.Time, time.Time, error) {
	return dashboardRange(value, now)
}

// DashboardRecent 合并、排序并截取首页最近执行记录。
func DashboardRecent(records []model.DashboardExecutionRecord, limit int) []model.DashboardRecentItem {
	copyRecords := append([]model.DashboardExecutionRecord(nil), records...)
	sortDashboardRecords(copyRecords)
	if limit < 0 {
		limit = 0
	}
	if len(copyRecords) > limit {
		copyRecords = copyRecords[:limit]
	}
	items := make([]model.DashboardRecentItem, 0, len(copyRecords))
	for _, record := range copyRecords {
		items = append(items, dashboardRecentItem(record))
	}
	return items
}

// DashboardAttentionItems 提供纯函数测试和复用入口。
func DashboardAttentionItems(records []model.DashboardExecutionRecord, partial bool) model.DashboardAttention {
	copyRecords := append([]model.DashboardExecutionRecord(nil), records...)
	sortDashboardRecords(copyRecords)
	return dashboardAttention(copyRecords, partial)
}
