package regex

const (
	GUID      = "\\{?[0-9a-fA-F]{8}\\-[0-9a-fA-F]{4}\\-[0-9a-fA-F]{4}\\-[0-9a-fA-F]{4}\\-[0-9a-fA-F]{12}\\}?"
	BASE64    = "^(.*[^A-Za-z0-9])?([A-Za-z0-9+/]{4})*([A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)"
	ALPHANUM  = "([0-9]+[A-Za-z]+|[A-Za-z]+[0-9]+)[0-9A-Za-z]*"
	EMAILS    = "\\S+@\\S+"
	NUMBERS   = "\\d+"
	LONGHEX   = "[0-9a-fA-F]{16,}"
	LONGWORDS = "\\w{20,}"

	// look for rfc3339-like numeric datetimes
	RFC3339LIKE = "\\d\\d\\d\\d[-/]\\d\\d[-/]\\d\\d[T ]\\d\\d:\\d\\d:\\d\\d(\\.\\d*)?Z?[+-]?(\\d\\d)?:?(\\d\\d)?"
)
