// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package term

// SGR sequences of the 2.x palette (lib/core/output.sh), byte for byte.
const (
	SeqOff       = "\x1b[0m"
	SeqRed       = "\x1b[0;31m"
	SeqGreen     = "\x1b[0;32m"
	SeqYellow    = "\x1b[0;33m"
	SeqBlue      = "\x1b[0;34m"
	SeqCyan      = "\x1b[0;36m"
	SeqWhite     = "\x1b[0;37m"
	SeqBoldRed   = "\x1b[1;31m"
	SeqBoldGreen = "\x1b[1;32m"
	SeqBoldYel   = "\x1b[1;33m"
	SeqBoldBlue  = "\x1b[1;34m"
	SeqBoldCyan  = "\x1b[1;36m"
	SeqBoldWhite = "\x1b[1;37m"
	SeqBold      = "\x1b[1m"
	SeqDim       = "\x1b[2m"
	SeqUnderline = "\x1b[4m"
	SeqPrimary   = "\x1b[38;5;39m"
	SeqSecondary = "\x1b[38;5;243m"
	SeqAccent    = "\x1b[38;5;214m"
	SeqMuted     = "\x1b[38;5;245m"
	SeqSuccess   = "\x1b[38;5;82m"
	SeqWarning   = "\x1b[38;5;220m"
	SeqError     = "\x1b[38;5;196m"
	SeqInfo      = "\x1b[38;5;75m"
)

// Terminal control sequences: not colors, kept when colors are off, and
// only ever written to a terminal.
const (
	EraseEOL    = "\x1b[K"
	CursorHide  = "\x1b[?25l"
	CursorShow  = "\x1b[?25h"
	ClearScreen = "\x1b[H\x1b[2J"
)

// Palette holds the values of the 2.x color variables (__GASH_*).
type Palette struct {
	Off                                      string
	Red, Green, Yellow, Blue, Cyan, White    string
	BoldRed, BoldGreen, BoldYellow, BoldBlue string
	BoldCyan, BoldWhite                      string
	Bold, Dim, Underline                     string
	Primary, Secondary, Accent, Muted        string
	Success, Warning, Error, Info            string
}

// Colors is the palette with every sequence set.
var Colors = Palette{
	Off: SeqOff,
	Red: SeqRed, Green: SeqGreen, Yellow: SeqYellow, Blue: SeqBlue, Cyan: SeqCyan, White: SeqWhite,
	BoldRed: SeqBoldRed, BoldGreen: SeqBoldGreen, BoldYellow: SeqBoldYel, BoldBlue: SeqBoldBlue,
	BoldCyan: SeqBoldCyan, BoldWhite: SeqBoldWhite,
	Bold: SeqBold, Dim: SeqDim, Underline: SeqUnderline,
	Primary: SeqPrimary, Secondary: SeqSecondary, Accent: SeqAccent, Muted: SeqMuted,
	Success: SeqSuccess, Warning: SeqWarning, Error: SeqError, Info: SeqInfo,
}

// NoColors is the palette with every variable empty (colors off).
var NoColors = Palette{}

// Style returns the sequence of a semantic style name (__gash_color_of):
// primary, secondary, accent, muted, success, warning, error, info, bold,
// dim, underline; "" or none for no style; anything else as given.
func (p Palette) Style(name string) string {
	switch name {
	case "", "none":
		return ""
	case "primary":
		return p.Primary
	case "secondary":
		return p.Secondary
	case "accent":
		return p.Accent
	case "muted":
		return p.Muted
	case "success":
		return p.Success
	case "warning":
		return p.Warning
	case "error":
		return p.Error
	case "info":
		return p.Info
	case "bold":
		return p.Bold
	case "dim":
		return p.Dim
	case "underline":
		return p.Underline
	}
	return name
}

// Glyphs holds the 2.x glyph variables (__GASH_GLYPH_*).
type Glyphs struct {
	OK, Err, Warn, Info, Arrow, Bullet string
	HLine, VLine, Corners              string
	BarFull, BarEmpty                  string
	Dot, Ellipsis, Pointer             string
	TreeMid, TreeEnd                   string
	SpinnerFrames                      string
}

// UnicodeGlyphs is the set of a UTF-8 locale.
var UnicodeGlyphs = Glyphs{
	OK: "✓", Err: "✗", Warn: "!", Info: "•", Arrow: "→", Bullet: "•",
	HLine: "─", VLine: "│", Corners: "╭╮╰╯",
	BarFull: "━", BarEmpty: "─",
	Dot: "·", Ellipsis: "…", Pointer: "❯",
	TreeMid: "├─", TreeEnd: "└─",
	SpinnerFrames: "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏",
}

// ASCIIGlyphs is the set of any other locale.
var ASCIIGlyphs = Glyphs{
	OK: "+", Err: "x", Warn: "!", Info: "*", Arrow: "->", Bullet: "*",
	HLine: "-", VLine: "|", Corners: "++++",
	BarFull: "#", BarEmpty: ".",
	Dot: "-", Ellipsis: "~", Pointer: ">",
	TreeMid: "|-", TreeEnd: "`-",
	SpinnerFrames: `|/-\`,
}
