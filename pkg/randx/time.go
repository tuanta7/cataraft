package randx

import (
	"math/rand"
	"time"
)

func RandomDuration(min, max time.Duration) time.Duration {
	r := rand.Intn(int(max - min))
	return min + time.Duration(r)
}
