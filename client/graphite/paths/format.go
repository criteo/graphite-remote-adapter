package paths

// FormatType describesCarbon format type
type FormatType int

// Format describes carbon format; It can include a list of filtered tags
// that must be exported using FormatCarbon
type Format struct {
	Type         FormatType
	FilteredTags []string // Only for Graphite Tag (Only used for FormatCarbonTags)
}

// Format values.
const (
	FormatCarbon            FormatType = 1
	FormatCarbonTags                   = 2
	FormatCarbonOpenMetrics            = 3
)
