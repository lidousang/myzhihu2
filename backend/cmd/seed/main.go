// Command seed generates the deterministic demo dataset and doubles as the
// daily reset tool:
//
//	go run ./cmd/seed -f settings/settings.yaml -profile demo -seed 20261107 -reset
//
// It replaces ALL business data in the configured database, so it refuses to
// run unless -reset is given and demo.enabled is true (or -force is used).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"myzhihu2/core"
	"myzhihu2/internal/seed"
	"myzhihu2/service"
	"myzhihu2/util/flags"
	"myzhihu2/util/global"
)

func main() {
	configPath := flag.String("f", "settings/settings.yaml", "settings file")
	profileName := flag.String("profile", "demo", "dataset profile: demo or stress")
	seedValue := flag.Uint64("seed", 20261107, "random seed; the same seed produces the same data")
	reset := flag.Bool("reset", false, "truncate all business tables before inserting (required)")
	purge := flag.Bool("purge-queues", false, "purge the feed and behavior RabbitMQ queues")
	dryRun := flag.Bool("dry-run", false, "generate and print statistics without writing")
	force := flag.Bool("force", false, "allow running when demo.enabled is false")
	flag.Parse()

	if err := run(*configPath, *profileName, *seedValue, *reset, *purge, *dryRun, *force); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func run(configPath, profileName string, seedValue uint64, reset, purge, dryRun, force bool) error {
	profile, ok := seed.Profiles[profileName]
	if !ok {
		return fmt.Errorf("unknown profile %q", profileName)
	}
	flags.FlagOption.File = configPath
	global.Conf = core.ReadConf()
	cfg := global.Conf
	if !dryRun {
		if !reset {
			return fmt.Errorf("refusing to run without -reset: seeding replaces all business data in %q", cfg.DB.Name)
		}
		if !cfg.Demo.Enabled && !force {
			return fmt.Errorf("refusing to reset %q: demo.enabled is false (use -force for a dedicated test database)", cfg.DB.Name)
		}
	}

	started := time.Now()
	ds, err := seed.Generate(profile, seedValue, started)
	if err != nil {
		return err
	}
	fmt.Printf("profile=%s seed=%d users=%d follows=%d contents=%d (showcase rows %d) reactions=%d behaviors=%d online=%d\n",
		profile.Name, seedValue, len(ds.Users), len(ds.Follows), len(ds.Contents), ds.ShowcaseCount,
		len(ds.Reactions), len(ds.Behaviors), len(ds.OnlineUsers))
	if dryRun {
		return nil
	}

	ctx := context.Background()
	global.DB = core.InitDB()
	fmt.Printf("loading into database %q ...\n", cfg.DB.Name)
	if err := seed.Load(ctx, global.DB, ds, reset); err != nil {
		return err
	}

	global.Redis = core.InitRedis()
	if global.Redis != nil {
		// Projections from the previous dataset would reference deleted rows.
		if err := global.Redis.FlushDB(ctx).Err(); err != nil {
			return fmt.Errorf("flush redis db %d: %w", cfg.Redis.DB, err)
		}
		proj := service.GetProjection()
		if err := proj.EnsureBigVSet(ctx); err != nil {
			return err
		}
		if err := proj.RefreshExplorePool(ctx); err != nil {
			return err
		}
		for _, id := range ds.OnlineUsers {
			if err := proj.RebuildInbox(ctx, id); err != nil {
				return err
			}
		}
	}
	if purge {
		if err := purgeQueues(); err != nil {
			return err
		}
	}

	failed := 0
	for _, c := range seed.Verify(ctx, global.DB, ds, profile, cfg.Feed.BigVThreshold, cfg.Demo.ProtectedMaxContentID) {
		mark := "PASS"
		if !c.OK && c.Warn {
			mark = "WARN"
		} else if !c.OK {
			mark = "FAIL"
			failed++
		}
		fmt.Printf("  [%s] %-38s %s\n", mark, c.Name, c.Detail)
	}
	fmt.Printf("done in %s\n", time.Since(started).Round(time.Millisecond))
	if failed > 0 {
		return fmt.Errorf("%d verification check(s) failed", failed)
	}
	return nil
}

// purgeQueues drops messages that reference the previous dataset.
func purgeQueues() error {
	if !global.Conf.RabbitMq.Enabled {
		return nil
	}
	conn := core.InitRMQ()
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	for _, q := range []string{global.Conf.RabbitMq.FeedQueue, global.Conf.RabbitMq.Queue} {
		n, err := ch.QueuePurge(q, false)
		if err != nil {
			return fmt.Errorf("purge %s: %w", q, err)
		}
		fmt.Printf("purged %d message(s) from %s\n", n, q)
	}
	return nil
}
