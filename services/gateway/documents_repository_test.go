package main

import "testing"

func TestSeedDocumentRepository(t *testing.T) {
	repository := seedDocumentRepository{documents: seedDocuments}

	nodes, found := repository.List("atlas-agent")
	if !found || len(nodes) != 1 || nodes[0].Slug != "quick-start" {
		t.Fatalf("unexpected Atlas document list: found=%v nodes=%#v", found, nodes)
	}

	document, projectFound, documentFound := repository.Get("atlas-agent", "quick-start")
	if !projectFound || !documentFound || document.ID != "doc-atlas-quick-start" {
		t.Fatalf("unexpected document result: project=%v document=%v data=%#v", projectFound, documentFound, document)
	}

	_, projectFound, documentFound = repository.Get("missing", "quick-start")
	if projectFound || documentFound {
		t.Fatalf("missing project result = (%v, %v), want false, false", projectFound, documentFound)
	}

	_, projectFound, documentFound = repository.Get("atlas-agent", "missing")
	if !projectFound || documentFound {
		t.Fatalf("missing document result = (%v, %v), want true, false", projectFound, documentFound)
	}
}
