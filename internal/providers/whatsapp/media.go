package whatsapp

import (
	"context"
	"fmt"
	"io"
)

// Media is the metadata returned by a media lookup, plus the download stream.
type Media struct {
	ID          string
	URL         string
	MIMEType    string
	SHA256      string
	FileSize    int64
	Body        io.ReadCloser
	ContentType string
}

// DownloadMedia resolves a Meta media ID to a short-lived download URL, then
// streams the bytes. Caller MUST close Body.
func (p *Provider) DownloadMedia(ctx context.Context, mediaID string) (*Media, error) {
	if mediaID == "" {
		return nil, fmt.Errorf("whatsapp: DownloadMedia: mediaID required")
	}
	lookup, err := p.client.getMediaURL(ctx, mediaID)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: lookup media url: %w", err)
	}
	body, ctype, err := p.client.downloadMedia(ctx, lookup.URL)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: download media bytes: %w", err)
	}
	return &Media{
		ID:          lookup.ID,
		URL:         lookup.URL,
		MIMEType:    lookup.MimeType,
		SHA256:      lookup.SHA256,
		FileSize:    lookup.FileSize,
		Body:        body,
		ContentType: ctype,
	}, nil
}
