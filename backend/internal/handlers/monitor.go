package handlers

import "monitoring/internal/ping"

// MonitorEngine is wired in from cmd/server so customer mutations can nudge
// the background monitor to pick up changes immediately instead of waiting
// for the periodic reload.
var MonitorEngine *ping.MonitorEngine