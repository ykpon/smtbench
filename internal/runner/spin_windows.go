package runner

import "time"

// spinUs: sleep on Windows wakes up to a millisecond late.
const spinUs = 1000

func osSleep(d time.Duration) { time.Sleep(d) }
