package print2pdf

type PrintMargins struct {
	Top    float64 `json:"top,omitempty"`
	Bottom float64 `json:"bottom,omitempty"`
	Left   float64 `json:"left,omitempty"`
	Right  float64 `json:"right,omitempty"`
}

type RequestData struct {
	Url        string        `json:"url"`
	FileName   string        `json:"file_name"`
	Media      string        `json:"media,omitempty"`
	Format     string        `json:"format,omitempty"`
	Background *bool         `json:"background,omitempty"` // pointer to handle missing value from request (nil, false, true)
	Layout     string        `json:"layout,omitempty"`
	Margins    *PrintMargins `json:"margin,omitempty"` // pointer to handle missing value from request (nil, PrintMargins)
	Scale      float64       `json:"scale,omitempty"`
}

type ResponseData struct {
	Url string `json:"url"`
}
