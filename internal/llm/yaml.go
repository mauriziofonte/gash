// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"errors"
	"io"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"

	"github.com/mauriziofonte/gash/internal/contract"
)

// The YAML converter of llm_config (LEDGER LLM-35). 2.x printed a YAML
// file through python-yq: PyYAML built Python values with yq's YAML 1.2
// resolvers, json.dumps wrote them, jq printed them. This converter makes
// the same values and prints them as `jq .` did:
//
//   - plain scalars resolve by the YAML 1.2 core schema (010 is ten, where
//     python-yq's integer constructor read it as octal);
//   - a mapping is a Python dict: keys in first-seen order, a repeated key
//     keeps its first place and its last value, keys equal as Python
//     values are one key (1, 1.0 and true), and jq then merges the keys
//     that print alike (1 and "1");
//   - merge keys (<<) expand as PyYAML's flatten_mapping did;
//   - explicit tags go through PyYAML's constructors; a tag it has no
//     constructor for (an application tag, !!binary, !!set) gives the
//     node's content;
//   - an alias shares the value of its anchor, and a stream that would
//     write more than 1024 characters of JSON per character of YAML is
//     refused, as python-yq refused it.
//
// Whatever python-yq failed on (a parse error, an unhashable or a date
// key, a merge of a scalar, a value its constructor refuses) is an error,
// and the caller prints the raw text.

var (
	errYAMLKey    = errors.New("yaml: a key with no JSON form")
	errYAMLMerge  = errors.New("yaml: a merge key needs a mapping or a sequence of mappings")
	errYAMLTag    = errors.New("yaml: the node does not fit its tag")
	errYAMLBudget = errors.New("yaml: the document expands too far")
	errYAMLChar   = errors.New("yaml: a character YAML does not allow")
	errYAMLAnchor = errors.New("yaml: an anchor defined twice")
	errYAMLCycle  = errors.New("yaml: a value that contains itself")
	errYAMLBigInt = errors.New("yaml: an integer too long for Python's conversion")
)

const (
	// yamlExpansion is python-yq's max_expansion_factor.
	yamlExpansion = 1024
	// yamlWorkLimit bounds the nodes and merged pairs of one stream.
	yamlWorkLimit = 1 << 24
	// pyMaxIntDigits is Python's default int_max_str_digits.
	pyMaxIntDigits = 4300
	// The decoder's errors for a %YAML other than 1.1 and for a directive
	// other than YAML and TAG, which PyYAML took.
	yamlVersionErr   = "found incompatible YAML document"
	yamlDirectiveErr = "found unknown directive name"
)

// yamlDocs converts every document of a YAML stream. An empty stream (no
// document, or comments only) gives none: python-yq printed nothing, and
// 2.x then printed the raw text.
func yamlDocs(data []byte) ([]*yv, error) {
	if err := yamlPrintable(data); err != nil {
		return nil, err
	}
	docs, err := decodeYAML(data)
	if err != nil && (strings.Contains(err.Error(), yamlVersionErr) || strings.Contains(err.Error(), yamlDirectiveErr)) {
		if fixed, ok := pyDirectives(data); ok {
			docs, err = decodeYAML(fixed)
		}
	}
	if err != nil {
		return nil, err
	}
	limit := yamlExpansion * utf8.RuneCount(data)
	c := &yconv{memo: map[*yaml.Node]*yv{}, flat: map[*yaml.Node][]*yaml.Node{}}
	out := make([]*yv, 0, len(docs))
	total := 0
	for _, doc := range docs {
		if err := checkAnchors(doc, map[string]bool{}); err != nil {
			return nil, err
		}
		v, err := c.value(doc)
		if err != nil {
			return nil, err
		}
		if total = addSize(total, v.size); total > limit {
			return nil, errYAMLBudget
		}
		out = append(out, v)
	}
	return out, nil
}

