package palette

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Eval evaluates a simple integer expression: + - * / % ** & | ^ << >> ( ) and 0x hex.
func Eval(expr string) (string, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "", fmt.Errorf("empty expression")
	}
	p := &parser{s: expr}
	p.skip()
	v, err := p.parseExpr()
	if err != nil {
		return "", err
	}
	p.skip()
	if p.pos < len(p.s) {
		return "", fmt.Errorf("unexpected %q", p.s[p.pos:])
	}
	return FormatInt(v), nil
}

type parser struct {
	s   string
	pos int
}

func (p *parser) skip() {
	for p.pos < len(p.s) && unicode.IsSpace(rune(p.s[p.pos])) {
		p.pos++
	}
}

func (p *parser) peek() byte {
	if p.pos >= len(p.s) {
		return 0
	}
	return p.s[p.pos]
}

func (p *parser) parseExpr() (int64, error) {
	return p.parseOr()
}

func (p *parser) parseOr() (int64, error) {
	v, err := p.parseXor()
	if err != nil {
		return 0, err
	}
	for {
		p.skip()
		if p.peek() != '|' || (p.pos+1 < len(p.s) && p.s[p.pos+1] == '|') {
			break
		}
		p.pos++
		r, err := p.parseXor()
		if err != nil {
			return 0, err
		}
		v |= r
	}
	return v, nil
}

func (p *parser) parseXor() (int64, error) {
	v, err := p.parseAnd()
	if err != nil {
		return 0, err
	}
	for {
		p.skip()
		if p.peek() != '^' {
			break
		}
		p.pos++
		r, err := p.parseAnd()
		if err != nil {
			return 0, err
		}
		v ^= r
	}
	return v, nil
}

func (p *parser) parseAnd() (int64, error) {
	v, err := p.parseShift()
	if err != nil {
		return 0, err
	}
	for {
		p.skip()
		if p.peek() != '&' || (p.pos+1 < len(p.s) && p.s[p.pos+1] == '&') {
			break
		}
		p.pos++
		r, err := p.parseShift()
		if err != nil {
			return 0, err
		}
		v &= r
	}
	return v, nil
}

func (p *parser) parseShift() (int64, error) {
	v, err := p.parseAdd()
	if err != nil {
		return 0, err
	}
	for {
		p.skip()
		if strings.HasPrefix(p.s[p.pos:], "<<") {
			p.pos += 2
			r, err := p.parseAdd()
			if err != nil {
				return 0, err
			}
			v <<= uint(r)
			continue
		}
		if strings.HasPrefix(p.s[p.pos:], ">>") {
			p.pos += 2
			r, err := p.parseAdd()
			if err != nil {
				return 0, err
			}
			v >>= uint(r)
			continue
		}
		break
	}
	return v, nil
}

func (p *parser) parseAdd() (int64, error) {
	v, err := p.parseMul()
	if err != nil {
		return 0, err
	}
	for {
		p.skip()
		op := p.peek()
		if op != '+' && op != '-' {
			break
		}
		p.pos++
		r, err := p.parseMul()
		if err != nil {
			return 0, err
		}
		if op == '+' {
			v += r
		} else {
			v -= r
		}
	}
	return v, nil
}

func (p *parser) parseMul() (int64, error) {
	v, err := p.parsePow()
	if err != nil {
		return 0, err
	}
	for {
		p.skip()
		op := p.peek()
		if op != '*' && op != '/' && op != '%' {
			break
		}
		if op == '*' && p.pos+1 < len(p.s) && p.s[p.pos+1] == '*' {
			break // ** handled in pow
		}
		p.pos++
		r, err := p.parsePow()
		if err != nil {
			return 0, err
		}
		switch op {
		case '*':
			v *= r
		case '/':
			if r == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			v /= r
		case '%':
			if r == 0 {
				return 0, fmt.Errorf("mod by zero")
			}
			v %= r
		}
	}
	return v, nil
}

func (p *parser) parsePow() (int64, error) {
	v, err := p.parseUnary()
	if err != nil {
		return 0, err
	}
	p.skip()
	if strings.HasPrefix(p.s[p.pos:], "**") {
		p.pos += 2
		exp, err := p.parsePow() // right-assoc
		if err != nil {
			return 0, err
		}
		if exp < 0 {
			return 0, fmt.Errorf("negative exponent")
		}
		return ipow(v, exp), nil
	}
	return v, nil
}

func (p *parser) parseUnary() (int64, error) {
	p.skip()
	if p.peek() == '+' {
		p.pos++
		return p.parseUnary()
	}
	if p.peek() == '-' {
		p.pos++
		v, err := p.parseUnary()
		return -v, err
	}
	if p.peek() == '~' {
		p.pos++
		v, err := p.parseUnary()
		return ^v, err
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (int64, error) {
	p.skip()
	if p.peek() == '(' {
		p.pos++
		v, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		p.skip()
		if p.peek() != ')' {
			return 0, fmt.Errorf("missing )")
		}
		p.pos++
		return v, nil
	}
	start := p.pos
	if strings.HasPrefix(strings.ToLower(p.s[p.pos:]), "0x") {
		p.pos += 2
		for p.pos < len(p.s) && isHex(p.s[p.pos]) {
			p.pos++
		}
		n, err := strconv.ParseInt(p.s[start:p.pos], 0, 64)
		if err != nil {
			return 0, fmt.Errorf("bad hex")
		}
		return n, nil
	}
	if p.pos < len(p.s) && unicode.IsDigit(rune(p.s[p.pos])) {
		for p.pos < len(p.s) && unicode.IsDigit(rune(p.s[p.pos])) {
			p.pos++
		}
		n, err := strconv.ParseInt(p.s[start:p.pos], 10, 64)
		if err != nil {
			return 0, err
		}
		return n, nil
	}
	return 0, fmt.Errorf("expected number")
}

func isHex(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

func ipow(base, exp int64) int64 {
	var r int64 = 1
	for exp > 0 {
		if exp&1 == 1 {
			r *= base
		}
		base *= base
		exp >>= 1
	}
	return r
}
