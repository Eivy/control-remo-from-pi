package main

// Type is Appliance Type
type Type int

const (
	// IR is
	IR = iota
	// LIGHT is
	LIGHT
	// TV is
	TV
	// AirCon is
	AirCon
)

func (t Type) String() string {
	switch t {
	case LIGHT:
		return "LIGHT"
	case TV:
		return "RV"
	case AirCon:
		return "AirCon"
	case IR:
		return "IR"
	default:
		return "Unknown"
	}
}

// MarshalYAML define custom marshaling for Type
func (t Type) MarshalYAML() (interface{}, error) {
	return t.String(), nil
}

// UnmarshalYAML define custom marshaling for Type
func (t *Type) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var aux interface{}
	if err := unmarshal(&aux); err != nil {
		return err
	}
	switch aux.(string) {
	case "LIGHT":
		*t = LIGHT
	case "TV":
		*t = TV
	case "AirCon":
		*t = AirCon
	case "IR":
		*t = IR
	}
	return nil
}
