package sync

import (
	"time"
)

func (s *Sync) HandleOutput(output string) {
	s.handleOutput(output)
}

func (s *Sync) GetLastError() error {
	return s.lastError
}

func (s *Sync) GetLastErrorDetails() string {
	return s.lastErrorDetails
}

func (s *Sync) GetShouldIgnoreArchives() bool {
	return s.shouldIgnoreArchives
}

func (s *Sync) GetShouldIgnoreLocks() bool {
	return s.shouldIgnoreLocks
}

func (s *Sync) GetRetryDelay() time.Duration {
	return s.retryDelay
}

func (s *Sync) GetErrorIsFatal() bool {
	return s.errorIsFatal
}

func (s *Sync) GetLastTransferCount() int {
	return s.lastTransferCount
}
