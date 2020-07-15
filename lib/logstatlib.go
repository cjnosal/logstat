package lib

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/cjnosal/logstat/pkg/line"
)

var (
	epoch = time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)
)

type LogStat interface {
	ProcessFiles(logFiles []string, config Config) (*Result, error)
	ProcessStream(reader io.Reader, config Config) (*Result, error)
	Histogram(result *Result, out io.Writer) error
	Buckets(result *Result, out io.Writer, showOriginalLines bool, minCount int) error
	LastSeen(result *Result, out io.Writer, minGap *time.Duration, maxGap *time.Duration,
		minRepetition int, maxRepetition int, minCount int, margin int) error
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
	DenoisePatterns    [][]string
	NoiseReplacement   string
	BucketDuration     time.Duration
	KeepOriginalLines  bool
	StartTime          *time.Time
	EndTime            *time.Time
}

type Result struct {
	ReferenceTime *time.Time
	Buckets       map[time.Time]*Bucket
}

type Bucket struct {
	Notes    map[string]string
	Clusters map[string]*Cluster

	LineCount int
}

type Cluster struct {
	Reference     string
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
		Buckets: map[time.Time]*Bucket{},
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
		Buckets: map[time.Time]*Bucket{},
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

	uniqueLine := lp.Denoise(line)

	bucket := result.Buckets[bucketStart]
	if bucket == nil {
		bucket = &Bucket{
			Notes:    map[string]string{},
			Clusters: map[string]*Cluster{},
		}
		result.Buckets[bucketStart] = bucket
	}
	cluster := bucket.Clusters[uniqueLine]
	if cluster == nil {
		cluster = &Cluster{
			Reference:     uniqueLine,
			OriginalLines: map[time.Time][]string{},
		}
		bucket.Clusters[uniqueLine] = cluster
	}

	clusterItem := ""
	if config.KeepOriginalLines {
		clusterItem = line
	}

	clusterLines := cluster.OriginalLines[*logtime]
	if clusterLines == nil {
		clusterLines = []string{}
	}
	clusterLines = append(clusterLines, clusterItem)
	cluster.OriginalLines[*logtime] = clusterLines

	bucket.LineCount++

	return logtime, &bucketStart, nil
}

func (l *logStat) Histogram(result *Result, out io.Writer) error {
	bucketTimes := make(timeSlice, len(result.Buckets))
	i := 0
	minCount := 1<<32 - 1
	maxCount := 0
	for k, bucket := range result.Buckets {
		bucketTimes[i] = k
		i++

		if bucket.LineCount > maxCount {
			maxCount = bucket.LineCount
		}
		if bucket.LineCount < minCount {
			minCount = bucket.LineCount
		}
	}
	sort.Sort(bucketTimes)

	scale := maxCount - minCount
	desiredScale := 40

	for _, startTime := range bucketTimes {
		bucket := result.Buckets[startTime]
		for note := range bucket.Notes {
			out.Write([]byte(fmt.Sprintf("  %s\n", note)))
		}

		bar := bucket.LineCount
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
		out.Write([]byte(fmt.Sprintf(" %d\n", bucket.LineCount)))
	}
	return nil
}

