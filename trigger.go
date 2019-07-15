package main

// Trigger is trigger type
type Trigger int

const (
	// TOGGLE is toggle
	TOGGLE = iota
	// SYNC is syncronization
	SYNC
)

func (t Trigger) String() string {
	switch t {
	case TOGGLE:
		return "TOGGLE"
	case SYNC:
		return "SYNC"
	default:
		return "Unknown"
	}
}

// MarshalYAML define custom marshaling for Type
func (t Trigger) MarshalYAML() (interface{}, error) {
	return t.String(), nil
}

// UnmarshalYAML define custom marshaling for Type
func (t *Trigger) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var aux interface{}
	if err := unmarshal(&aux); err != nil {
		return err
	}
	switch aux.(string) {
	case "TOGGLE":
		*t = TOGGLE
	case "SYNC":
		*t = SYNC
	}
	return nil
}
