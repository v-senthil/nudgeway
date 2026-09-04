package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
)

// maxMediaUploadBytes caps a media upload at 16 MiB. Meta's own limits
// vary by mime type (audio 16 MiB, video 16 MiB, image 5 MiB, document
// 100 MiB, sticker 500 KiB, animated sticker 500 KiB) — we pick the
// conservative floor that keeps everything a single-request upload.
// Larger files require Meta's resumable upload API (TODO).
const maxMediaUploadBytes int64 = 16 << 20

// uploadMediaResponse mirrors the Meta POST /{phone_number_id}/media
// success body. See ~/Documents/whatsapp_doc_tracker/docs/business-phone-numbers/media.md.
type uploadMediaResponse struct {
	ID string `json:"id"`
}

// uploadMedia POSTs r as a multipart form to /{phone_number_id}/media
// and returns the opaque Meta media_id. The multipart body carries:
//
//	messaging_product = whatsapp
//	type              = <contentType>            (e.g. "image/jpeg")
//	file              = <bytes streamed from r>  (Content-Type: <contentType>)
//
// filename is used as the multipart file name (falls back to
// "attachment" when empty). The upload is bounded at maxMediaUploadBytes;
// larger inputs return an error before touching the network.
//
// Ref: media.md (POST /{PHONE_NUMBER_ID}/media). Bearer auth on the
// standard Access Token. On success Meta returns {"id":"<MEDIA_ID>"}.
func (c *client) uploadMedia(ctx context.Context, contentType, filename string, r io.Reader) (string, error) {
	if c.cfg.PhoneNumberID == "" {
		return "", fmt.Errorf("whatsapp: uploadMedia: phone_number_id required")
	}
	if contentType == "" {
		return "", fmt.Errorf("whatsapp: uploadMedia: contentType required")
	}
	if filename == "" {
		filename = "attachment"
	}
	// Buffer to memory so we can compute Content-Length and retry on
	// transient failures without asking the caller to re-open the
	// stream. 16 MiB is small enough that a heap-buffered copy is not
	// a concern in the single-binary deploy.
	limited := io.LimitReader(r, maxMediaUploadBytes+1)
	fileBuf := &bytes.Buffer{}
	if _, err := io.Copy(fileBuf, limited); err != nil {
		return "", fmt.Errorf("whatsapp: uploadMedia: buffer body: %w", err)
	}
	if int64(fileBuf.Len()) > maxMediaUploadBytes {
		return "", fmt.Errorf("whatsapp: uploadMedia: file exceeds %d bytes", maxMediaUploadBytes)
	}

	body := &bytes.Buffer{}
	mp := multipart.NewWriter(body)
	if err := mp.WriteField("messaging_product", "whatsapp"); err != nil {
		return "", fmt.Errorf("whatsapp: uploadMedia: write messaging_product: %w", err)
	}
	if err := mp.WriteField("type", contentType); err != nil {
		return "", fmt.Errorf("whatsapp: uploadMedia: write type: %w", err)
	}
	// Custom part header so we can stamp Content-Type on the file part
	// (multipart.Writer.CreateFormFile hard-codes application/octet-stream).
	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filename))
	hdr.Set("Content-Type", contentType)
	filePart, err := mp.CreatePart(hdr)
	if err != nil {
		return "", fmt.Errorf("whatsapp: uploadMedia: create file part: %w", err)
	}
	if _, err := io.Copy(filePart, fileBuf); err != nil {
		return "", fmt.Errorf("whatsapp: uploadMedia: write file bytes: %w", err)
	}
	if err := mp.Close(); err != nil {
		return "", fmt.Errorf("whatsapp: uploadMedia: close multipart: %w", err)
	}

	url := fmt.Sprintf("%s/%s/%s/media", c.cfg.baseURL(), c.cfg.version(), c.cfg.PhoneNumberID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return "", fmt.Errorf("whatsapp: uploadMedia: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.AccessToken)
	req.Header.Set("Content-Type", mp.FormDataContentType())

	res, err := c.cfg.httpClient().Do(req)
	if err != nil {
		return "", &APIError{Class: ClassTransient, Message: err.Error()}
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return "", fmt.Errorf("whatsapp: uploadMedia: read body: %w", err)
	}
	if res.StatusCode >= 400 {
		return "", parseErrorResponse(res.StatusCode, raw, res.Header.Get("x-fb-trace-id"))
	}
	var out uploadMediaResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("whatsapp: uploadMedia: decode response: %w", err)
	}
	if strings.TrimSpace(out.ID) == "" {
		return "", fmt.Errorf("whatsapp: uploadMedia: empty media_id in response")
	}
	return out.ID, nil
}
