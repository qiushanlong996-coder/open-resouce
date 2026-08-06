package main

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"sort"
)

// sitemap.xml：收录首页与全部已发布项目深链，配合 robots/搜索引擎抓取。

type urlSet struct {
	XMLName xml.Name  `xml:"urlset"`
	Xmlns   string    `xml:"xmlns,attr"`
	URLs    []urlItem `xml:"url"`
}

type urlItem struct {
	Loc        string `xml:"loc"`
	LastMod    string `xml:"lastmod,omitempty"`
	Changefreq string `xml:"changefreq,omitempty"`
	Priority   string `xml:"priority,omitempty"`
}

func sitemapHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeAPIError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持")
		return
	}
	projects, err := managedProjectRepositoryStore.ListPublished(request.Context())
	if err != nil {
		writeAPIError(writer, request, http.StatusInternalServerError, "repository_error", "站点地图暂时不可用")
		return
	}
	sort.Slice(projects, func(left, right int) bool {
		return projects[left].UpdatedAt.After(projects[right].UpdatedAt)
	})

	base := siteBaseURL()
	entries := []urlItem{
		{Loc: base + "/", Changefreq: "daily", Priority: "1.0"},
	}
	for _, project := range projects {
		entries = append(entries, urlItem{
			Loc:        fmt.Sprintf("%s/projects/%s", base, url.PathEscape(project.Slug)),
			LastMod:    project.UpdatedAt.UTC().Format("2006-01-02"),
			Changefreq: "weekly",
			Priority:   "0.8",
		})
	}
	writer.Header().Set("Content-Type", "application/xml; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	_ = xml.NewEncoder(writer).Encode(urlSet{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  entries,
	})
}
