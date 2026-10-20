package yt

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestLivePodcastsAndMusic(t *testing.T) {
	if os.Getenv("RUN_LIVE_CHECK") != "1" {
		t.Skip("skipping live test without RUN_LIVE_CHECK=1")
	}

	acc := NewAccount()
	if !acc.SignedIn() {
		t.Fatal("account is not signed in")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	t0 := time.Now()
	shows, err := acc.PodcastsFeed(ctx)
	dPodcasts := time.Since(t0)
	if err != nil {
		t.Fatalf("Podcasts feed error: %v", err)
	}
	fmt.Printf("LIVE CHECK: podcasts count=%d time=%v\n", len(shows), dPodcasts)

	t1 := time.Now()
	tracks, err := acc.LikedMusic(ctx)
	dMusic := time.Since(t1)
	if err != nil {
		t.Fatalf("Liked music error: %v", err)
	}
	fmt.Printf("LIVE CHECK: liked_music count=%d time=%v\n", len(tracks), dMusic)
}
