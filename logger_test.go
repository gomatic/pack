package pack_test

import (
	"bytes"
	"regexp"
	"testing"

	"github.com/fatih/color"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/pack"
	h "github.com/buildpack/pack/testhelpers"
)

func TestLogger(t *testing.T) {
	color.NoColor = false // IMPORTANT: Keep this to avoid false positive tests
	spec.Run(t, "Logger", testLogger, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testLogger(t *testing.T, when spec.G, it spec.S) {

	var (
		logger  *pack.Logger
		outBuf  bytes.Buffer
		errBuff bytes.Buffer
	)

	when("debug", func() {
		when("logger has debug enabled", func() {
			it.Before(func() {
				logger = pack.NewLogger(&outBuf, &errBuff, true, false)
			})

			it("shows verbose output", func() {
				logger.Debug("Some verbose output")

				h.AssertContains(t, outBuf.String(), "Some verbose output")
			})
		})

		when("logger has debug disabled", func() {
			it.Before(func() {
				logger = pack.NewLogger(&outBuf, &errBuff, false, false)
			})

			it("does not show verbose output", func() {
				logger.Debug("Some verbose output")

				h.AssertEq(t, outBuf.String(), "")
			})
		})
	})

	when("timestamps", func() {
		when("logger has timestamps enabled", func() {
			it.Before(func() {
				logger = pack.NewLogger(&outBuf, &errBuff, false, true)
			})

			it("prefixes logging with timestamp", func() {
				logger.Info("Some text")

				h.AssertMatch(t, outBuf.String(), regexp.MustCompile(
					`^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} \Q`+color.HiCyanString("| ")+`\ESome text`))
			})
		})

		when("logger has timestamps disabled", func() {
			it.Before(func() {
				logger = pack.NewLogger(&outBuf, &errBuff, false, false)
			})

			it("does not prefix logging with timestamp", func() {
				logger.Info("Some text")

				h.AssertEq(t, outBuf.String(), "Some text\n")
			})
		})
	})

	when("styling", func() {
		it.Before(func() {
			logger = pack.NewLogger(&outBuf, &errBuff, false, false)
		})

		when("#Info", func() {
			it("displays unstyled info message", func() {
				logger.Info("This is some info")

				h.AssertContains(t, outBuf.String(), "This is some info")
			})
		})

		when("#Error", func() {
			it("displays styled error message to error buffer", func() {
				logger.Error("Something went wrong!")

				expectedColor := color.New(color.FgRed, color.Bold).SprintFunc()
				h.AssertContains(t, errBuff.String(), expectedColor("ERROR: ")+"Something went wrong!")
			})
		})

		when("#Tip", func() {
			it("displays styled tip message", func() {
				logger.Tip("This is a tip")

				expectedColor := color.New(color.FgHiGreen, color.Bold).SprintFunc()
				h.AssertContains(t, outBuf.String(), expectedColor("Tip: ")+"This is a tip")
			})
		})
	})
}