func decodeYAML(data []byte) ([]*yaml.Node, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var docs []*yaml.Node
	for {
		doc := new(yaml.Node)
		err := dec.Decode(doc)
		if errors.Is(err, io.EOF) {
			return docs, nil
		}
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
}

var (
	yamlVersionLine = regexp.MustCompile(`(?m)^%YAML[ \t]+1\.([0-9]+)(?:[ \t\r]|$)`)
	yamlOtherLine   = regexp.MustCompile(`(?m)^%([0-9A-Za-z_-]+)(?:[ \t\r]|$)`)
)

// pyDirectives makes the directive lines PyYAML took and the decoder
// refuses into ones it takes, every offset kept: %YAML 1.x becomes 1.1
// (PyYAML warned about x other than 1), a directive other than YAML and
// TAG a comment (PyYAML ignored it). Used only after the decoder refused
// the text.
func pyDirectives(data []byte) ([]byte, bool) {
	out := append([]byte(nil), data...)
	changed := false
	for _, m := range yamlVersionLine.FindAllSubmatchIndex(data, -1) {
		out[m[2]] = '1'
		for i := m[2] + 1; i < m[3]; i++ {
			out[i] = ' '
		}
		changed = true
	}
	for _, m := range yamlOtherLine.FindAllSubmatchIndex(data, -1) {
		if name := string(data[m[2]:m[3]]); name != "YAML" && name != "TAG" {
			out[m[0]] = '#'
			changed = true
		}
	}
	return out, changed
}

// yamlPrintable refuses what PyYAML's reader refused: bytes that are not
// UTF-8 and characters outside YAML's printable set.
func yamlPrintable(data []byte) error {
	for len(data) > 0 {
		r, n := utf8.DecodeRune(data)
		if r == utf8.RuneError && n <= 1 {
			return errYAMLChar
		}
		switch {
		case r == '\t' || r == '\n' || r == '\r' || r == 0x85:
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0):
			return errYAMLChar
		case r == 0xfffe, r == 0xffff:
			return errYAMLChar
		}
		data = data[n:]
	}
	return nil
}

// checkAnchors refuses an anchor defined twice in one document, as
// PyYAML's composer did.
func checkAnchors(n *yaml.Node, seen map[string]bool) error {
	if n == nil || n.Kind == yaml.AliasNode {
		return nil
	}
	if n.Anchor != "" {
		if seen[n.Anchor] {
			return errYAMLAnchor
		}
		seen[n.Anchor] = true
	}
	for _, c := range n.Content {
		if err := checkAnchors(c, seen); err != nil {
			return err
		}
	}
	return nil
}

// yv is a converted value: n null, b bool, d number, s string, a array,
// o object.
type yv struct {
	kind byte
	b    bool
	num  string // a number as jq prints it
	str  string // a string; a number's Python text (its key text)
	id   string // a number's identity as a Python dict key
	date bool   // a timestamp, which json.dumps refused as a key
	arr  []*yv
	keys []string
	vals []*yv
	size int // the length of json.dumps of it
}

// yconv converts the documents of one stream. An alias is the value its
// anchor built, shared as PyYAML's constructor shared it.
type yconv struct {
	memo map[*yaml.Node]*yv
	flat map[*yaml.Node][]*yaml.Node
	work int
}

func (c *yconv) spend(n int) error {
	c.work += n
	if c.work > yamlWorkLimit {
		return errYAMLBudget
	}
	return nil
}

// building marks a node whose value is being built.
var building = &yv{}

func (c *yconv) value(n *yaml.Node) (*yv, error) {
	n = deref(n)
	if n == nil {
		return nil, errYAMLTag
	}
	if n.Kind == yaml.DocumentNode {
		if len(n.Content) == 0 {
			return nullValue(), nil
		}
		return c.value(n.Content[0])
	}
	if v, ok := c.memo[n]; ok {
		if v == building {
			return nil, errYAMLCycle
		}
		return v, nil
	}
	if err := c.spend(1); err != nil {
		return nil, err
	}
	c.memo[n] = building
	v, err := c.build(n)
	if err != nil {
		return nil, err
	}
	c.memo[n] = v
	return v, nil
}

