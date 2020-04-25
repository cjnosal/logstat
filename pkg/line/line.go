package line

import (
	"regexp"
)

type LineProcessor interface {
	Match(line string) bool
	Denoise(line string) string
	Extract(line string) []string
}

func NewLineProcessor(matchPatterns []string, denoisePatterns []string, extractPatterns []string) (LineProcessor, error) {
	lp := &lineProcessor{
		matchPatterns:   []*regexp.Regexp{},
		denoisePatterns: []*regexp.Regexp{},
		extractPatterns: []*regexp.Regexp{},
	}
	for _, i := range matchPatterns {
		r, e := regexp.Compile(i)
		if e != nil {
			return nil, e
		}
		lp.matchPatterns = append(lp.matchPatterns, r)
	}
	for _, i := range denoisePatterns {
		r, e := regexp.Compile(i)
		if e != nil {
			return nil, e
		}
		lp.denoisePatterns = append(lp.denoisePatterns, r)
	}
	for _, i := range extractPatterns {
		r, e := regexp.Compile(i)
		if e != nil {
			return nil, e
		}
		lp.extractPatterns = append(lp.extractPatterns, r)
	}
	return lp, nil
}

type lineProcessor struct {
	matchPatterns   []*regexp.Regexp
	denoisePatterns []*regexp.Regexp
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
		line = r.ReplaceAllString(line, "{noise}")
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
