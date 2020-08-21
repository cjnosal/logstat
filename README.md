# logstat
A CLI tool to search and analyze log files

Features:
* Parse dates in log entries to merge and correlate related log files
* Display a histogram of log volume over time
* Filter highly variable strings (e.g. dates, guids, IPs) to find similar log entries
* Filter by time range
* Search for log entries that repeat on a regular interval

```
Usage:
  logstat [files...] [flags]

Flags:
      --alphanum                 denoise all alphanumeric strings (default true)
      --base64                   denoise base64 strings (default true)
  -l, --bucketlength string      length of time in each bucket (default "1m")
  -f, --dateformat stringArray   format for parsing extracted datetimes (use golang reference time 'Mon Jan 2 15:04:05 MST 2006')
  -t, --datetime stringArray     extract line datetime regex pattern
  -d, --denoise stringArray      regex patterns to ignore when determining unique lines (e.g. timestamps, guids)
                                 can include custom replacement (overriding -n) with -d pattern=replacement
                                 can escape = with \
      --emails                   denoise all emails (default true)
      --endtime string           exclude lines after this time
      --guids                    denoise guids (default true)
  -h, --help                     help for logstat
      --longhex                  denoise 16+ character hexadecimal strings (default true)
      --longwords                denoise 20+ character words (default true)
      --margin int               max difference in number of similar lines in two buckets
      --maxgap string            exclude gaps larger than this duration
      --maxrep int               exclude gaps with many repetitions (default -1)
  -m, --mergefiles               show original lines from each file interleaved by time
      --mincount int             minimum number of similar lines in a bucket (default 1)
      --mingap string            exclude gaps smaller than this duration
      --minrep int               exclude gaps with few repetitions (default -1)
  -n, --noise string             default string to show where user provided denoise patterns were removed (default "*")
      --numbers                  denoise all numbers (default true)
  -s, --search stringArray       search for lines matching regex pattern
  -b, --showbuckets              show line counts for each time bucket
  -g, --showgaps                 show bucket gaps and occurrences for denoised lines
      --starttime string         exclude lines before this time
```