// yamlQuoted is the styles PyYAML resolved as strings without a tag.
const yamlQuoted = yaml.DoubleQuotedStyle | yaml.SingleQuotedStyle | yaml.LiteralStyle | yaml.FoldedStyle

func (c *yconv) build(n *yaml.Node) (*yv, error) {
	// An explicit tag (the decoder reads the non-specific "!" as none).
	tag := ""
	if n.Style&yaml.TaggedStyle != 0 {
		tag = n.ShortTag()
	}
	switch tag {
	case "!!null", "!!bool", "!!int", "!!float", "!!str", "!!timestamp":
		if n.Kind != yaml.ScalarNode {
			return nil, errYAMLTag
		}
		return taggedScalar(tag, n.Value)
	case "!!omap", "!!pairs":
		return c.pairs(n)
	case "!!map":
		if n.Kind != yaml.MappingNode {
			return nil, errYAMLTag
		}
	case "!!seq":
		if n.Kind != yaml.SequenceNode {
			return nil, errYAMLTag
		}
	}
	switch n.Kind {
	case yaml.SequenceNode:
		out := &yv{kind: 'a', arr: make([]*yv, 0, len(n.Content))}
		for _, item := range n.Content {
			v, err := c.value(item)
			if err != nil {
				return nil, err
			}
			out.arr = append(out.arr, v)
		}
		out.size = listSize(out.arr)
		return out, nil
	case yaml.MappingNode:
		return c.mapping(n)
	case yaml.ScalarNode:
		if tag != "" {
			// No constructor for the tag: the scalar's text.
			return stringValue(n.Value), nil
		}
		if n.Style&yamlQuoted != 0 {
			return stringValue(n.Value), nil
		}
		return plainScalar(n.Value)
	}
	return nil, errYAMLTag
}

func listSize(items []*yv) int {
	s := 2
	for i, v := range items {
		if i > 0 {
			s += 2
		}
		s = addSize(s, v.size)
	}
	return s
}

// addSize adds without overflowing.
func addSize(a, b int) int {
	if a > math.MaxInt/4 || b > math.MaxInt/4 {
		return math.MaxInt / 2
	}
	return a + b
}

// pairs is PyYAML's !!omap and !!pairs: a sequence of one-pair mappings
// as a list of [key, value] lists.
func (c *yconv) pairs(n *yaml.Node) (*yv, error) {
	if n.Kind != yaml.SequenceNode {
		return nil, errYAMLTag
	}
	out := &yv{kind: 'a', arr: []*yv{}}
	for _, item := range n.Content {
		item = deref(item)
		if item == nil || item.Kind != yaml.MappingNode || len(item.Content) != 2 {
			return nil, errYAMLTag
		}
		k, err := c.value(item.Content[0])
		if err != nil {
			return nil, err
		}
		v, err := c.value(item.Content[1])
		if err != nil {
			return nil, err
		}
		pair := &yv{kind: 'a', arr: []*yv{k, v}}
		pair.size = listSize(pair.arr)
		out.arr = append(out.arr, pair)
	}
	out.size = listSize(out.arr)
	return out, nil
}

// mapping builds the Python dict of the flattened pairs, then merges the
// keys jq reads as one.
func (c *yconv) mapping(n *yaml.Node) (*yv, error) {
	pairs, err := c.flatten(n, 0)
	if err != nil {
		return nil, err
	}
	type entry struct {
		text string
		val  *yv
	}
	var dict []entry
	at := map[string]int{}
	for i := 0; i+1 < len(pairs); i += 2 {
		k, err := c.value(pairs[i])
		if err != nil {
			return nil, err
		}
		id, text, err := pyKey(k)
		if err != nil {
			return nil, err
		}
		v, err := c.value(pairs[i+1])
		if err != nil {
			return nil, err
		}
		if j, ok := at[id]; ok {
			dict[j].val = v
			continue
		}
		at[id] = len(dict)
		dict = append(dict, entry{text, v})
	}
	out := &yv{kind: 'o'}
	size := 2
	for i, e := range dict {
		if i > 0 {
			size += 2
		}
		size = addSize(size, pyStringSize(e.text)+2)
		size = addSize(size, e.val.size)
		out.set(e.text, e.val)
	}
	out.size = size
	return out, nil
}

