package main

import (
	"log"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/cjnosal/logstat/lib"
)

var denoisePatterns []string
var searchPatterns []string
var datetimePatterns []string
var datetimeFormats []string
var bucketLength string
var logger *log.Logger

func main() {
	logger = log.New(os.Stderr, "[main] ", 0)

	command := cobra.Command{
		Use:   "logstat [files...]",
		Short: "find interesting things in your logs",
		Args:  cobra.ArbitraryArgs,
		Run:   run,
	}

	command.Flags().StringSliceVarP(&denoisePatterns, "denoise", "d", []string{}, "regex patterns to ignore when determining unique lines (e.g. timestamps, guids)")
	command.Flags().StringSliceVarP(&searchPatterns, "search", "s", []string{}, "search for lines matching regex pattern")
	command.Flags().StringSliceVarP(&datetimePatterns, "datetime", "t", []string{}, "extract line datetime regex pattern")
	command.Flags().StringSliceVarP(&datetimeFormats, "dateformat", "f", []string{}, "format for parsing extracted datetimes (use golang reference time 'Mon Jan 2 15:04:05 MST 2006')")
	command.Flags().StringVarP(&bucketLength, "bucketlength", "l", "1m", "length of time in each bucket")

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

	// look for rfc3339-like numeric datetimes
	defaultDateTimePattern := "\\d\\d\\d\\d[-/]\\d\\d[-/]\\d\\d[T ]\\d\\d:\\d\\d:\\d\\d(\\.\\d*)?Z?[+-]?(\\d\\d)?:?(\\d\\d)?"
	defaultDateTimeFormats := []string{
		time.RFC3339Nano,                      // - T n Zhh:mm
		"2006-01-02T15:04:05.999999999Z0700",  // - T n Zhhmm
		"2006-01-02T15:04:05.999999999Z07",    // - T n Zhh
		time.RFC3339,                          // - T   Zhh:mm
		"2006-01-02T15:04:05Z0700",            // - T   Zhhmm
		"2006-01-02T15:04:05Z07",              // - T   Zhh
		"2006-01-02T15:04:05.999999999",       // - T n
		"2006-01-02T15:04:05",                 // - T

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
	config := lib.Config{
		LineFilters:        searchPatterns,
		DenoisePatterns:    append(denoisePatterns, datetimePatterns...),
		DateTimeExtractors: datetimePatterns,
		DateTimeFormats:    datetimeFormats,
		BucketDuration:     duration,
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

	err = lsl.Histogram(config, result, os.Stdout)
	if err != nil {
		logger.Printf("Error rendering histogram: %v\n", err)
		os.Exit(1)
	}
}
