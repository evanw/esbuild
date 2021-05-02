package api_helpers

// This flag is set by the CLI to activate the timer. It's put here instead of
// by the timer to discourage code from checking this flag. Only the code that
// creates the root timer should check this flag. Other code should check that
// the timer is not null to detect if the timer is being used or not.
var UseTimer bool
