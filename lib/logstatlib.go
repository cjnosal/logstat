package lib

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

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

	BucketDuration time.Duration
}

type Result struct {
	TotalLines int
	StartTime  *time.Time
	Buckets    map[int]Bucket
	LastBucket int
}

type Bucket struct {
	StartTime *time.Time
	LineCount map[string]int
}

type logStat struct {
	logger *log.Logger
}

func (l *logStat) ProcessFiles(logFiles []string, config Config) (*Result, error) {
	if len(logFiles) == 0 {
		return nil, fmt.Errorf("At least one log file required")
	}
	streams := []io.Reader{}
	for _, lf := range logFiles {
		f, e := os.Open(lf)
		if e != nil {
			return nil, e
		}
		defer f.Close()
		streams = append(streams, f)
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
		Buckets:    map[int]Bucket{},
		LastBucket: -1,
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
	bucket := 0
	var bucketStart time.Time
	datetimes := lp.Extract(line)
	var datetime string
	if len(datetimes) == 0 {
		l.logger.Printf("No datetime extracted from '%s' - appending to current bucket\n", line)
		if len(result.Buckets) > 0 {
			bucket = len(result.Buckets) - 1
		}
	} else {
		datetime = datetimes[0]
		var logtime *time.Time
		parseErrors := []error{}
		for _, i := range config.DateTimeFormats {
			lt, e := time.Parse(i, datetime)
			if e == nil {
				logtime = &lt
				break
			}
			parseErrors = append(parseErrors, e)
		}
		if logtime == nil {
			return fmt.Errorf("Failed to parse datetime from %s: %v\n", datetime, parseErrors)
		}
		if result.StartTime == nil {
			result.StartTime = logtime
		}
		if logtime.Before(*result.StartTime) {
			return fmt.Errorf("No going back - ensure the log with the earliest timestamp (%v) is the first argument\n", logtime)
		}
		bucketStart = *result.StartTime
		t := result.StartTime.Add(config.BucketDuration)
		for {
			if t.After(*logtime) {
				break
			}
			bucketStart = t
			t = t.Add(config.BucketDuration)
			bucket = bucket + 1
		}

	}

	uniqueLine := lp.Denoise(line)
	if result.Buckets[bucket].LineCount == nil {
		result.Buckets[bucket] = Bucket{
			LineCount: map[string]int{},
			StartTime: &bucketStart,
		}
	}
	result.Buckets[bucket].LineCount[uniqueLine] = result.Buckets[bucket].LineCount[uniqueLine] + 1
	if bucket > result.LastBucket {
		result.LastBucket = bucket
	}

	result.TotalLines = result.TotalLines + 1
	return nil
}

func (l *logStat) Histogram(config Config, result *Result, out io.Writer) error {
	outLog := log.New(out, "", 0)

	minCount := 1<<32 - 1
	maxCount := 0
	for i := 0; i <= result.LastBucket; i = i + 1 {
		value := 0
		if result.Buckets[i].StartTime != nil {
			for _, c := range result.Buckets[i].LineCount {
				value = value + c
			}
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

	for i := 0; i <= result.LastBucket; i = i + 1 {
		value := 0
		startTime := result.Buckets[i].StartTime
		if startTime != nil {
			for _, c := range result.Buckets[i].LineCount {
				value = value + c
			}
		} else {
			t := result.StartTime.Add(config.BucketDuration * time.Duration(i))
			startTime = &t
		}

		bar := (int)((float64(value-minCount) / float64(scale)) * float64(desiredScale))

		out.Write([]byte(fmt.Sprintf("%s: ", startTime)))
		for j := 0; j < bar; j = j + 1 {
			out.Write([]byte("*"))
		}
		out.Write([]byte(fmt.Sprintf(" %d\n", value)))
	}

	outLog.Println("")
	for i := 0; i <= result.LastBucket; i = i + 1 {
		if result.Buckets[i].StartTime != nil {
			for l, c := range result.Buckets[i].LineCount {
				outLog.Printf("%s: %d %s\n", *result.Buckets[i].StartTime, c, l)
			}
		}
	}

	return nil
}
