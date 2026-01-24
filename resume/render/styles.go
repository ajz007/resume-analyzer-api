package render

// RunStyle captures the inline run formatting expected in templates.
type RunStyle struct {
	Bold   bool
	Italic bool
	Size   int
	Color  string
}

const (
	HeadingColor = "1F2937"
	NameColor    = "111111"
	HeadingSize  = 24
	NameSize     = 32
)

// StyleMap centralizes the expected formatting for key resume elements.
var StyleMap = map[string]RunStyle{
	"name": {
		Bold:  true,
		Size:  NameSize,
		Color: NameColor,
	},
	"sectionHeading": {
		Bold:  true,
		Size:  HeadingSize,
		Color: HeadingColor,
	},
	"roleLine": {
		Bold: true,
	},
	"meta": {
		Italic: true,
	},
}
