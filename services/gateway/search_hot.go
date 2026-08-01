package main

import (
	"sort"
	"strings"
	"sync"
)

// 热门搜索词：进程内聚合。
//
// 取舍：用内存计数而不是落库——热门词是弱一致的展示数据，重启清零可接受，
// 也不需要为它引入迁移。多实例部署时各实例各自统计（与归档缓存同类限制）。
// 为防内存无界增长，限制去重词条上限；超限后不再登记新词，仅累计已有词。

const (
	hotTermMinRunes    = 2
	hotTermMaxRunes    = 40
	hotTermMaxDistinct = 5000
)

type hotSearchTerms struct {
	sync.Mutex
	counts map[string]int
}

func newHotSearchTerms() *hotSearchTerms {
	return &hotSearchTerms{counts: make(map[string]int)}
}

// record 登记一次搜索。过短、过长的词忽略。
func (store *hotSearchTerms) record(query string) {
	term := strings.TrimSpace(query)
	runes := len([]rune(term))
	if runes < hotTermMinRunes || runes > hotTermMaxRunes {
		return
	}
	store.Lock()
	defer store.Unlock()
	if _, exists := store.counts[term]; !exists && len(store.counts) >= hotTermMaxDistinct {
		return
	}
	store.counts[term]++
}

// top 返回计数最高的前 n 个词，按计数降序、同计数按词升序稳定排序。
func (store *hotSearchTerms) top(n int) []hotSearchTerm {
	store.Lock()
	terms := make([]hotSearchTerm, 0, len(store.counts))
	for term, count := range store.counts {
		terms = append(terms, hotSearchTerm{Term: term, Count: count})
	}
	store.Unlock()

	sort.Slice(terms, func(left, right int) bool {
		if terms[left].Count != terms[right].Count {
			return terms[left].Count > terms[right].Count
		}
		return terms[left].Term < terms[right].Term
	})
	if len(terms) > n {
		terms = terms[:n]
	}
	return terms
}

var hotSearchTermsStore = newHotSearchTerms()
