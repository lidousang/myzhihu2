package service

import (
	"slices"
	"testing"
)

func TestMergeDesc(t *testing.T) {
	inbox := []uint{30, 21, 12, 5}
	bigV1 := []uint{29, 21, 11}
	bigV2 := []uint{40, 3}
	got := mergeDesc([][]uint{inbox, bigV1, bigV2}, 6)
	want := []uint{40, 30, 29, 21, 12, 11}
	if !slices.Equal(got, want) {
		t.Fatalf("mergeDesc = %v, want %v", got, want)
	}
	if got := mergeDesc([][]uint{{}, nil}, 5); len(got) != 0 {
		t.Fatalf("empty lists merged to %v", got)
	}
}

func TestTopInterestSignalsUsesTopics(t *testing.T) {
	authors, topics := topInterestSignals(map[string]int64{
		"author:118": 9, "author:119": 6, "topic:photo": 24, "topic:tech": 8,
		"topic:fitness": 3, "topic:food": 1, "topic:bogus": 50, "kind:article": 99,
	})
	if !slices.Equal(authors, []uint{118, 119}) {
		t.Fatalf("authors = %v", authors)
	}
	// "bogus" is rejected before ranking, fitness and food are below 30% of
	// the strongest topic, and legacy kind fields are ignored.
	if !slices.Equal(topics, []string{"photo", "tech"}) {
		t.Fatalf("topics = %v", topics)
	}
}

func TestIntersectAndSubtract(t *testing.T) {
	if got := intersect([]uint{1, 2, 100, 3}, []uint{3, 1}); !slices.Equal(got, []uint{1, 3}) {
		t.Fatalf("intersect = %v", got)
	}
	if got := subtract([]uint{5, 4, 3}, []uint{4}); !slices.Equal(got, []uint{5, 3}) {
		t.Fatalf("subtract = %v", got)
	}
}
