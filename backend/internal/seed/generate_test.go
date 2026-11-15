package seed

import (
	"reflect"
	"slices"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

func TestGenerateIsDeterministic(t *testing.T) {
	a, err := Generate(Profiles["demo"], 42, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Generate(Profiles["demo"], 42, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same seed produced different datasets")
	}
	c, _ := Generate(Profiles["demo"], 43, fixedNow)
	if reflect.DeepEqual(a.Follows, c.Follows) {
		t.Fatal("different seeds produced identical follow graphs")
	}
}

func TestDemoDatasetShape(t *testing.T) {
	p := Profiles["demo"]
	ds, err := Generate(p, 20261107, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	const threshold = 100
	var bigVs []uint
	for _, u := range ds.Users {
		if u.FollowerCount >= threshold {
			bigVs = append(bigVs, u.ID)
		}
		if u.PasswordHash != UnusablePasswordHash {
			t.Fatalf("user %d has a usable password hash", u.ID)
		}
	}
	if !slices.Equal(bigVs, []uint{BigVTechID, BigVFoodID, BigVTravelID}) {
		t.Fatalf("big Vs = %v, want [1 2 3]", bigVs)
	}
	var following []uint
	for _, f := range ds.Follows {
		if f.FollowerID == ReaderID {
			following = append(following, f.FollowingID)
		}
	}
	if !slices.Equal(following, readerFollows) {
		t.Fatalf("reader follows %v, want %v", following, readerFollows)
	}
	maxFiller, minShowcase := uint(0), ^uint(0)
	for _, c := range ds.Contents {
		if c.AuthorID == ReaderID {
			t.Fatal("reader must not author content")
		}
		if c.Topic == "" {
			t.Fatalf("content %d has no topic", c.ID)
		}
		if c.ID < ShowcaseStartID {
			maxFiller = max(maxFiller, c.ID)
		} else if c.ID < CommentStartID {
			minShowcase = min(minShowcase, c.ID)
		}
		if c.CreatedAt.After(fixedNow) {
			t.Fatalf("content %d is in the future", c.ID)
		}
	}
	if maxFiller >= minShowcase {
		t.Fatal("filler IDs must be lower than showcase IDs")
	}
	if ds.HotContentID == 0 {
		t.Fatal("hot content was not marked")
	}
	hot := 0
	readerLikes := 0
	for _, r := range ds.Reactions {
		if r.ContentID == ds.HotContentID && r.Kind == "like" {
			hot++
		}
		if r.UserID == ReaderID && r.Kind == "like" {
			readerLikes++
		}
	}
	if hot < p.HotLikes {
		t.Fatalf("hot content has %d likes, want >= %d", hot, p.HotLikes)
	}
	if readerLikes != 8 {
		t.Fatalf("reader has %d likes, want 8 photo likes", readerLikes)
	}
}

// The newest showcase IDs must rotate through big V tech, interest photo,
// followed mid author, explore and big V food, in that order.
func TestShowcaseRotation(t *testing.T) {
	ds, err := Generate(Profiles["demo"], 1, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	var roots []uint // newest first, roots only
	author := map[uint]uint{}
	for i := len(ds.Contents) - 1; i >= 0; i-- {
		c := ds.Contents[i]
		if c.ID >= ShowcaseStartID && c.ID < CommentStartID && c.ParentID == nil {
			roots = append(roots, c.ID)
			author[c.ID] = c.AuthorID
		}
	}
	for i, want := range []int{0, 1, 2, 3, 4} {
		if got := showcaseClass(author[roots[i]]); got != want {
			t.Fatalf("root %d (content %d) has class %d, want %d", i, roots[i], got, want)
		}
	}
}
