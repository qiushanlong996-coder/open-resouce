package main

import "testing"

func TestHotSearchTermsRankingAndFilters(t *testing.T) {
	store := newHotSearchTerms()

	store.record("agent")
	store.record("agent")
	store.record("agent")
	store.record("rag")
	store.record("rag")
	store.record("mcp")
	store.record(" a ")    // 过短（1 个字符），忽略
	store.record("")       // 空，忽略
	store.record("  rag ") // 去空白后与 rag 合并

	top := store.top(10)
	if len(top) != 3 {
		t.Fatalf("distinct terms = %d, want 3: %#v", len(top), top)
	}
	if top[0].Term != "agent" || top[0].Count != 3 {
		t.Fatalf("top[0] = %#v, want agent/3", top[0])
	}
	if top[1].Term != "rag" || top[1].Count != 3 {
		t.Fatalf("top[1] = %#v, want rag/3", top[1])
	}
	// agent 与 rag 同为 3，按词升序 agent 在前。
	if top[2].Term != "mcp" || top[2].Count != 1 {
		t.Fatalf("top[2] = %#v, want mcp/1", top[2])
	}

	if limited := store.top(2); len(limited) != 2 {
		t.Fatalf("top(2) len = %d, want 2", len(limited))
	}
}
