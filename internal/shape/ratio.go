package shape

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Ratio is a width:height aspect ratio such as 5:4.
type Ratio struct{ W, H float64 }

// Aspect returns W/H.
func (r Ratio) Aspect() float64 { return r.W / r.H }

// Valid reports whether the ratio is usable.
func (r Ratio) Valid() bool {
	if !(r.W > 0 && r.H > 0) || math.IsInf(r.W, 0) || math.IsInf(r.H, 0) {
		return false
	}
	a := r.Aspect()
	return a >= 1.0/MaxAspect && a <= MaxAspect
}

func (r Ratio) String() string {
	return trimFloat(r.W) + ":" + trimFloat(r.H)
}

// MaxAspect bounds how extreme an aspect ratio may be.
const MaxAspect = 50

func trimFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// Presets are the aspect ratios offered in the UI.
var Presets = []Ratio{
	{1, 1}, {5, 4}, {4, 3}, {3, 2}, {16, 9}, {2, 1},
	{4, 5}, {3, 4}, {2, 3}, {9, 16},
}

var errRatio = errors.New(`use a ratio like "5:4", "16/9" or "1.5"`)

// ParseRatio parses "5:4", "5/4", "5x4" or a single number such as "1.5"
// (a decimal comma is accepted too).
func ParseRatio(s string) (Ratio, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return Ratio{}, errRatio
	}
	var r Ratio
	if i := strings.IndexAny(s, ":/x×"); i >= 0 {
		sep := s[i:]
		_, size := firstRune(sep)
		w, err1 := parseNum(s[:i])
		h, err2 := parseNum(s[i+size:])
		if err1 != nil || err2 != nil {
			return Ratio{}, errRatio
		}
		r = Ratio{w, h}
	} else {
		v, err := parseNum(s)
		if err != nil {
			return Ratio{}, errRatio
		}
		r = Ratio{v, 1}
	}
	if !r.Valid() {
		return Ratio{}, fmt.Errorf("ratio must be positive and at most %d:1", MaxAspect)
	}
	return r, nil
}

func firstRune(s string) (rune, int) {
	for _, r := range s {
		return r, len(string(r))
	}
	return 0, 0
}

func parseNum(s string) (float64, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) {
		return 0, errRatio
	}
	return v, nil
}
