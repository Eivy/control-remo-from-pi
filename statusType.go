package main

// StatusType shows
type StatusType int

const (
	// STR means straight
	STR = iota
	// REV means reverse
	REV
)

func (s StatusType) String() string {
	switch s {
	case STR:
		return "STR"
	case REV:
		return "REV"
	default:
		return "Unknown"
	}
}

// MarshalYAML define custom marshaling for Type
func (s StatusType) MarshalYAML() (interface{}, error) {
	return s.String(), nil
}

// UnmarshalYAML define custom marshaling for Type
func (s *StatusType) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var aux interface{}
	if err := unmarshal(&aux); err != nil {
		return err
	}
	switch aux.(string) {
	case "STR":
		*s = STR
	case "REV":
		*s = REV
	}
	return nil
}
