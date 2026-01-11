package controlremo

// Trigger is trigger type
type Trigger string

const (
	// TOGGLE is toggle
	TriggerTOGGLE Trigger = "TOGGLE"
	// SYNC is syncronization
	TriggerSYNC Trigger = "SYNC"
	// TriggerTimer is syncronization
	TriggerTimer Trigger = "TIMER"
)
