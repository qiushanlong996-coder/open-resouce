package main

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// Atom 订阅源：输出最近发布的已发布项目，便于用户用 RSS 阅读器关注平台。
// 条目链接指向 /projects/{slug}（前端支持深链直达）。

type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Xmlns   string      `xml:"xmlns,attr"`
	Title   string      `xml:"title"`
	ID      string      `xml:"id"`
	Link    []atomLink  `xml:"link"`
	Updated string      `xml:"updated"`
	Entries []atomEntry `xml:"entry"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr,omitempty"`
	Type string `xml:"type,attr,omitempty"`
}

type atomEntry struct {
	Title   string   `xml:"title"`
	ID      string   `xml:"id"`
	Updated string   `xml:"updated"`
	Summary string   `xml:"summary"`
	Link    atomLink `xml:"link"`
}

func siteBaseURL() string {
	if value := strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")); value != "" {
		return strings.TrimRight(value, "/")
	}
	return "https://www.openresource.cn"
}

func feedHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	projects, err := managedProjectRepositoryStore.ListPublished(request.Context())
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "订阅源暂时不可用")
		return
	}
	sort.Slice(projects, func(left, right int) bool {
		return projects[left].UpdatedAt.After(projects[right].UpdatedAt)
	})
	if len(projects) > 20 {
		projects = projects[:20]
	}

	base := siteBaseURL()
	feed := atomFeed{
		Xmlns: "http://www.w3.org/2005/Atom",
		Title: "新猿译码 - 最新开源项目",
		ID:    base + "/",
		Link: []atomLink{
			{Href: base + "/", Rel: "alternate", Type: "text/html"},
			{Href: base + "/feed.xml", Rel: "self", Type: "application/atom+xml"},
		},
		Updated: time.Now().UTC().Format(time.RFC3339),
	}
	for _, project := range projects {
		href := fmt.Sprintf("%s/projects/%s", base, url.PathEscape(project.Slug))
		feed.Entries = append(feed.Entries, atomEntry{
			Title:   project.Name,
			ID:      href,
			Updated: project.UpdatedAt.UTC().Format(time.RFC3339),
			Summary: project.Summary,
			Link:    atomLink{Href: href, Rel: "alternate", Type: "text/html"},
		})
	}
	writer.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	_ = xml.NewEncoder(writer).Encode(feed)
}
