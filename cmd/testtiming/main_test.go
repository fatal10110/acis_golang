package main

import (
	"strings"
	"testing"
)

func TestSummarizeKeepsTopLevelTestsSeparateFromSubtests(t *testing.T) {
	input := strings.NewReader(`{"Action":"pass","Package":"example/slow","Test":"TestParent/child","Elapsed":2}
{"Action":"pass","Package":"example/slow","Test":"TestParent","Elapsed":3}
{"Action":"pass","Package":"example/slow","Elapsed":4}
`)
	got, err := summarize(input, 1)
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	if !strings.Contains(got, "example/slow 4.000s") || !strings.Contains(got, "example/slow TestParent 3.000s") {
		t.Fatalf("summary = %q, want package and parent test", got)
	}
	if strings.Contains(got, "child") {
		t.Fatalf("summary included nested test: %q", got)
	}
}
