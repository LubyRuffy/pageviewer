package pageviewer

import "net/http"

type TextResponse struct {
	Body        string
	ContentType string
	StatusCode  int
	FinalURL    string
	Header      http.Header
}
