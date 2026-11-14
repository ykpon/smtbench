// Package affinity pins OS threads to logical CPUs and reads per-thread CPU time.
// Callers must hold runtime.LockOSThread while using Pin, Current and ThreadCPUTime.
package affinity
