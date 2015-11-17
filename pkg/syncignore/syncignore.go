package syncignore

import (
	"fmt"
	"regexp"
	"strings"
)

var regexSpecialCharsRe = regexp.MustCompile(`([\-\[\]\{\}\(\)\*\+\?\.\\\^\$\|])`)

func Parse(content string) []string {
	var ignores []string

	lines := strings.Split(content, "\n")
	for i := 0; i < len(lines); i++ {
		ignore := ParseLine(lines[i])
		if len(ignore) != 0 {
			ignores = append(ignores, ignore...)
		}
	}

	return ignores
}

func ParseLine(line string) []string {
	var result []string

	pathMode := false
	line = strings.TrimSpace(line)

	if len(line) == 0 {
		return result
	}

	if strings.HasPrefix(line, "#") {
		return result
	}

	if strings.HasPrefix(line, "/") {
		pathMode = true
		line = line[1:]
	}

	if strings.Contains(line, "**") {
		escapedLine := regexSpecialCharsRe.ReplaceAllString(line, `\${1}`)
		escapedLine = strings.Replace(escapedLine, `\*\*`, `.*`, -1)
		if !pathMode {
			result = append(result, fmt.Sprintf("Regex .*/%s", escapedLine))
		}
		result = append(result, fmt.Sprintf("Regex %s", escapedLine))
	} else if strings.HasSuffix(line, "/") {
		escapedLine := regexSpecialCharsRe.ReplaceAllString(line, `\${1}`)
		result = append(result, fmt.Sprintf("Regex .*/%s", escapedLine))
		result = append(result, fmt.Sprintf("Regex %s.*", escapedLine))
		result = append(result, fmt.Sprintf("Path %s", line[:len(line)-1]))
	} else {
		if pathMode {
			result = append(result, fmt.Sprintf("Path %s", line))
		} else {
			result = append(result, fmt.Sprintf("Name %s", line))
		}
	}

	return result
}
