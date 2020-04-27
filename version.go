package version

const (
	ApkVersionUnknown  = 0
	ApkVersionEqual    = 1
	ApkVersionLess     = 2
	ApkVersionGreater  = 4
	ApkVersionFuzzy    = 8
	ApkDepmaskAny      = (ApkVersionEqual | ApkVersionLess | ApkVersionGreater | ApkVersionFuzzy)
	ApkDepmaskChecksum = (ApkVersionLess | ApkVersionGreater)
)

func isDigit(c byte) bool {
	return '0' <= c && c <= '9'
}

func isLower(c byte) bool {
	return 'a' <= c && c <= 'z'
}

func apkVersionOpString(mask int) string {
	switch mask {
	case ApkVersionLess:
		return "<"
	case ApkVersionLess | ApkVersionEqual:
		return "<="
	case ApkVersionEqual | ApkVersionFuzzy:
		fallthrough
	case ApkVersionFuzzy:
		return "~"
	case ApkVersionEqual:
		return "="
	case ApkVersionGreater | ApkVersionEqual:
		return ">="
	case ApkVersionGreater:
		return ">"
	case ApkDepmaskChecksum:
		return "><"
	default:
		return "?"
	}
}

type tokenType int

const (
	tokenInvalid tokenType = iota - 1
	tokenDigitOrZero
	tokenDigit
	tokenLetter
	tokenSuffix
	tokenSuffixNo
	tokenRevisionNo
	tokenEnd
)

func (t tokenType) String() string {
	vals := []string{
		"tokenInvalid",
		"tokenDigitOrZero",
		"tokenDigit",
		"tokenLetter",
		"tokenSuffix",
		"tokenSuffix_NO",
		"tokenRevisionNo",
		"tokenEnd"}

	return vals[int(t)+1]
}

// ApkVersion type
type ApkVersion struct {
	version string
	pos     int
}

func (v *ApkVersion) left() int {
	return len(v.version) - v.pos
}

func (v *ApkVersion) skip(n int) {
	v.pos += n
	if v.pos > len(v.version) {
		panic("skipped too far")
	}
}

func (v *ApkVersion) next() byte {
	v.pos++
	return v.peekAt(-1)
}

func (v *ApkVersion) peek() byte {
	return v.peekAt(0)
}

func (v *ApkVersion) peekCount(count int) string {
	return v.version[v.pos : v.pos+count]
}

func (v *ApkVersion) peekAt(offset int) byte {
	return v.version[v.pos+offset]
}

func (v *ApkVersion) isEOF() bool {
	return v.pos == len(v.version)
}

func (v *ApkVersion) nextToken(cur tokenType) tokenType {
	ret := tokenInvalid

	if v.isEOF() {
		ret = tokenEnd
	} else if (cur == tokenDigit || cur == tokenDigitOrZero) && isLower(v.peek()) {
		ret = tokenLetter
	} else if cur == tokenLetter && isDigit(v.peek()) {
		ret = tokenDigit
	} else if cur == tokenSuffix && isDigit(v.peek()) {
		ret = tokenSuffixNo
	} else {
		switch v.next() {
		case '.':
			ret = tokenDigitOrZero
		case '_':
			ret = tokenSuffix
		case '-':
			ret = tokenInvalid
			if v.left() > 0 && v.peek() == 'r' {
				ret = tokenRevisionNo
				v.next()
			}
		}
	}

	if ret < cur {
		if !((ret == tokenDigitOrZero && cur == tokenDigit) ||
			(ret == tokenSuffix && cur == tokenSuffixNo) ||
			(ret == tokenDigit && cur == tokenLetter)) {
			ret = tokenInvalid
		}
	}

	return ret
}

var preSuffixes = []string{"alpha", "beta", "pre", "rc"}
var postSuffixes = []string{"cvs", "svn", "git", "hg", "p"}

func (v *ApkVersion) getToken(cur tokenType) (tokenType, int) {
	if v.isEOF() {
		return tokenEnd, 0
	}

	nt := tokenInvalid
	i := 0
	val := 0

	switch cur {
	case tokenDigitOrZero:
		/* Leading zero digits get a special treatment */
		if v.peek() == '0' {
			for i = 0; i < v.left() && v.peekAt(i) == '0'; i++ {
			}
			nt = tokenDigit
			val = -i
			break
		}
		fallthrough
	case tokenDigit:
		fallthrough
	case tokenSuffixNo:
		fallthrough
	case tokenRevisionNo:
		for i = 0; i < v.left() && isDigit(v.peekAt(i)); i++ {
			val *= 10
			val += int(v.peekAt(i) - '0')
		}
	case tokenLetter:
		val = int(v.peek())
		i++
	case tokenSuffix:
		for val = 0; val < len(preSuffixes); val++ {
			i = len(preSuffixes[val])
			if len(preSuffixes[val]) <= v.left() && v.peekCount(i) == preSuffixes[val] {
				break
			}
		}
		if val < len(preSuffixes) {
			val = val - len(preSuffixes)
			break
		}
		for val = 0; val < len(postSuffixes); val++ {
			i = len(postSuffixes[val])
			if len(postSuffixes[val]) <= v.left() && v.peekCount(i) == postSuffixes[val] {
				break
			}
		}
		if val < len(postSuffixes) {
			break
		}

		fallthrough
	default:
		nt = tokenInvalid
		return nt, -1
	}

	v.skip(i)
	if v.isEOF() {
		nt = tokenEnd
	} else if nt == tokenInvalid {
		nt = v.nextToken(cur)
	}
	return nt, val
}

// NewVersion Create a new version from string
func NewVersion(version string) *ApkVersion {
	return &ApkVersion{
		version: version,
		pos:     0,
	}
}

// CompareVersions compares 2 version
func CompareVersions(a, b *ApkVersion) int {
	at := tokenDigit
	bt := tokenDigit
	var av, bv int

	if a == nil || b == nil {
		if a == nil && b == nil {
			return ApkVersionEqual
		}
		return ApkVersionEqual | ApkVersionGreater | ApkVersionLess
	}

	for at == bt && at != tokenEnd && at != tokenInvalid && av == bv {
		at, av = a.getToken(at)
		bt, bv = b.getToken(bt)
	}

	/* value of this token differs? */
	if av < bv {
		return ApkVersionLess
	} else if av > bv {
		return ApkVersionGreater
	}

	/* both have tokenEnd or tokenInvalid next? */
	if at == bt {
		return ApkVersionEqual
	}

	/* leading version components and their values are equal,
	 * now the non-terminating version is greater unless it's a suffix
	 * indicating pre-release */
	if at == tokenSuffix {
		if _, v := a.getToken(at); v < 0 {
			return ApkVersionLess
		}
	}
	if bt == tokenSuffix {
		if _, v := b.getToken(bt); v < 0 {
			return ApkVersionGreater
		}
	}
	if at > bt {
		return ApkVersionLess
	} else if bt > at {
		return ApkVersionGreater
	}

	return ApkVersionEqual
}

// AlpineVersionLessThan return true if vera < verb
func AlpineVersionLessThan(vera *ApkVersion, verb *ApkVersion) bool {
	return CompareVersions(vera, verb) == ApkVersionLess
}