func (l *logStat) Buckets(result *Result, out io.Writer, showOriginalLines bool, minCount int) error {
	outLog := log.New(out, "", 0)

	bucketTimes := make(timeSlice, len(result.Buckets))
	i := 0
	for k := range result.Buckets {
		bucketTimes[i] = k
		i++
	}
	sort.Sort(bucketTimes)

	for _, startTime := range bucketTimes {
		header := fmt.Sprintf("%s:\n", startTime)
		bucket := result.Buckets[startTime]
		for note := range bucket.Notes {
			header += fmt.Sprintf("  %s\n", note)
		}
		empty := true

		for l, c := range bucket.Clusters {
			sum := 0
			for _, lines := range c.OriginalLines {
				sum += len(lines)
			}
			if sum >= minCount {
				if empty {
					outLog.Println(header)
					empty = false
				}
				outLog.Printf("  %4d %s\n", sum, l)
			}
		}
		if !empty {
			outLog.Printf("\n")
		}

		if showOriginalLines {
			lineTimes := make(timeSlice, bucket.LineCount)
			j := 0
			for _, c := range bucket.Clusters {
				for k := range c.OriginalLines {
					lineTimes[j] = k
					j++
				}
			}
			sort.Sort(lineTimes)
			prevTime := time.Time{}
			for _, lineTime := range lineTimes {
				if lineTime == prevTime {
					continue
				}
				prevTime = lineTime
				for _, c := range bucket.Clusters {
					sum := 0
					for _, lines := range c.OriginalLines {
						sum += len(lines)
					}
					if sum < minCount {
						continue
					}
					clusterLines := c.OriginalLines[lineTime]
					if clusterLines == nil {
						continue
					}
					for _, line := range clusterLines { // logging by cluster, not original order
						outLog.Printf("  %s\n", line)
					}
				}
			}
			if !empty {
				outLog.Printf("\n")
			}
		}
	}

	return nil
}

type Occurrences struct {
	repsByMagnitude map[int]int
}

func (l *logStat) LastSeen(result *Result, out io.Writer, minGap *time.Duration, maxGap *time.Duration,
	minRepetition int, maxRepetition int, minCount int, margin int) error {
	outLog := log.New(out, "", 0)

	bucketTimes := make(timeSlice, len(result.Buckets))
	i := 0
	for k := range result.Buckets {
		bucketTimes[i] = k
		i++
	}
	sort.Sort(bucketTimes)

	gaps := map[time.Duration]map[string]*Occurrences{}
	for i, startTime := range bucketTimes {
		for ref, cluster := range result.Buckets[startTime].Clusters {
			sum := 0
			for _, lines := range cluster.OriginalLines {
				sum += len(lines)
			}

			for j := i - 1; j >= 0; j-- {
				match := result.Buckets[bucketTimes[j]].Clusters[ref]

				if match != nil {
					matchsum := 0
					for _, lines := range match.OriginalLines {
						matchsum += len(lines)
					}

					if int(math.Abs(float64(matchsum-sum))) <= margin {
						gap := startTime.Sub(bucketTimes[j])
						smaller := int(math.Min(float64(matchsum), float64(sum)))
						grouping := gaps[gap]
						if grouping == nil {
							grouping = map[string]*Occurrences{}
							gaps[gap] = grouping
						}
						occurrences := grouping[ref]
						if occurrences == nil {
							occurrences = &Occurrences{
								repsByMagnitude: map[int]int{},
							}
							grouping[ref] = occurrences
						}
						index := smaller
						if margin > 0 {
							index = smaller/margin + 1
						}
						occurrences.repsByMagnitude[index] = occurrences.repsByMagnitude[index] + 1
						break
					}
				}
			}
		}
	}
	gapLengths := make(durationSlice, len(gaps))
	i = 0
	for k := range gaps {
		gapLengths[i] = k
		i++
	}
	sort.Sort(gapLengths)

	for _, d := range gapLengths {
		if (minGap != nil && d < *minGap) || (maxGap != nil && d > *maxGap) {
			continue
		}
		for ref, occurrences := range gaps[d] {
			for index, reps := range occurrences.repsByMagnitude {
				if (minRepetition > 0 && reps < minRepetition) || (maxRepetition > 0 && reps > maxRepetition) {
					continue
				}
				magnitude := index
				if margin > 0 {
					magnitude = magnitude * margin
				}
				if magnitude >= minCount {
					outLog.Printf("%s %3d occurrences of magnitude %3d: %s\n", d, reps, magnitude, ref)
				}
			}
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

type durationSlice []time.Duration

func (p durationSlice) Len() int {
	return len(p)
}

func (p durationSlice) Less(i, j int) bool {
	return p[i] < p[j]
}

func (p durationSlice) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}
