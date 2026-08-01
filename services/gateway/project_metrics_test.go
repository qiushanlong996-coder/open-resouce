package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMemoryProjectMetrics(t *testing.T) {
	repository := newMemoryProjectMetricsRepository()
	ctx := context.Background()
	_ = repository.IncrementView(ctx, "atlas")
	_ = repository.IncrementView(ctx, "atlas")
	_ = repository.IncrementDownload(ctx, "atlas")

	snapshot, err := repository.Snapshot(ctx, []string{"atlas", "missing"})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot["atlas"].Views != 2 || snapshot["atlas"].Downloads != 1 {
		t.Fatalf("atlas metrics = %#v, want views 2 downloads 1", snapshot["atlas"])
	}
	if _, present := snapshot["missing"]; present {
		t.Error("missing project must not appear")
	}
}

func TestProjectViewBeaconAndDetailReflectsViews(t *testing.T) {
	original := projectMetricsRepositoryStore
	projectMetricsRepositoryStore = newMemoryProjectMetricsRepository()
	t.Cleanup(func() { projectMetricsRepositoryStore = original })

	// 打两次浏览 beacon（种子项目 atlas-agent）。
	for range 2 {
		response := httptest.NewRecorder()
		newHandler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/projects/atlas-agent/view", nil))
		if response.Code != http.StatusNoContent {
			t.Fatalf("view beacon status = %d", response.Code)
		}
	}

	// 详情应反映 views=2。
	detailResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(detailResponse, httptest.NewRequest(http.MethodGet, "/api/v1/projects/atlas-agent", nil))
	var detail projectDetailResponse
	if err := json.Unmarshal(detailResponse.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Data.Metrics.Views != 2 {
		t.Fatalf("detail views = %d, want 2", detail.Data.Metrics.Views)
	}

	// 未知项目的 beacon 返回 404。
	unknown := httptest.NewRecorder()
	newHandler().ServeHTTP(unknown, httptest.NewRequest(http.MethodPost, "/api/v1/projects/does-not-exist/view", nil))
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown project view = %d, want 404", unknown.Code)
	}
}
