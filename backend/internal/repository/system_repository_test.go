package repository

import (
	"strings"
	"testing"
)

func TestSystemRunTrendQueryUsesUnambiguousDateAlias(t *testing.T) {
	query := systemRunTrendQuery()
	if strings.Contains(query, "::date day") || strings.Contains(query, "days.day") || strings.Contains(query, "runs.day") {
		t.Fatalf("趋势查询仍使用 PostgreSQL 关键字 day 作为列别名：%s", query)
	}
	if !strings.Contains(query, "as run_date") {
		t.Fatalf("趋势查询未使用明确的 run_date 别名：%s", query)
	}
}
