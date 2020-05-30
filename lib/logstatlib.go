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

var (
	epoch = time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)
)

type LogStat interface {
	ProcessFiles(logFiles []string, config Config) (*Result, error)
	ProcessStream(reader io.Reader, config Config) (*Result, error)
	Histogram(result *Result, out io.Writer) error
	Buckets(result *Result, out io.Writer) error
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
	BucketDuration     time.Duration
	KeepOriginalLines  bool
	StartTime          *time.Time
	EndTime            *time.Time
}

type Result struct {
	TotalLines int
	ReferenceTime *time.Time
	Buckets    map[time.Time]Bucket
}

type Bucket struct {
	LineCount map[string]int
	Notes map[string]string
	OriginalLines map[time.Time][]string
}

type logStat struct {
	logger *log.Logger
}

func (l *logStat) ProcessFiles(logFiles []string, config Config) (*Result, error) {
	if len(logFiles) == 0 {
		return nil, fmt.Errorf("At least one log file required")
	}
	lp, err := line.NewLineProcessor(config.LineFilters, config.DenoisePatterns, config.DateTimeExtractors)
	if err != nil {
		return nil, err
	}
	result := &Result{
		Buckets:    map[time.Time]Bucket{},
	}
	for _, lf := range logFiles {
		f, e := os.Open(lf)
		if e != nil {
			return nil, e
		}
		defer f.Close()
		bufr := bufio.NewReader(f)
		err = l.processLines(fmt.Sprintf("start of %s", lf), bufr, lp, config, result)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (l *logStat) ProcessStream(reader io.Reader, config Config) (*Result, error) {
	lp, err := line.NewLineProcessor(config.LineFilters, config.DenoisePatterns, config.DateTimeExtractors)
	if err != nil {
		return nil, err
	}
	result := &Result{
		Buckets:    map[time.Time]Bucket{},
	}
	bufr := bufio.NewReader(reader)
	err = l.processLines("stream", bufr, lp, config, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}


func (l *logStat) processLines(tag string, bufr *bufio.Reader, lp line.LineProcessor, config Config, result *Result) error {
	var tagRefTime *time.Time
	var prevLineTime *time.Time
	empty := true
	for {
		str, err := bufr.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			} else {
				return err
			}
		}
		str = strings.TrimSuffix(str, "\n")
		if len(config.LineFilters) > 0 && !lp.Match(str) {
			continue
		}
		str = strings.TrimSpace(str)
		if len(str) == 0 {
			continue
		}
		var bucketStart *time.Time
		prevLineTime, bucketStart, err = l.processLine(lp, config, str, result, prevLineTime)
		if err != nil {
			l.logger.Println(err)
		} else if bucketStart != nil {
			empty = false
			if tagRefTime == nil && prevLineTime != nil {
				tagRefTime = bucketStart
			}
		}
	}
	if !empty {
		if tagRefTime == nil {
			tagRefTime = &epoch
		}
		if tagRefTime != nil {
			result.Buckets[*tagRefTime].Notes[tag] = ""
		}
	}
	return nil
}

func (l *logStat) processLine(lp line.LineProcessor, config Config, line string, result *Result, prevLineTime *time.Time) (*time.Time, *time.Time, error) {
	datetimes := lp.Extract(line)
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
	if result.ReferenceTime == nil {
		if logtime != nil {
			result.ReferenceTime = logtime
		} else {
			result.ReferenceTime = &epoch
		}
	}
	if logtime == nil {
		if prevLineTime == nil {
			logtime = &epoch
		} else {
			logtime = prevLineTime
		}
	}

	if config.StartTime != nil && logtime.Before(*config.StartTime) {
		return nil, nil, nil
	}
	if config.EndTime != nil && logtime.After(*config.EndTime) {
		return nil, nil, nil
	}

	offset := float64(logtime.Sub(*result.ReferenceTime))

	bucketOffset := int64(math.Floor(offset / float64(config.BucketDuration)))
	bucketStart := result.ReferenceTime.Add(time.Duration(bucketOffset) * config.BucketDuration)

	uniqueLine := lp.Denoise(line, config.NoiseReplacement)
	if result.Buckets[bucketStart].LineCount == nil {
		result.Buckets[bucketStart] = Bucket{
			LineCount: map[string]int{},
			Notes: map[string]string{},
			OriginalLines: map[time.Time][]string{},
		}
	}
	result.Buckets[bucketStart].LineCount[uniqueLine] = result.Buckets[bucketStart].LineCount[uniqueLine] + 1
	if config.KeepOriginalLines {
		if result.Buckets[bucketStart].OriginalLines[*logtime] == nil {
			result.Buckets[bucketStart].OriginalLines[*logtime] = []string{}
		}
		result.Buckets[bucketStart].OriginalLines[*logtime] = append(result.Buckets[bucketStart].OriginalLines[*logtime], line)
	}

	result.TotalLines = result.TotalLines + 1
	return logtime, &bucketStart, nil
}

func (l *logStat) Histogram(result *Result, out io.Writer) error {
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
		for note := range result.Buckets[startTime].Notes {
			out.Write([]byte(fmt.Sprintf("  %s\n", note)))
		}

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
	return nil
}

func (l *logStat) Buckets(result *Result, out io.Writer) error {
	outLog := log.New(out, "", 0)

	bucketTimes := make(timeSlice, len(result.Buckets))
	i := 0
	for k := range result.Buckets {
	    bucketTimes[i] = k
	    i++
	}
	sort.Sort(bucketTimes)

	for _, startTime := range bucketTimes {
		outLog.Printf("%s:\n", startTime)
		for note := range result.Buckets[startTime].Notes {
			outLog.Printf("  %s\n", note)
		}
		outLog.Printf("\n")
		
		for l, c := range result.Buckets[startTime].LineCount {
			outLog.Printf("  %4d %s\n", c, l)
		}
		outLog.Printf("\n")

		if len(result.Buckets[startTime].OriginalLines) > 0 {
			lineTimes := make(timeSlice, len(result.Buckets[startTime].OriginalLines))
			j := 0
			for k := range result.Buckets[startTime].OriginalLines {
			    lineTimes[j] = k
			    j++
			}
			sort.Sort(lineTimes)
			for _, lineTime := range lineTimes {
				for _, line := range result.Buckets[startTime].OriginalLines[lineTime] {
					outLog.Printf("  %s\n", line)
				}
			}
			outLog.Printf("\n")
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