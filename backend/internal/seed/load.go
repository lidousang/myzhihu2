package seed

import (
	"context"
	"fmt"

	"myzhihu2/repository/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// businessTables are emptied by a reset, children before parents.
var businessTables = []string{"outbox_events", "behavior_events", "reactions", "user_follows", "contents", "users"}

// Load migrates the schema, optionally truncates every business table and
// inserts the dataset. Rows are written directly, bypassing the service layer,
// so no outbox events are produced; Redis projections are rebuilt lazily.
func Load(ctx context.Context, db *gorm.DB, ds *Dataset, reset bool) error {
	db = db.WithContext(ctx)
	if err := db.AutoMigrate(model.All()...); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if reset {
		// TRUNCATE and the session variable must run on the same connection.
		err := db.Connection(func(conn *gorm.DB) error {
			if err := conn.Exec("SET FOREIGN_KEY_CHECKS = 0").Error; err != nil {
				return err
			}
			defer conn.Exec("SET FOREIGN_KEY_CHECKS = 1")
			for _, table := range businessTables {
				if err := conn.Exec("TRUNCATE TABLE " + table).Error; err != nil {
					return fmt.Errorf("truncate %s: %w", table, err)
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	steps := []struct {
		name string
		rows any
	}{
		{"users", &ds.Users},
		{"user_follows", &ds.Follows},
		{"contents", &ds.Contents},
		{"reactions", &ds.Reactions},
		{"behavior_events", &ds.Behaviors},
	}
	for _, step := range steps {
		// Omit associations: Content.Author etc. are zero structs here and must
		// not be upserted.
		if err := db.Omit(clause.Associations).CreateInBatches(step.rows, 1000).Error; err != nil {
			return fmt.Errorf("insert %s: %w", step.name, err)
		}
	}
	// Without this, InnoDB would continue right after the largest seeded ID
	// (a comment around 20070), inside the protected range.
	if err := db.Exec(fmt.Sprintf("ALTER TABLE contents AUTO_INCREMENT = %d", DemoContentStartID)).Error; err != nil {
		return fmt.Errorf("set contents AUTO_INCREMENT: %w", err)
	}
	return nil
}
