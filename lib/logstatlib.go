package lib

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"
	"math"
	"sort"

	"github.com/cjnosal/logstat/pkg/line"
)

type LogStat interface {
	ProcessFiles(logFiles []string, config Config) (*Result, error)
	ProcessStream(reader io.Reader, config Config) (*Result, error)
	Histogram(config Config, result *Result, out io.Writer) error
}

func NewLogStat(logger *log.Logger) LogStat {
	return &logStat{
		logger: logger,
	}
}

type Config struct {
	LineFilters        []string
	DateTimeExtractors []string
	DateTimeFormats    []string
	DenoisePatterns    []string
	NoiseReplacement   string
	BucketDuration time.Duration
}

type Result struct {
	TotalLines int
	ReferenceTime *time.Time
	Buckets    map[time.Time]Bucket
}

type Bucket struct {
	LineCount map[string]int
}

type logStat struct {
	logger *log.Logger
}

func (l *logStat) ProcessFiles(logFiles []string, config Config) (*Result, error) {
	if len(logFiles) == 0 {
		return nil, fmt.Errorf("At least one log file required")
	}
	streams := make([]io.Reader, len(logFiles))
	for i, lf := range logFiles {
		f, e := os.Open(lf)
		if e != nil {
			return nil, e
		}
		defer f.Close()
		streams[i] = f
	}
	mr := io.MultiReader(streams...)
	return l.ProcessStream(mr, config)
}

func (l *logStat) ProcessStream(reader io.Reader, config Config) (*Result, error) {
	lp, err := line.NewLineProcessor(config.LineFilters, config.DenoisePatterns, config.DateTimeExtractors)
	if err != nil {
		return nil, err
	}
	bufr := bufio.NewReader(reader)
	result := &Result{
		Buckets:    map[time.Time]Bucket{},
	}
	for {
		str, err := bufr.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			} else {
				return nil, err
			}
		}
		str = strings.TrimSuffix(str, "\n")
		if len(config.LineFilters) > 0 && !lp.Match(str) {
			continue
		}
		if err = l.processLine(lp, config, str, result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (l *logStat) processLine(lp line.LineProcessor, config Config, line string, result *Result) error {
	datetimes := lp.Extract(line)
	if len(datetimes) == 0 {
		return fmt.Errorf("No datetime extracted from '%s'\n", line)
	}
	var logtime *time.Time
	parseErrors := []error{}
	for _, datetime := range datetimes {
		for _, format := range config.DateTimeFormats {
			lt, e := time.Parse(format, datetime)
			if e == nil {
				logtime = &lt
				break
			}
			parseErrors = append(parseErrors, e)
		}
		if logtime != nil {
			break
		}
	}
	if logtime == nil {
		return fmt.Errorf("Failed to parse datetime from %s: %v\n", datetimes, parseErrors)
	}
	if result.ReferenceTime == nil {
		result.ReferenceTime = logtime
	}

	offset := float64(logtime.Sub(*result.ReferenceTime))

	bucketOffset := int64(math.Floor(offset / float64(config.BucketDuration)))
	bucketStart := result.ReferenceTime.Add(time.Duration(bucketOffset) * config.BucketDuration)

	uniqueLine := lp.Denoise(line, config.NoiseReplacement)
	if result.Buckets[bucketStart].LineCount == nil {
		result.Buckets[bucketStart] = Bucket{
			LineCount: map[string]int{},
		}
	}
	result.Buckets[bucketStart].LineCount[uniqueLine] = result.Buckets[bucketStart].LineCount[uniqueLine] + 1

	result.TotalLines = result.TotalLines + 1
	return nil
}

func (l *logStat) Histogram(config Config, result *Result, out io.Writer) error {
	outLog := log.New(out, "", 0)

	bucketTimes := make(timeSlice, len(result.Buckets))
	i := 0
	for k := range result.Buckets {
	    bucketTimes[i] = k
	    i++
	}
	sort.Sort(bucketTimes)

	minCount := 1<<32 - 1
	maxCount := 0
	for _, bucket := range result.Buckets {
		value := 0
		for _, c := range bucket.LineCount {
			value = value + c
		}
		if value > maxCount {
			maxCount = value
		}
		if value < minCount {
			minCount = value
		}
	}

	scale := maxCount - minCount
	desiredScale := 40

	for _, startTime := range bucketTimes {
		value := 0
		for _, c := range result.Buckets[startTime].LineCount {
			value = value + c
		}

		bar := value
		if maxCount > desiredScale {
			// offset
			bar -= minCount - 1
		}
		if scale > desiredScale {
			// squish
			bar = (int)((float64(bar) / float64(scale)) * float64(desiredScale))
		}

		out.Write([]byte(fmt.Sprintf("%s: ", startTime)))
		for j := 0; j < bar; j = j + 1 {
			out.Write([]byte("*"))
		}
		out.Write([]byte(fmt.Sprintf(" %d\n", value)))
	}

	outLog.Println("")
	for _, startTime := range bucketTimes {
		outLog.Printf("%s:\n", startTime)
		for l, c := range result.Buckets[startTime].LineCount {
			outLog.Printf("  %d %s\n", c, l)
		}
	}

	return nil
}

type timeSlice []time.Time

func (p timeSlice) Len() int {
    return len(p)
}

func (p timeSlice) Less(i, j int) bool {
    return p[i].Before(p[j])
}

func (p timeSlice) Swap(i, j int) {
    p[i], p[j] = p[j], p[i]
}