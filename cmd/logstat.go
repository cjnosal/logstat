package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"time"

	"github.com/cjnosal/logstat/pkg/regex"

	"github.com/spf13/cobra"

	"github.com/cjnosal/logstat/lib"
)

var userDenoisePatterns []string
var searchPatterns []string
var datetimePatterns []string
var datetimeFormats []string
var bucketLength string
var noiseReplacement string
var showBuckets bool
var mergeFiles bool

var startTime string
var endTime string

var replaceGuids bool
var replaceBase64 bool
var replaceAlphaNumeric bool
var replaceNumbers bool
var replaceLongWords bool
var replaceLongHex bool
var replaceEmails bool

var logger *log.Logger

func main() {
	logger = log.New(os.Stderr, "[main] ", 0)

	command := cobra.Command{
		Use:   "logstat [files...]",
		Short: "find interesting things in your logs",
		Args:  cobra.ArbitraryArgs,
		Run:   run,
	}

	command.Flags().StringSliceVarP(&userDenoisePatterns, "denoise", "d", []string{}, "regex patterns to ignore when determining unique lines (e.g. timestamps, replaceGuidss)")
	command.Flags().StringSliceVarP(&searchPatterns, "search", "s", []string{}, "search for lines matching regex pattern")
	command.Flags().StringSliceVarP(&datetimePatterns, "datetime", "t", []string{}, "extract line datetime regex pattern")
	command.Flags().StringSliceVarP(&datetimeFormats, "dateformat", "f", []string{}, "format for parsing extracted datetimes (use golang reference time 'Mon Jan 2 15:04:05 MST 2006')")

	command.Flags().StringVarP(&bucketLength, "bucketlength", "l", "1m", "length of time in each bucket")
	command.Flags().BoolVarP(&showBuckets, "showbuckets", "b", false, "show line counts for each time bucket")
	command.Flags().BoolVarP(&mergeFiles, "mergefiles", "m", false, "show original lines from each file interleaved by time")
	command.Flags().StringVarP(&startTime, "starttime", "", "", "exclude lines before this time")
	command.Flags().StringVarP(&endTime, "endtime", "", "", "exclude lines after this time")

	command.Flags().StringVarP(&noiseReplacement, "noise", "n", "*", "string to show where noise was removed")
	command.Flags().BoolVarP(&replaceGuids, "guids", "", true, "denoise guids")
	command.Flags().BoolVarP(&replaceBase64, "base64", "", true, "denoise base64 strings")
	command.Flags().BoolVarP(&replaceAlphaNumeric, "alphanum", "", true, "denoise all alphanumeric strings")
	command.Flags().BoolVarP(&replaceNumbers, "numbers", "", true, "denoise all numbers")
	command.Flags().BoolVarP(&replaceLongWords, "longwords", "", true, "denoise 20+ character words")
	command.Flags().BoolVarP(&replaceLongHex, "longhex", "", true, "denoise 16+ character hexadecimal strings")
	command.Flags().BoolVarP(&replaceEmails, "emails", "", true, "denoise all emails")

	err := command.Execute()
	if err != nil {
		logger.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) {
	libLogger := log.New(os.Stderr, "[logstatlib] ", 0)
	lsl := lib.NewLogStat(libLogger)
	var result *lib.Result
	var err error

	duration, err := time.ParseDuration(bucketLength)
	if err != nil {
		logger.Printf("Error parsing bucket length: %v\n", err)
		os.Exit(1)
	}

	defaultDateTimePattern := regex.RFC3339LIKE
	defaultDateTimeFormats := []string{
		time.RFC3339Nano,                     // - T n Zhh:mm
		"2006-01-02T15:04:05.999999999Z0700", // - T n Zhhmm
		"2006-01-02T15:04:05.999999999Z07",   // - T n Zhh
		time.RFC3339,                         // - T   Zhh:mm
		"2006-01-02T15:04:05Z0700",           // - T   Zhhmm
		"2006-01-02T15:04:05Z07",             // - T   Zhh
		"2006-01-02T15:04:05.999999999",      // - T n
		"2006-01-02T15:04:05",                // - T

		"2006/01/02T15:04:05.999999999Z07:00", // / T n Zhh:mm
		"2006/01/02T15:04:05.999999999Z0700",  // / T n Zhhmm
		"2006/01/02T15:04:05.999999999Z07",    // / T n Zhh
		"2006/01/02T15:04:05Z07:00",           // / T   Zhh:mm
		"2006/01/02T15:04:05Z0700",            // / T   Zhhmm
		"2006/01/02T15:04:05Z07",              // / T   Zhh
		"2006/01/02T15:04:05.999999999",       // / T n
		"2006/01/02T15:04:05",                 // / T

		"2006-01-02 15:04:05.999999999Z07:00", // -   n Zhh:mm
		"2006-01-02 15:04:05.999999999Z0700",  // -   n Zhhmm
		"2006-01-02 15:04:05.999999999Z07",    // -   n Zhh
		"2006-01-02 15:04:05Z07:00",           // -     Zhh:mm
		"2006-01-02 15:04:05Z0700",            // -     Zhhmm
		"2006-01-02 15:04:05Z07",              // -     Zhh
		"2006-01-02 15:04:05.999999999",       // -   n
		"2006-01-02 15:04:05",                 // -

		"2006/01/02 15:04:05.999999999Z07:00", // /   n Zhh:mm
		"2006/01/02 15:04:05.999999999Z0700",  // /   n Zhhmm
		"2006/01/02 15:04:05.999999999Z07",    // /   n Zhh
		"2006/01/02 15:04:05Z07:00",           // /     Zhh:mm
		"2006/01/02 15:04:05Z0700",            // /     Zhhmm
		"2006/01/02 15:04:05Z07",              // /     Zhh
		"2006/01/02 15:04:05.999999999",       // /   n
		"2006/01/02 15:04:05",                 // /
	}
	datetimeFormats = append(datetimeFormats, defaultDateTimeFormats...)
	datetimePatterns = append(datetimePatterns, defaultDateTimePattern)

	// regexes must be ordered from more structured to less structured
	denoisePatterns := datetimePatterns
	if replaceGuids {
		denoisePatterns = append(denoisePatterns, regex.GUID)
	}
	if replaceBase64 {
		denoisePatterns = append(denoisePatterns, regex.BASE64)
	}
	if replaceLongHex {
		denoisePatterns = append(denoisePatterns, regex.LONGHEX)
	}
	if replaceLongWords {
		denoisePatterns = append(denoisePatterns, regex.LONGWORDS)
	}
	if replaceEmails {
		denoisePatterns = append(denoisePatterns, regex.EMAILS)
	}
	denoisePatterns = append(denoisePatterns, userDenoisePatterns...)
	if replaceAlphaNumeric {
		denoisePatterns = append(denoisePatterns, regex.ALPHANUM)
	}
	if replaceNumbers {
		denoisePatterns = append(denoisePatterns, regex.NUMBERS)
	}
	denoisePatterns = append(denoisePatterns, fmt.Sprintf("(%s)+", regexp.QuoteMeta(noiseReplacement)))

	var start *time.Time
	var end *time.Time
	if startTime != "" {
		start, err = parseTime(startTime, datetimeFormats)
	}
	if err != nil {
		logger.Printf("Error parsing start time: %v\n", err)
		os.Exit(1)
	}
	if endTime != "" {
		end, err = parseTime(endTime, datetimeFormats)
	}
	if err != nil {
		logger.Printf("Error parsing end time: %v\n", err)
		os.Exit(1)
	}

	config := lib.Config{
		LineFilters:        searchPatterns,
		DenoisePatterns:    denoisePatterns,
		DateTimeExtractors: datetimePatterns,
		DateTimeFormats:    datetimeFormats,
		BucketDuration:     duration,
		NoiseReplacement:   noiseReplacement,
		KeepOriginalLines:  mergeFiles,
		StartTime:          start,
		EndTime:            end,
	}
	if len(args) == 0 {
		result, err = lsl.ProcessStream(os.Stdin, config)
	} else {
		result, err = lsl.ProcessFiles(args, config)
	}
	if err != nil {
		logger.Printf("Error processing logs: %v\n", err)
		os.Exit(1)
	}

	err = lsl.Histogram(result, os.Stdout)
	if err != nil {
		logger.Printf("Error rendering histogram: %v\n", err)
		os.Exit(1)
	}

	if showBuckets {
		os.Stdout.Write([]byte{'\n'})
		err = lsl.Buckets(result, os.Stdout, mergeFiles)
		if err != nil {
			logger.Printf("Error rendering buckets: %v\n", err)
			os.Exit(1)
		}
	}

	lsl.LastSeen(result, os.Stdout)
}

func parseTime(datetime string, formats []string) (*time.Time, error) {
	for _, format := range formats {
		lt, e := time.Parse(format, datetime)
		if e == nil {
			return &lt, nil
		}
	}
	return nil, fmt.Errorf("%s does not match any formats in %v", datetime, formats)
}
