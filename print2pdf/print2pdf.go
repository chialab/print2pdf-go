/*
Package print2pdf provides functions to save a webpage as a PDF file, leveraging Chromium and the DevTools Protocol.

Requires two environment variables to be set:
  - CHROMIUM_PATH, with the full path to the Chromium binary
  - BUCKET, with the name of the AWS S3 bucket

This packages uses init functions to initialize the AWS SDK and start an headless instance of Chromium, to reduce
startup time when used as a web service.
*/
package print2pdf

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
}
