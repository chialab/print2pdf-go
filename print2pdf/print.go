package print2pdf

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// Represents a print format's width and height, in inches.
type PrintFormat struct {
	Width  float64
	Height float64
}

// Map of format names to their dimensions, in inches. Taken from https://pptr.dev/api/puppeteer.paperformat.
var FormatsMap = map[string]PrintFormat{
	"Letter":  {8.5, 11},
	"Legal":   {8.5, 14},
	"Tabloid": {11, 17},
	"Ledger":  {17, 11},
	"A0":      {33.1, 46.8},
	"A1":      {23.4, 33.1},
	"A2":      {16.54, 23.4},
	"A3":      {11.7, 16.54},
	"A4":      {8.27, 11.7},
	"A5":      {5.83, 8.27},
	"A6":      {4.13, 5.83},
}

// Validation error in supplied parameter.
type ValidationError struct {
	message string
}

// Implement error interface.
func (v ValidationError) Error() string {
	return v.message
}

// Create a new validation error.
func NewValidationError(message string) ValidationError {
	return ValidationError{message}
}

// Chromium binary path. Required.
var ChromiumPath = os.Getenv("CHROMIUM_PATH")

// Reference to browser context, initialized in init function of this package.
var browserCtx context.Context

// Allocate a browser to be reused by multiple handler invocations, to reduce startup time.
func init() {
	if ChromiumPath == "" {
		fmt.Fprintln(os.Stderr, "missing required environment variable CHROMIUM_PATH")
		os.Exit(1)
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(ChromiumPath))
	allocatorCtx, allocatorCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	browserCtx, _ = chromedp.NewContext(allocatorCtx)

	// Navigate to blank page so that the browser is started.
	err := chromedp.Run(browserCtx, chromedp.Tasks{chromedp.Navigate("about:blank")})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error initializing browser: %v", err)
		os.Exit(1)
	}

	// Listen for interrupt/sigterm and gracefully close the browser.
	ch := make(chan os.Signal, 2)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ch
		fmt.Println("interrupt received, closing browser before exiting...")
		allocatorCancel()
		os.Exit(0)
	}()
}

// Get print format dimensions from string name.
func getFormat(format string) (PrintFormat, error) {
	f, ok := FormatsMap[format]
	if !ok {
		supportedFormats := make([]string, len(FormatsMap))
		for k := range FormatsMap {
			supportedFormats = append(supportedFormats, k)
		}

		return PrintFormat{}, NewValidationError(fmt.Sprintf("invalid format \"%s\", valid formats are: %s", format, strings.Join(supportedFormats, ", ")))
	}

	return f, nil
}

// Prepare chromedp's print parameters from provider parameters, with defaults.
// Return an error if validation of any parameter fails.
func getPrintParams(data GetPDFParams) (page.PrintToPDFParams, error) {
	params := page.PrintToPDFParams{
		PrintBackground:         true,
		Landscape:               false,
		MarginTop:               0,
		MarginBottom:            0,
		MarginLeft:              0,
		MarginRight:             0,
		Scale:                   1,
		GenerateDocumentOutline: false,
	}

	formatName := "A4"
	if data.Format != "" {
		formatName = data.Format
	}
	format, err := getFormat(formatName)
	if err != nil {
		return page.PrintToPDFParams{}, err
	}
	params.PaperWidth = format.Width
	params.PaperHeight = format.Height

	if data.Background != nil {
		params.PrintBackground = *data.Background
	}

	if data.Layout != "" {
		if !slices.Contains([]string{"landscape", "portrait"}, data.Layout) {
			return page.PrintToPDFParams{}, NewValidationError(fmt.Sprintf("invalid layout \"%s\", valid layouts are: landscape, portrait", data.Layout))
		}

		params.Landscape = data.Layout == "landscape"
	}

	if data.Margins != nil {
		params.MarginTop = data.Margins.Top
		params.MarginBottom = data.Margins.Bottom
		params.MarginLeft = data.Margins.Left
		params.MarginRight = data.Margins.Right
	}

	if data.Scale != 0 {
		if data.Scale < 0 {
			return page.PrintToPDFParams{}, NewValidationError("scale must be a positive decimal number")
		}

		params.Scale = data.Scale
	}

	return params, nil
}

// Get a buffer of bytes representing a webpage in PDF format.
func GetPDFBuffer(ctx context.Context, data GetPDFParams, res *[]byte) error {
	defer Elapsed("Total time to print PDF")()

	params, err := getPrintParams(data)
	if err != nil {
		return err
	}

	media := "print"
	if data.Media != "" {
		if !slices.Contains([]string{"screen", "print"}, data.Media) {
			return NewValidationError(fmt.Sprintf("invalid media value \"%s\", valid media values are: screen, print", data.Media))
		}

		media = data.Media
	}

	// NOTE: here we're using browserCtx instead of the one for this handler's invocation.
	tCtx, cancel := chromedp.NewContext(browserCtx)
	defer cancel()

	interactiveReached := false
	idleReached := false
	err = chromedp.Run(tCtx, chromedp.Tasks{
		chromedp.ActionFunc(func(ctx context.Context) error {
			defer Elapsed(fmt.Sprintf("Navigate to %s", data.Url))()

			if err := chromedp.Navigate(data.Url).Do(ctx); err != nil {
				return err
			}

			// Wait for both "InteractiveTime" and "networkIdle" events.
			ch := make(chan struct{})
			wCtx, cancel := context.WithCancel(ctx)
			chromedp.ListenTarget(wCtx, func(ev interface{}) {
				switch ev := ev.(type) {
				case *page.EventLifecycleEvent:
					if ev.Name == "InteractiveTime" {
						interactiveReached = true
					}
					if ev.Name == "networkIdle" {
						idleReached = true
					}
					if interactiveReached && idleReached {
						cancel()
						close(ch)
					}
				}
			})

			select {
			case <-ch:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			defer Elapsed("Export as PDF")()

			err := emulation.SetEmulatedMedia().WithMedia(media).Do(ctx)
			if err != nil {
				return err
			}

			buf, _, err := params.Do(ctx)
			if err != nil {
				return err
			}

			*res = buf

			return nil
		}),
	})
	if err != nil {
		return err
	}

	return nil
}