// set is jq's reading of an object: a repeated key keeps its first place
// and its last value.
func (o *yv) set(k string, v *yv) {
	for i, have := range o.keys {
		if have == k {
			o.vals[i] = v
			return
		}
	}
	o.keys = append(o.keys, k)
	o.vals = append(o.vals, v)
}

// pyKey is a key's identity in a Python dict and its json.dumps text.
func pyKey(k *yv) (id, text string, err error) {
	switch k.kind {
	case 'n':
		if k.id != "" {
			return k.id, k.str, nil // NaN
		}
		return "z", "null", nil
	case 'b':
		if k.b {
			return "i:1", "true", nil
		}
		return "i:0", "false", nil
	case 'd':
		return k.id, k.str, nil
	case 's':
		if k.date {
			return "", "", errYAMLKey
		}
		return "s:" + k.str, k.str, nil
	}
	return "", "", errYAMLKey
}

func deref(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	return n
}

// isMergeKey is a key resolved as tag:yaml.org,2002:merge, as PyYAML
// resolved it: a plain <<, or an explicit !!merge.
func isMergeKey(k *yaml.Node) bool {
	k = deref(k)
	return k != nil && k.Kind == yaml.ScalarNode && k.ShortTag() == "!!merge"
}

// flatten is PyYAML's flatten_mapping: n's key and value nodes with each
// << replaced by the pairs it merges, all of them ahead of n's own (a
// sequence of mappings merges its last mapping first).
func (c *yconv) flatten(n *yaml.Node, depth int) ([]*yaml.Node, error) {
	if p, ok := c.flat[n]; ok {
		if p == nil {
			return nil, errYAMLCycle
		}
		return p, nil
	}
	if depth > 1000 {
		return nil, errYAMLMerge
	}
	c.flat[n] = nil
	var merge, own []*yaml.Node
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if !isMergeKey(k) {
			own = append(own, k, v)
			continue
		}
		v = deref(v)
		switch {
		case v != nil && v.Kind == yaml.MappingNode:
			p, err := c.flatten(v, depth+1)
			if err != nil {
				return nil, err
			}
			merge = append(merge, p...)
		case v != nil && v.Kind == yaml.SequenceNode:
			var sub [][]*yaml.Node
			for _, item := range v.Content {
				item = deref(item)
				if item == nil || item.Kind != yaml.MappingNode {
					return nil, errYAMLMerge
				}
				p, err := c.flatten(item, depth+1)
				if err != nil {
					return nil, err
				}
				sub = append(sub, p)
			}
			for i := len(sub) - 1; i >= 0; i-- {
				merge = append(merge, sub[i]...)
			}
		default:
			return nil, errYAMLMerge
		}
		if err := c.spend(len(merge)); err != nil {
			return nil, err
		}
	}
	p := append(merge, own...)
	if p == nil {
		p = []*yaml.Node{}
	}
	c.flat[n] = p
	return p, nil
}

var (
	yamlInt10  = regexp.MustCompile(`^[-+]?[0-9]+$`)
	yamlInt8   = regexp.MustCompile(`^0o[0-7]+$`)
	yamlInt16  = regexp.MustCompile(`^0x[0-9a-fA-F]+$`)
	yamlFloatR = regexp.MustCompile(`^[-+]?(\.[0-9]+|[0-9]+(\.[0-9]*)?)([eE][-+]?[0-9]+)?$`)
	yamlInf    = regexp.MustCompile(`^[-+]?\.(inf|Inf|INF)$`)
	yamlNaN    = regexp.MustCompile(`^\.(nan|NaN|NAN)$`)
)

