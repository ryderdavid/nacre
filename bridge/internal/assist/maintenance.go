package assist

// Busy includes both synchronous calls and detached turns waiting to start.
func (r *Runner) Busy() bool { return r.active.Load() > 0 }
