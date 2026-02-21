package ur

import (
	"testing"
)

func TestSplit2of3OneURPerShare(t *testing.T) {
	data := Data{Data: []byte("hello-world"), Threshold: 2, Shards: 3}
	for i := 0; i < 3; i++ {
		got := Split(data, i)
		if len(got) != 1 {
			t.Fatalf("share %d: got %d URs, want 1", i+1, len(got))
		}
	}
}

func TestSplit3of5TwoURPerShare(t *testing.T) {
	data := Data{Data: []byte("hello-world"), Threshold: 3, Shards: 5}
	for i := 0; i < 5; i++ {
		got := Split(data, i)
		if len(got) != 2 {
			t.Fatalf("share %d: got %d URs, want 2", i+1, len(got))
		}
	}
}