// plainScalar resolves a plain scalar by the YAML 1.2 core schema.
func plainScalar(s string) (*yv, error) {
	switch s {
	case "", "~", "null", "Null", "NULL":
		return nullValue(), nil
	case "true", "True", "TRUE":
		return boolValue(true), nil
	case "false", "False", "FALSE":
		return boolValue(false), nil
	}
	z := new(big.Int)
	ok := false
	switch {
	case yamlInt10.MatchString(s):
		_, ok = z.SetString(strings.TrimPrefix(s, "+"), 10)
	case yamlInt8.MatchString(s):
		_, ok = z.SetString(s[2:], 8)
	case yamlInt16.MatchString(s):
		_, ok = z.SetString(s[2:], 16)
	}
	if ok {
		return intValue(z)
	}
	switch {
	case yamlInf.MatchString(s):
		if s[0] == '-' {
			return floatValue(math.Inf(-1)), nil
		}
		return floatValue(math.Inf(1)), nil
	case yamlNaN.MatchString(s):
		return floatValue(math.NaN()), nil
	case yamlFloatR.MatchString(s):
		f, err := strconv.ParseFloat(s, 64)
		if err != nil && !errors.Is(err, strconv.ErrRange) {
			return nil, err
		}
		return floatValue(f), nil
	}
	return stringValue(s), nil
}

// taggedScalar is a scalar with an explicit core tag, read by PyYAML's
// constructor for it (integers with YAML 1.2's decimal reading).
func taggedScalar(tag, s string) (*yv, error) {
	switch tag {
	case "!!null":
		return nullValue(), nil
	case "!!str":
		return stringValue(s), nil
	case "!!bool":
		switch strings.ToLower(s) {
		case "yes", "true", "on":
			return boolValue(true), nil
		case "no", "false", "off":
			return boolValue(false), nil
		}
		return nil, errYAMLTag
	case "!!int":
		z, ok := pyIntConstruct(s)
		if !ok {
			return nil, errYAMLTag
		}
		return intValue(z)
	case "!!float":
		f, ok := pyFloatConstruct(s)
		if !ok {
			return nil, errYAMLTag
		}
		return floatValue(f), nil
	}
	return timestampValue(s)
}

// pyIntConstruct is PyYAML's construct_yaml_int (underscores dropped, a
// sign, 0b, 0x, 0o, sexagesimal a:b:c, Python's int() for the parts),
// with a leading zero read as decimal (YAML 1.2) rather than octal.
func pyIntConstruct(s string) (*big.Int, bool) {
	v := strings.ReplaceAll(s, "_", "")
	if v == "" {
		return nil, false
	}
	neg := v[0] == '-'
	if v[0] == '-' || v[0] == '+' {
		v = v[1:]
	}
	var z *big.Int
	ok := false
	switch {
	case v == "0":
		return new(big.Int), true
	case strings.HasPrefix(v, "0b"):
		z, ok = pyInt(v[2:], 2)
	case strings.HasPrefix(v, "0x"):
		z, ok = pyInt(v[2:], 16)
	case strings.HasPrefix(v, "0o"):
		z, ok = pyInt(v[2:], 8)
	case strings.Contains(v, ":"):
		z, ok = new(big.Int), true
		base := big.NewInt(1)
		parts := strings.Split(v, ":")
		for i := len(parts) - 1; i >= 0 && ok; i-- {
			var d *big.Int
			if d, ok = pyInt(parts[i], 10); ok {
				z.Add(z, d.Mul(d, base))
				base.Mul(base, big.NewInt(60))
			}
		}
	default:
		z, ok = pyInt(v, 10)
	}
	if !ok {
		return nil, false
	}
	if neg {
		z.Neg(z)
	}
	return z, true
}

