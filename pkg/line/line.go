package line

import (
	"regexp"
)

type LineProcessor interface {
	Match(line string) bool
	Denoise(line string) string
	Extract(line string) []string
}

type noiseReplacement struct {
	pattern *regexp.Regexp
	replacement string
}

func NewLineProcessor(matchPatterns []string, denoisePatterns [][]string, extractPatterns []string) (LineProcessor, error) {
	lp := &lineProcessor{
		matchPatterns:   make([]*regexp.Regexp, len(matchPatterns)),
		denoisePatterns: make([]*noiseReplacement, len(denoisePatterns)),
		extractPatterns: make([]*regexp.Regexp, len(extractPatterns)),
	}
	for i, p := range matchPatterns {
		r, e := regexp.Compile(p)
		if e != nil {
			return nil, e
		}
		lp.matchPatterns[i] = r
	}
	for i, a := range denoisePatterns {
		r, e := regexp.Compile(a[0])
		if e != nil {
			return nil, e
		}
		lp.denoisePatterns[i] = &noiseReplacement {
			pattern: r,
			replacement: a[1],
		}
	}
	for i, p := range extractPatterns {
		r, e := regexp.Compile(p)
		if e != nil {
			return nil, e
		}
		lp.extractPatterns[i] = r
	}
	return lp, nil
}

type lineProcessor struct {
	matchPatterns   []*regexp.Regexp
	denoisePatterns []*noiseReplacement
	extractPatterns []*regexp.Regexp
}

func (lp *lineProcessor) Match(line string) bool {
	for _, r := range lp.matchPatterns {
		if r.MatchString(line) {
			return true
		}
	}
	return false
}

func (lp *lineProcessor) Denoise(line string) string {
	for _, r := range lp.denoisePatterns {
		line = r.pattern.ReplaceAllString(line, r.replacement)
	}
	return line
}

func (lp *lineProcessor) Extract(line string) []string {
	matches := []string{}
	for _, r := range lp.extractPatterns {
		m := r.FindAllString(line, -1)
		if m != nil {
			matches = append(matches, m...)
		}
	}
	return matches
}
