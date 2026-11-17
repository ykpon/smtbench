package runner

import (
	"time"

	"golang.org/x/sys/unix"
)

const spinUs = 0

// osSleep sleeps in the kernel. time.Sleep wakes a locked thread through the
// Go scheduler, which adds about a millisecond on an idle machine and several
// under full load: that is runtime latency, not what a game server sees.
func osSleep(d time.Duration) {
	ts := unix.NsecToTimespec(d.Nanoseconds())
	for unix.Nanosleep(&ts, &ts) == unix.EINTR {
	}
}