// pyInt is Python's int(s, base): blanks around, one sign, the base's
// prefix (other than 10), digits.
func pyInt(s string, base int) (*big.Int, bool) {
	s = strings.TrimSpace(s)
	neg := false
	if s != "" && (s[0] == '-' || s[0] == '+') {
		neg, s = s[0] == '-', s[1:]
	}
	prefix := map[int]string{2: "0b", 8: "0o", 16: "0x"}[base]
	if prefix != "" && len(s) > 2 && strings.EqualFold(s[:2], prefix) {
		s = s[2:]
	}
	if s == "" || s[0] == '-' || s[0] == '+' {
		return nil, false
	}
	z, ok := new(big.Int).SetString(s, base)
	if !ok {
		return nil, false
	}
	if neg {
		z.Neg(z)
	}
	return z, true
}

var pyFloatText = regexp.MustCompile(`^[-+]?([0-9]+\.?[0-9]*|\.[0-9]+)(e[-+]?[0-9]+)?$`)

// pyFloatConstruct is PyYAML's construct_yaml_float with Python's float().
func pyFloatConstruct(s string) (float64, bool) {
	v := strings.ToLower(strings.ReplaceAll(s, "_", ""))
	if v == "" {
		return 0, false
	}
	sign := 1.0
	if v[0] == '-' {
		sign = -1
	}
	if v[0] == '-' || v[0] == '+' {
		v = v[1:]
	}
	switch {
	case v == ".inf":
		return sign * math.Inf(1), true
	case v == ".nan":
		return math.NaN(), true
	case strings.Contains(v, ":"):
		total, base := 0.0, 1.0
		parts := strings.Split(v, ":")
		for i := len(parts) - 1; i >= 0; i-- {
			d, ok := pyFloat(parts[i])
			if !ok {
				return 0, false
			}
			total += d * base
			base *= 60
		}
		return sign * total, true
	}
	f, ok := pyFloat(v)
	return sign * f, ok
}

// pyFloat is Python's float() of a lowercase text.
func pyFloat(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	t := strings.TrimLeft(s, "+-")
	if len(s)-len(t) > 1 {
		return 0, false
	}
	switch t {
	case "inf", "infinity":
		if s[0] == '-' {
			return math.Inf(-1), true
		}
		return math.Inf(1), true
	case "nan":
		return math.NaN(), true
	}
	if !pyFloatText.MatchString(s) {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return 0, false
	}
	return f, true
}

var pyTimestamp = regexp.MustCompile(`^([0-9]{4})-([0-9][0-9]?)-([0-9][0-9]?)` +
	`(?:(?:[Tt]|[ \t]+)([0-9][0-9]?):([0-9][0-9]):([0-9][0-9])(?:\.([0-9]*))?` +
	`(?:[ \t]*(Z|([-+])([0-9][0-9]?)(?::([0-9][0-9]))?))?)?\n?$`)

// timestampValue is PyYAML's construct_yaml_timestamp as python-yq's
// encoder wrote it (isoformat()).
func timestampValue(s string) (*yv, error) {
	m := pyTimestamp.FindStringSubmatch(s)
	if m == nil {
		return nil, errYAMLTag
	}
	num := func(t string) int { n, _ := strconv.Atoi(t); return n }
	year, month, day := num(m[1]), num(m[2]), num(m[3])
	if year < 1 || month < 1 || month > 12 || day < 1 ||
		day > time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day() {
		return nil, errYAMLTag
	}
	out := strings.Repeat("0", 4-len(strconv.Itoa(year))) + strconv.Itoa(year) + "-" + two(month) + "-" + two(day)
	if m[4] != "" {
		hour, minute, second := num(m[4]), num(m[5]), num(m[6])
		if hour > 23 || minute > 59 || second > 59 {
			return nil, errYAMLTag
		}
		out += "T" + two(hour) + ":" + two(minute) + ":" + two(second)
		if f := m[7]; f != "" {
			if len(f) > 6 {
				f = f[:6]
			}
			if f += strings.Repeat("0", 6-len(f)); f != "000000" {
				out += "." + f
			}
		}
		switch {
		case m[9] != "":
			off := num(m[10])*60 + num(m[11])
			if off >= 24*60 {
				return nil, errYAMLTag
			}
			sign := "+"
			if m[9] == "-" && off != 0 {
				sign = "-"
			}
			out += sign + two(off/60) + ":" + two(off%60)
		case m[8] == "Z":
			out += "+00:00"
		}
	}
	v := stringValue(out)
	v.date = true
	return v, nil
}

