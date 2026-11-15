package model

// All returns every persisted model in migration order. It is shared by the
// -db migration flag and the demo-data seeder.
func All() []any {
	return []any{
		&User{}, &Content{}, &UserFollow{}, &Reaction{}, &BehaviorEvent{}, &OutboxEvent{},
	}
}
