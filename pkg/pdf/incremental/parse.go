package incremental

import (
	"strconv"

	"github.com/pkg/errors"
)

// errShort means the parser ran off the end of its buffer before the end
// of the data, so the caller should retry with more bytes.
var errShort = errors.New("need more data")

// maxDepth bounds nested arrays and dictionaries, so a hostile file cannot
// exhaust the stack.
const maxDepth = 128

// keyword is a bare token that is not a number, a boolean, or null, such
// as obj, R, stream, xref, or trailer.
type keyword string

// parser reads PDF syntax from buf. final means buf runs to the end of the
// data; otherwise running off its end is errShort.
type parser struct {
	buf   []byte
	pos   int
	final bool
}

func isWhitespace(c byte) bool {
	switch c {
	case 0, '\t', '\n', '\f', '\r', ' ':
		return true
	}
	return false
}

func isDelimiter(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

func (p *parser) short() error {
	if p.final {
		return errors.New("unexpected end of PDF data")
	}
	return errShort
}

// skipSpace skips whitespace and comments.
func (p *parser) skipSpace() {
	for p.pos < len(p.buf) {
		c := p.buf[p.pos]
		switch {
		case isWhitespace(c):
			p.pos++
		case c == '%':
			for p.pos < len(p.buf) && p.buf[p.pos] != '\n' && p.buf[p.pos] != '\r' {
				p.pos++
			}
		default:
			return
		}
	}
}

// regular reads a run of regular characters: a number or a keyword.
func (p *parser) regular() (string, error) {
	start := p.pos
	for p.pos < len(p.buf) && !isWhitespace(p.buf[p.pos]) && !isDelimiter(p.buf[p.pos]) {
		p.pos++
	}
	if p.pos == len(p.buf) && !p.final {
		return "", errShort
	}
	return string(p.buf[start:p.pos]), nil
}

// object reads the next object. Keywords come back as keyword values, so
// callers that parse indirect objects or cross-reference tables can see
// them; inside arrays and dictionaries they are a syntax error.
func (p *parser) object() (Object, error) {
	return p.objectDepth(0)
}

func (p *parser) objectDepth(depth int) (Object, error) {
	if depth > maxDepth {
		return nil, errors.New("PDF objects nested too deeply")
	}
	p.skipSpace()
	if p.pos >= len(p.buf) {
		return nil, p.short()
	}
	switch c := p.buf[p.pos]; c {
	case '/':
		return p.name()
	case '(':
		return p.literalString()
	case '<':
		if p.pos+1 >= len(p.buf) {
			return nil, p.short()
		}
		if p.buf[p.pos+1] == '<' {
			return p.dict(depth)
		}
		return p.hexString()
	case '[':
		return p.array(depth)
	case ')', '>', ']', '{', '}':
		return nil, errors.Errorf("unexpected %q at offset %d", c, p.pos)
	}

	tok, err := p.regular()
	if err != nil {
		return nil, err
	}
	switch tok {
	case "true":
		return Bool(true), nil
	case "false":
		return Bool(false), nil
	case "null":
		return Null{}, nil
	}
	if n, ok := parseInt(tok); ok {
		return p.maybeRef(n)
	}
	if isReal(tok) {
		return Real(tok), nil
	}
	return keyword(tok), nil
}

// maybeRef reads "gen R" after an integer when it is there, making a
// reference, and otherwise leaves the integer alone.
func (p *parser) maybeRef(num int64) (Object, error) {
	save := p.pos
	p.skipSpace()
	gen, err := p.regular()
	if err != nil {
		return nil, err
	}
	g, ok := parseInt(gen)
	if ok {
		p.skipSpace()
		r, err := p.regular()
		if err != nil {
			return nil, err
		}
		if r == "R" {
			return Ref{Num: int(num), Gen: int(g)}, nil
		}
	}
	p.pos = save
	return Integer(num), nil
}

func parseInt(tok string) (int64, bool) {
	if tok == "" {
		return 0, false
	}
	start := 0
	if tok[0] == '+' || tok[0] == '-' {
		start = 1
	}
	if start == len(tok) {
		return 0, false
	}
	for i := start; i < len(tok); i++ {
		if tok[i] < '0' || tok[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(tok, 10, 64)
	return n, err == nil
}

func isReal(tok string) bool {
	digits, dots := 0, 0
	for i := 0; i < len(tok); i++ {
		c := tok[i]
		switch {
		case c >= '0' && c <= '9':
			digits++
		case c == '.':
			dots++
		case (c == '+' || c == '-') && i == 0:
		default:
			return false
		}
	}
	return digits > 0 && dots == 1
}

func (p *parser) name() (Object, error) {
	p.pos++ // '/'
	var out []byte
	for p.pos < len(p.buf) && !isWhitespace(p.buf[p.pos]) && !isDelimiter(p.buf[p.pos]) {
		c := p.buf[p.pos]
		if c == '#' && p.pos+2 < len(p.buf) {
			if v, err := strconv.ParseUint(string(p.buf[p.pos+1:p.pos+3]), 16, 8); err == nil {
				out = append(out, byte(v))
				p.pos += 3
				continue
			}
		}
		out = append(out, c)
		p.pos++
	}
	if p.pos == len(p.buf) && !p.final {
		return nil, errShort
	}
	return Name(out), nil
}

func (p *parser) literalString() (Object, error) {
	p.pos++ // '('
	var out []byte
	depth := 1
	for {
		if p.pos >= len(p.buf) {
			return nil, p.short()
		}
		c := p.buf[p.pos]
		p.pos++
		switch c {
		case '(':
			depth++
			out = append(out, c)
		case ')':
			depth--
			if depth == 0 {
				return String(out), nil
			}
			out = append(out, c)
		case '\\':
			if p.pos >= len(p.buf) {
				return nil, p.short()
			}
			e := p.buf[p.pos]
			p.pos++
			switch e {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case '\r':
				// A backslash before an end of line continues the string.
				if p.pos < len(p.buf) && p.buf[p.pos] == '\n' {
					p.pos++
				}
			case '\n':
			default:
				if e >= '0' && e <= '7' {
					v := int(e - '0')
					for i := 0; i < 2 && p.pos < len(p.buf) && p.buf[p.pos] >= '0' && p.buf[p.pos] <= '7'; i++ {
						v = v*8 + int(p.buf[p.pos]-'0')
						p.pos++
					}
					out = append(out, byte(v))
				} else {
					out = append(out, e)
				}
			}
		default:
			out = append(out, c)
		}
	}
}

func (p *parser) hexString() (Object, error) {
	p.pos++ // '<'
	var out []byte
	var hi byte
	half := false
	for {
		if p.pos >= len(p.buf) {
			return nil, p.short()
		}
		c := p.buf[p.pos]
		p.pos++
		if c == '>' {
			if half {
				out = append(out, hi<<4)
			}
			return String(out), nil
		}
		if isWhitespace(c) {
			continue
		}
		v, ok := hexValue(c)
		if !ok {
			return nil, errors.Errorf("invalid hex string character %q", c)
		}
		if half {
			out = append(out, hi<<4|v)
		} else {
			hi = v
		}
		half = !half
	}
}

func hexValue(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

func (p *parser) array(depth int) (Object, error) {
	p.pos++ // '['
	arr := Array{}
	for {
		p.skipSpace()
		if p.pos >= len(p.buf) {
			return nil, p.short()
		}
		if p.buf[p.pos] == ']' {
			p.pos++
			return arr, nil
		}
		o, err := p.objectDepth(depth + 1)
		if err != nil {
			return nil, err
		}
		if k, ok := o.(keyword); ok {
			return nil, errors.Errorf("unexpected keyword %q in array", string(k))
		}
		arr = append(arr, o)
	}
}

func (p *parser) dict(depth int) (Object, error) {
	p.pos += 2 // '<<'
	d := Dict{}
	for {
		p.skipSpace()
		if p.pos+1 >= len(p.buf) {
			return nil, p.short()
		}
		if p.buf[p.pos] == '>' && p.buf[p.pos+1] == '>' {
			p.pos += 2
			return d, nil
		}
		if p.buf[p.pos] != '/' {
			return nil, errors.Errorf("dictionary key at offset %d is not a name", p.pos)
		}
		key, err := p.name()
		if err != nil {
			return nil, err
		}
		v, err := p.objectDepth(depth + 1)
		if err != nil {
			return nil, err
		}
		if k, ok := v.(keyword); ok {
			return nil, errors.Errorf("unexpected keyword %q in dictionary", string(k))
		}
		if _, isNull := v.(Null); !isNull {
			d[key.(Name)] = v
		}
	}
}