func two(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

func nullValue() *yv { return &yv{kind: 'n', size: 4} }

func boolValue(b bool) *yv {
	if b {
		return &yv{kind: 'b', b: true, size: 4}
	}
	return &yv{kind: 'b', size: 5}
}

func stringValue(s string) *yv {
	return &yv{kind: 's', str: s, size: pyStringSize(s)}
}

// pyStringSize is the length of json.dumps(s) with ensure_ascii: `"`, `\`
// and the short escapes take two characters, the rest of what is not
// printable ASCII six (twelve as a surrogate pair).
func pyStringSize(s string) int {
	n := 2
	for _, r := range s {
		switch {
		case r == '"' || r == '\\' || r == '\n' || r == '\r' || r == '\t' || r == '\b' || r == '\f':
			n += 2
		case r >= 0x20 && r <= 0x7e:
			n++
		case r > 0xffff:
			n += 12
		default:
			n += 6
		}
	}
	return n
}

// intValue is a Python int; jq prints its decimal text as written.
func intValue(z *big.Int) (*yv, error) {
	t := z.String()
	if len(strings.TrimPrefix(t, "-")) > pyMaxIntDigits {
		return nil, errYAMLBigInt
	}
	return &yv{kind: 'd', num: t, str: t, id: "i:" + t, size: len(t)}, nil
}

// floatValue is a Python float as jq printed json.dumps of it: num is
// jq's text (NaN null, an infinity jq's largest double), str Python's
// text (a key's text), id its identity as a dict key (an integral float
// is the int it equals).
func floatValue(f float64) *yv {
	switch {
	case math.IsNaN(f):
		// PyYAML gave every .nan one object: as keys they are one key.
		return &yv{kind: 'n', str: "NaN", id: "f:nan", size: 3}
	case math.IsInf(f, 1):
		return &yv{kind: 'd', num: "1.7976931348623157e+308", str: "Infinity", id: "f:inf", size: 8}
	case math.IsInf(f, -1):
		return &yv{kind: 'd', num: "-1.7976931348623157e+308", str: "-Infinity", id: "f:-inf", size: 9}
	}
	r := pyRepr(f)
	id := "f:" + r
	if f == math.Trunc(f) {
		z, _ := new(big.Float).SetFloat64(f).Int(nil)
		id = "i:" + z.String()
	}
	return &yv{kind: 'd', num: jqNumber(r), str: r, id: id, size: len(r)}
}

// pyRepr is Python's repr of a float: the shortest digits, fixed notation
// (with .0 when integral) from 1e-4 up to 1e16, else d.ddde±XX.
func pyRepr(f float64) string {
	sign := ""
	if math.Signbit(f) {
		sign, f = "-", -f
	}
	if f == 0 {
		return sign + "0.0"
	}
	mant, exp, _ := strings.Cut(strconv.FormatFloat(f, 'e', -1, 64), "e")
	digits := strings.Replace(mant, ".", "", 1)
	x, _ := strconv.Atoi(exp)
	decpt := x + 1
	n := len(digits)
	if decpt > -4 && decpt <= 16 {
		switch {
		case decpt <= 0:
			return sign + "0." + strings.Repeat("0", -decpt) + digits
		case decpt >= n:
			return sign + digits + strings.Repeat("0", decpt-n) + ".0"
		default:
			return sign + digits[:decpt] + "." + digits[decpt:]
		}
	}
	out := sign + digits[:1]
	if n > 1 {
		out += "." + digits[1:]
	}
	e := decpt - 1
	es := "+"
	if e < 0 {
		es, e = "-", -e
	}
	if e < 10 {
		es += "0"
	}
	return out + "e" + es + strconv.Itoa(e)
}

// jqNumber is the text jq 1.7 prints for a number literal it read:
// decNumber's to-scientific-string (the same rule as internal/db's).
func jqNumber(lit string) string {
	s := lit
	neg := false
	if s != "" && (s[0] == '-' || s[0] == '+') {
		neg = s[0] == '-'
		s = s[1:]
	}
	mant, expPart := s, ""
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		mant, expPart = s[:i], s[i+1:]
	}
	intPart, frac := mant, ""
	if i := strings.IndexByte(mant, '.'); i >= 0 {
		intPart, frac = mant[:i], mant[i+1:]
	}
	exp := 0
	if expPart != "" {
		e, err := strconv.Atoi(strings.TrimPrefix(expPart, "+"))
		if err != nil {
			return lit
		}
		exp = e
	}
	coef := strings.TrimLeft(intPart+frac, "0")
	if coef == "" {
		coef = "0"
	}
	exp -= len(frac)
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	adj := exp + len(coef) - 1
	if exp <= 0 && adj >= -6 {
		switch {
		case exp == 0:
			b.WriteString(coef)
		case len(coef) > -exp:
			b.WriteString(coef[:len(coef)+exp] + "." + coef[len(coef)+exp:])
		default:
			b.WriteString("0." + strings.Repeat("0", -exp-len(coef)) + coef)
		}
		return b.String()
	}
	b.WriteByte(coef[0])
	if len(coef) > 1 {
		b.WriteString("." + coef[1:])
	}
	b.WriteByte('E')
	if adj < 0 {
		b.WriteByte('-')
		adj = -adj
	} else {
		b.WriteByte('+')
	}
	b.WriteString(strconv.Itoa(adj))
	return b.String()
}

