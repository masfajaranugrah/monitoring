//go:build !linux && !darwin

package handlers

import "context"

func cpuPercent(ctx context.Context) float64 {
	return -1
}

func memUsage() (total, used uint64) {
	return 0, 0
}