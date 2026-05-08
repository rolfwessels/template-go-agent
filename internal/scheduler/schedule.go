package scheduler

type Schedule struct {
	ID              string `json:"id"`
	UserID          string `json:"user_id"`
	ChannelID       string `json:"channel_id"`
	Prompt          string `json:"prompt"`
	NextFireAt      int64  `json:"next_fire_at"`
	IntervalSeconds int64  `json:"interval_seconds,omitempty"`
}
