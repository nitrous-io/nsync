package sync

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	outputErrRe         = regexp.MustCompile(`(?ms)[Ee]rror:\s*(.+?)\s*\z`)
	identicalFilenameRe = regexp.MustCompile(`\(File\s+'([^']+)`)
	malformedPatternRe  = regexp.MustCompile(`Malformed pattern: "(.+)"\.$`)
	transferCountRe     = regexp.MustCompile(`Synchronization complete at .+ \(([\d,]+) items? transferred`)
)

var (
	ErrInconsistent       = errors.New("Inconsistent state")
	ErrLockedArchives     = errors.New("Archives are locked")
	ErrConnectionLost     = errors.New("Connection lost")
	ErrIdenticalFilenames = errors.New("Files with identical names are present")
	ErrMalformedPattern   = errors.New("Patterns are malformed")
	ErrSSHAccessDenied    = errors.New("SSH access denied")
)

func (s *Sync) handleOutput(out string) {
	match := outputErrRe.FindStringSubmatch(out)
	if len(match) >= 2 {
		errMsg := match[1]

		switch {
		case strings.Contains(errMsg, "Warning: inconsistent state."):
			fallthrough
		case strings.Contains(errMsg, "On-disk archives are not identical."):
			{
				s.lastError = ErrInconsistent
				s.shouldIgnoreArchives = true
				s.retryDelay = 200 * time.Millisecond
			}
		case strings.Contains(errMsg, "Warning: the archives are locked."):
			{
				s.lastError = ErrLockedArchives
				s.shouldIgnoreLocks = true
				s.retryDelay = 200 * time.Millisecond
			}
		case strings.Contains(errMsg, "Lost connection with the server"):
			{
				s.lastError = ErrConnectionLost
			}
		case strings.Contains(errMsg, "They cannot be synchronized to a case-insensitive file system."):
			{
				s.lastError = ErrIdenticalFilenames
				s.lastErrorDetails = "Two or more files have identical names except for the case."
				s.retryDelay = 5 * time.Second

				match := identicalFilenameRe.FindStringSubmatch(out)
				if len(match) >= 2 {
					fn := match[1]
					s.lastErrorDetails += fmt.Sprintf(` Please rename "%s".`, fn)
				}
			}
		case strings.Contains(out, "Malformed pattern:"):
			{
				s.lastError = ErrMalformedPattern

				match := malformedPatternRe.FindStringSubmatch(errMsg)
				if len(match) >= 2 {
					patMsg := match[1]
					s.lastErrorDetails = patMsg
					s.errorIsFatal = true
				}
			}
		}
	}

	if strings.Contains(out, "Uncaught exception Sys_blocked_io") {
		s.retryDelay = 500 * time.Millisecond
	}

	if strings.Contains(out, "Permission denied (") {
		s.lastError = ErrSSHAccessDenied
		s.retryDelay = 200 * time.Millisecond
	}

	match = transferCountRe.FindStringSubmatch(out)
	if len(match) >= 2 {
		transferCount, err := strconv.Atoi(match[1])
		if err == nil {
			s.lastTransferCount = transferCount
		}
	}
}