// writeYAMLJSON writes the documents one after another as `jq .` does.
func writeYAMLJSON(w io.Writer, docs []*yv) error {
	jw := &jqWriter{w: w}
	for _, d := range docs {
		jw.value(d, "")
		jw.b = append(jw.b, '\n')
		jw.flush(false)
	}
	jw.flush(true)
	return jw.err
}

// jqWriter writes in chunks: an alias-heavy document prints far larger
// than it is held.
type jqWriter struct {
	w   io.Writer
	b   []byte
	err error
}

func (jw *jqWriter) flush(all bool) {
	if jw.err != nil || (!all && len(jw.b) < 1<<16) {
		return
	}
	_, jw.err = jw.w.Write(jw.b)
	jw.b = jw.b[:0]
}

// value writes v as `jq .` does: two-space indent, ": " after a key, []
// and {} when empty.
func (jw *jqWriter) value(v *yv, indent string) {
	if jw.err != nil {
		return
	}
	inner := indent + "  "
	switch v.kind {
	case 'n':
		jw.b = append(jw.b, "null"...)
	case 'b':
		jw.b = strconv.AppendBool(jw.b, v.b)
	case 'd':
		jw.b = append(jw.b, v.num...)
	case 's':
		jw.b = contract.AppendString(jw.b, v.str, contract.EscapeJQ)
	case 'a':
		if len(v.arr) == 0 {
			jw.b = append(jw.b, "[]"...)
			return
		}
		jw.b = append(jw.b, "[\n"...)
		for i, item := range v.arr {
			if i > 0 {
				jw.b = append(jw.b, ",\n"...)
			}
			jw.b = append(jw.b, inner...)
			jw.value(item, inner)
			jw.flush(false)
		}
		jw.b = append(append(append(jw.b, '\n'), indent...), ']')
	default:
		if len(v.keys) == 0 {
			jw.b = append(jw.b, "{}"...)
			return
		}
		jw.b = append(jw.b, "{\n"...)
		for i, k := range v.keys {
			if i > 0 {
				jw.b = append(jw.b, ",\n"...)
			}
			jw.b = contract.AppendString(append(jw.b, inner...), k, contract.EscapeJQ)
			jw.b = append(jw.b, ": "...)
			jw.value(v.vals[i], inner)
			jw.flush(false)
		}
		jw.b = append(append(append(jw.b, '\n'), indent...), '}')
	}
}
