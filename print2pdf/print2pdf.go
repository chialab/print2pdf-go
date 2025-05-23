/*
Package print2pdf provides functions to save a webpage as a PDF file, leveraging Chromium and the DevTools Protocol.

Requires the environment variable CHROMIUM_PATH to be set with the full path to the Chromium binary.

The StartBrowser() function starts a headless instance of Chromium, to reduce startup time in long running services (like a web server),
and therefore must be called before any call PrintPDF(). These functions can (and probably should) use different contexts: the one passed
to StartBrowser() closes the whole browser when done or cancelled, while the one passed to PrintPDF() closes only the tab it uses.
*/
package print2pdf

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"maps"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	chromedpio "github.com/chromedp/cdproto/io"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// Page margins of the generated PDF, in inches.
type PrintMargins struct {
	Top    float64 `json:"top,omitempty"`
	Bottom float64 `json:"bottom,omitempty"`
	Left   float64 `json:"left,omitempty"`
	Right  float64 `json:"right,omitempty"`
}

// Parameters for generating a PDF.
type GetPDFParams struct {
	// URL of the webpage to save. Required.
	Url string `json:"url"`
	// Filename of the generated PDF. A ".pdf" suffix will be appended if not present. Required.
	FileName string `json:"file_name"`
	// Media type to emulate. Accepted values are "print" and "screen". Default is "print".
	Media string `json:"media,omitempty"`
	// Page format. See FormatsMap for accepted values. Default is "A4".
	Format string `json:"format,omitempty"`
	// Print background graphics. Default is true.
	Background *bool `json:"background,omitempty"`
	// Page orientation. Accepted values are "landscape" and "portrait". Default is "portrait".
	Layout string `json:"layout,omitempty"`
	// Page margins in inches. Default is all 0.
	Margins *PrintMargins `json:"margin,omitempty"`
	// Scale of the webpage rendering. Default is 1.
	Scale float64 `json:"scale,omitempty"`
	// Cookies forwarded from request to URL. Default is empty.
	Cookies map[string]string `json:"-"`
}

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

// StreamHandleReader is a helper to read a StreamHandle returned by chromedp when printing a web page to PDF with "ReturnAsStream" transfer mode.
// For more information about the protocol, see:
//   - https://chromedevtools.github.io/devtools-protocol/tot/Page/#method-printToPDF
//   - https://chromedevtools.github.io/devtools-protocol/tot/IO/
type StreamHandleReader struct {
	c context.Context         // Context.
	h chromedpio.StreamHandle // Stream handle.
	r *chromedpio.ReadParams  // Read params.
}

// NewStreamHandleReader returns a new insance of StreamHandleReader.
func NewStreamHandleReader(ctx context.Context, h chromedpio.StreamHandle) *StreamHandleReader {
	r := StreamHandleReader{
		c: ctx,
		h: h,
	}
	r.r = chromedpio.Read(r.h)

	return &r
}

// Implement io.Reader interface.
func (r *StreamHandleReader) Read(p []byte) (int, error) {
	if err := r.c.Err(); err != nil {
		return 0, fmt.Errorf("parent context closed: %s", err)
	}

	r.r.Size = int64(len(p))
	data, eof, err := r.r.Do(r.c)
	if err != nil {
		return 0, err
	}

	if len(data) == 0 && eof {
		return 0, io.EOF
	}

	dec, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return 0, err
	}

	n := copy(p, dec)
	r.r.Offset += int64(n)
	if eof {
		return n, io.EOF
	}

	return n, nil
}

// Implement io.Closer interface.
func (r *StreamHandleReader) Close() error {
	return chromedpio.Close(r.h).Do(r.c)
}

// Chromium binary path. Required.
var ChromiumPath = os.Getenv("CHROMIUM_PATH")

// Reference to browser context, initialized in init function of this package.
var browserCtx context.Context

// Allocate a browser to be reused by multiple invocations, to reduce startup time. Cancelling the context will close the browser.
// This function must be called before starting to print PDFs.
func StartBrowser(ctx context.Context) error {
	if Running() {
		return nil
	}
	if ChromiumPath == "" {
		return fmt.Errorf("missing required environment variable CHROMIUM_PATH")
	}

	defer Elapsed("Browser startup")()
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(ChromiumPath))
	allocatorCtx, _ := chromedp.NewExecAllocator(ctx, opts...)
	browserCtx, _ = chromedp.NewContext(allocatorCtx)

	// Navigate to blank page so that the browser is started.
	err := chromedp.Run(browserCtx, chromedp.Tasks{chromedp.Navigate("about:blank")})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error initializing browser: %v", err)

		return err
	}

	return nil
}

// Check if the browser is still running.
func Running() bool {
	return browserCtx != nil && browserCtx.Err() == nil
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
		MarginTop:               0.4,
		MarginBottom:            0.4,
		MarginLeft:              0.4,
		MarginRight:             0.4,
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

// Print a webpage in PDF format and write the result to the input handler. Cancelling the context will close the tab.
// StartBrowser() must have been called once before calling this function.
func PrintPDF(ctx context.Context, data GetPDFParams, h PDFHandler) (string, error) {
	if browserCtx == nil {
		return "", fmt.Errorf("must call StartBrowser() before printing a PDF")
	}

	defer Elapsed("Total time to print PDF")()

	params, err := getPrintParams(data)
	if err != nil {
		return "", err
	}

	media := "print"
	if data.Media != "" {
		if !slices.Contains([]string{"screen", "print"}, data.Media) {
			return "", NewValidationError(fmt.Sprintf("invalid media value \"%s\", valid media values are: screen, print", data.Media))
		}

		media = data.Media
	}

	tabCtx, tabCancel := chromedp.NewContext(browserCtx, chromedp.WithNewBrowserContext())
	defer tabCancel()
	// Cancel the tab context (closing the tab) if the passed context is canceled.
	context.AfterFunc(ctx, tabCancel)

	interactiveReached := false
	idleReached := false
	res := ""
	err = chromedp.Run(tabCtx, chromedp.Tasks{
		chromedp.ActionFunc(func(ctx context.Context) error {
			defer Elapsed(fmt.Sprintf("Forward cookies (%s)", slices.Collect(maps.Keys(data.Cookies))))()

			u, err := url.Parse(data.Url)
			if err != nil {
				return fmt.Errorf("parsing URL error: %w", err)
			}

			for name, value := range data.Cookies {
				expr := cdp.TimeSinceEpoch(time.Now().Add(180 * 24 * time.Hour))

				err := network.SetCookie(name, value).
					WithExpires(&expr).
					WithDomain(u.Hostname()).
					WithPath("/").
					WithHTTPOnly(true).
					WithSecure(false).
					Do(ctx)
				if err != nil {
					return fmt.Errorf("failed to set cookie %s: %w", name, err)
				}
			}

			return nil
		}),
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

			_, stream, err := params.WithTransferMode(page.PrintToPDFTransferModeReturnAsStream).Do(ctx)
			if err != nil {
				return err
			}

			sh := NewStreamHandleReader(ctx, stream)
			res, err = h.Handle(sh)
			if err != nil {
				return err
			}
			if err := sh.Close(); err != nil {
				return err
			}

			return nil
		}),
	})
	if err != nil {
		return "", err
	}

	return res, nil
}
