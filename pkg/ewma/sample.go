package ewma

import "time"

type Sample struct {
	Value     float64
	Timestamp time.Time
}
