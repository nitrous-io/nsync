package sync_test

import (
	"testing"
	"time"

	"github.com/nitrous-io/nsync/nsync/sync"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "nsync/sync")
}

var _ = Describe("nsync/sync", func() {
	var s *sync.Sync

	Describe("handleOutput", func() {
		BeforeEach(func() {
			s = &sync.Sync{}
		})

		It("does not change anything", func() {
			s.HandleOutput("")
			Expect(s.GetLastError()).To(BeNil())
			Expect(s.GetLastErrorDetails()).To(BeZero())
			Expect(s.GetErrorIsFatal()).To(BeFalse())
			Expect(s.GetShouldIgnoreArchives()).To(BeFalse())
			Expect(s.GetShouldIgnoreLocks()).To(BeFalse())
			Expect(s.GetRetryDelay()).To(Equal(0 * time.Millisecond))
		})

		Context("the message starts with `Error: `", func() {
			Context("the message contains `On-disk archives are not identical.`", func() {
				It("sets error message", func() {
					s.HandleOutput("Error: On-disk archives are not identical.")
					Expect(s.GetLastError()).To(Equal(sync.ErrInconsistent))
					Expect(s.GetLastErrorDetails()).To(BeZero())
					Expect(s.GetErrorIsFatal()).To(BeFalse())
					Expect(s.GetShouldIgnoreArchives()).To(BeTrue())
					Expect(s.GetShouldIgnoreLocks()).To(BeFalse())
					Expect(s.GetRetryDelay()).To(Equal(200 * time.Millisecond))
				})
			})

			Context("the error message contains `Warning: the archives are locked.`", func() {
				It("sets error message", func() {
					s.HandleOutput("Error: Warning: the archives are locked.")
					Expect(s.GetLastError()).To(Equal(sync.ErrLockedArchives))
					Expect(s.GetLastErrorDetails()).To(BeZero())
					Expect(s.GetErrorIsFatal()).To(BeFalse())
					Expect(s.GetShouldIgnoreArchives()).To(BeFalse())
					Expect(s.GetShouldIgnoreLocks()).To(BeTrue())
					Expect(s.GetRetryDelay()).To(Equal(200 * time.Millisecond))
				})
			})

			Context("the error message contains `Lost connection with the server`", func() {
				It("sets error message", func() {
					s.HandleOutput("Error: Lost connection with the server")
					Expect(s.GetLastError()).To(Equal(sync.ErrConnectionLost))
					Expect(s.GetLastErrorDetails()).To(BeZero())
					Expect(s.GetErrorIsFatal()).To(BeFalse())
					Expect(s.GetShouldIgnoreArchives()).To(BeFalse())
					Expect(s.GetShouldIgnoreLocks()).To(BeFalse())
					Expect(s.GetRetryDelay()).To(Equal(0 * time.Millisecond))
				})
			})

			Context("the error message contains `They cannot be synchronized to a case-insensitive file system.`", func() {
				It("sets error message", func() {
					s.HandleOutput(`Error: They cannot be synchronized to a case-insensitive file system. (File 'abc.tmp')`)

					Expect(s.GetLastError()).To(Equal(sync.ErrIdenticalFilenames))
					Expect(s.GetLastErrorDetails()).To(Equal(`Two or more files have identical names except for the case. Please rename "abc.tmp".`))
					Expect(s.GetErrorIsFatal()).To(BeFalse())
					Expect(s.GetShouldIgnoreArchives()).To(BeFalse())
					Expect(s.GetShouldIgnoreLocks()).To(BeFalse())
					Expect(s.GetRetryDelay()).To(Equal(5 * time.Second))
				})
			})

			Context("the error message contains `They cannot be synchronized to a case-insensitive file system.`", func() {
				It("sets error message", func() {
					s.HandleOutput(`Error: They cannot be synchronized to a case-insensitive file system. (File 'abc.tmp')`)

					Expect(s.GetLastError()).To(Equal(sync.ErrIdenticalFilenames))
					Expect(s.GetLastErrorDetails()).To(Equal(`Two or more files have identical names except for the case. Please rename "abc.tmp".`))
					Expect(s.GetErrorIsFatal()).To(BeFalse())
					Expect(s.GetShouldIgnoreArchives()).To(BeFalse())
					Expect(s.GetShouldIgnoreLocks()).To(BeFalse())
					Expect(s.GetRetryDelay()).To(Equal(5 * time.Second))
				})
			})

			Context("the error message contains `Malformed pattern: `", func() {
				It("sets error message", func() {
					s.HandleOutput(`Error:
						line 1: Malformed pattern: "Regex .+(?<!keep)\.pdf$".`)

					Expect(s.GetLastError()).To(Equal(sync.ErrMalformedPattern))
					Expect(s.GetLastErrorDetails()).To(Equal(`Regex .+(?<!keep)\.pdf$`))
					Expect(s.GetErrorIsFatal()).To(BeTrue())
					Expect(s.GetShouldIgnoreArchives()).To(BeFalse())
					Expect(s.GetShouldIgnoreLocks()).To(BeFalse())
					Expect(s.GetRetryDelay()).To(Equal(0 * time.Second))
				})
			})
		})

		Context("the message contains `Uncaught exception Sys_blocked_io`", func() {
			It("set retry delay to 500ms", func() {
				s.HandleOutput("Uncaught exception Sys_blocked_io")

				Expect(s.GetLastError()).To(BeNil())
				Expect(s.GetLastErrorDetails()).To(BeZero())
				Expect(s.GetErrorIsFatal()).To(BeFalse())
				Expect(s.GetShouldIgnoreArchives()).To(BeFalse())
				Expect(s.GetShouldIgnoreLocks()).To(BeFalse())
				Expect(s.GetRetryDelay()).To(Equal(500 * time.Millisecond))
			})
		})

		Context("the message contains `Permission denied (`", func() {
			It("returns ssh ErrSSHAccessDenied and sets retry delay to 200ms", func() {
				s.HandleOutput("Permission denied (/usr/local/bin)")

				Expect(s.GetLastError()).To(Equal(sync.ErrSSHAccessDenied))
				Expect(s.GetLastErrorDetails()).To(BeZero())
				Expect(s.GetErrorIsFatal()).To(BeFalse())
				Expect(s.GetShouldIgnoreArchives()).To(BeFalse())
				Expect(s.GetShouldIgnoreLocks()).To(BeFalse())
				Expect(s.GetRetryDelay()).To(Equal(200 * time.Millisecond))
			})
		})

		Context("the message has complete message", func() {
			It("sets lastTransferCount", func() {
				s.HandleOutput("Synchronization complete at 11:19:40  (1 item transferred, 0 skipped, 0 failed)")

				Expect(s.GetLastError()).To(BeNil())
				Expect(s.GetLastErrorDetails()).To(BeZero())
				Expect(s.GetErrorIsFatal()).To(BeFalse())
				Expect(s.GetShouldIgnoreArchives()).To(BeFalse())
				Expect(s.GetShouldIgnoreLocks()).To(BeFalse())
				Expect(s.GetRetryDelay()).To(Equal(0 * time.Millisecond))
				Expect(s.GetLastTransferCount()).To(Equal(1))
			})
		})
	})
})
