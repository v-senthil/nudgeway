package v1

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/fullwa/fullwa/internal/infrastructure/http/middleware"
	"github.com/fullwa/fullwa/internal/ports/attachments"
)

// MaxAttachmentSize is the largest media blob accepted via the multipart
// upload endpoint. Beyond this Meta requires the resumable upload API
// (see docs/providers/whatsapp.md — TODO).
const MaxAttachmentSize int64 = 16 << 20 // 16 MiB

// AttachmentsUploadDeps bundles the state POST /api/v1/attachments needs.
// Provided by cmd/server at wire-up time. When Store is nil the route is
// not mounted.
type AttachmentsUploadDeps struct {
	// Store is the port implementation the handler writes blobs through.
	// A nil Store disables the endpoint (route not mounted).
	Store attachments.Store
	// MediaUploader is optional: when non-nil, after storing bytes locally
	// the handler asks the uploader to hand the same bytes to the provider
	// (Meta's Media Upload API). The returned media_id is stashed
	// alongside the attachment and returned in the response so the
	// composer can send with media:{id:<mid>} — Meta prefers a first-
	// class handle over a re-fetch of a URL.
	MediaUploader MediaUploader
	// PublicBaseURL is the externally-reachable origin (e.g.
	// "https://app.example.com"). It is prepended to the returned
	// media_url so operator-side responses are self-contained.
	// Trailing slashes are stripped.
	PublicBaseURL string
	// Logger receives one structured record per failed upload.
	Logger *slog.Logger
}

// MediaUploader hands a stored attachment to the provider and returns the
// provider-native handle (Meta media_id). Wire-up owns which integration
// the upload targets — the handler is provider-agnostic.
type MediaUploader interface {
	Upload(ctx context.Context, orgID, attachmentKey, contentType, filename string) (providerKey, integrationID, mediaID string, err error)
}

// AttachmentUploadResponse is the 201 body of POST /api/v1/attachments.
// media_url is the fully-qualified URL the send worker can hand to Meta
// under the `link` field of an image/video/audio/document/sticker payload.
// media_id is set when the wire-up MediaUploader successfully handed the
// blob to the provider — the composer prefers it over media_url.
type AttachmentUploadResponse struct {
	AttachmentID string `json:"attachment_id"`
	MediaURL     string `json:"media_url"`
	MediaID      string `json:"media_id,omitempty"`
	Provider     string `json:"provider,omitempty"`
	Size         int64  `json:"size"`
	ContentType  string `json:"content_type"`
	Filename     string `json:"filename,omitempty"`
}

// mountAttachmentsUpload installs POST /api/v1/attachments on mux.
// The route is auth + CSRF gated; the wire-up passes the same `authed`
// chain builder used by POST /api/v1/messages.
func mountAttachmentsUpload(mux Registrar, authed func(http.Handler) http.Handler, deps AttachmentsUploadDeps) {
	if deps.Store == nil {
		return
	}
	h := &attachmentsHandler{d: deps}
	mux.Handle("POST /api/v1/attachments", authed(http.HandlerFunc(h.upload)))
}

// attachmentsHandler carries per-endpoint state.
type attachmentsHandler struct{ d AttachmentsUploadDeps }

// upload handles POST /api/v1/attachments.
//
// Contract: multipart/form-data with a single `file` field. The handler
// enforces the MaxAttachmentSize cap, streams the blob into the
// attachments.Store, and returns the content-addressed key plus a
// self-contained media URL. Only auth + CSRF-authenticated principals
// reach here (the route builder attaches both middlewares).
func (h *attachmentsHandler) upload(w http.ResponseWriter, r *http.Request) {
	pr, ok := middleware.PrincipalFrom(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "unauthenticated", "session required")
		return
	}

	// Enforce the cap on the raw body before the multipart parser
	// allocates. +1KiB slack covers multipart framing overhead.
	r.Body = http.MaxBytesReader(w, r.Body, MaxAttachmentSize+(1<<10))

	if err := r.ParseMultipartForm(MaxAttachmentSize + (1 << 10)); err != nil {
		var mbErr *http.MaxBytesError
		if errors.As(err, &mbErr) {
			writeProblem(w, r, http.StatusRequestEntityTooLarge, "attachment_too_large",
				fmt.Sprintf("attachment exceeds %d bytes", MaxAttachmentSize))
			return
		}
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "invalid multipart body")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "validation", "missing file field")
		return
	}
	defer file.Close()

	if header.Size > MaxAttachmentSize {
		writeProblem(w, r, http.StatusRequestEntityTooLarge, "attachment_too_large",
			fmt.Sprintf("attachment exceeds %d bytes", MaxAttachmentSize))
		return
	}

	// Sniff content-type when the client didn't send one. We buffer the
	// first 512 bytes for detection then hand a concatenated reader to
	// the Store so nothing is lost.
	contentType := headerContentType(header)
	var body io.Reader = file
	if contentType == "" {
		var buf [512]byte
		n, _ := io.ReadFull(file, buf[:])
		contentType = http.DetectContentType(buf[:n])
		body = io.MultiReader(bytes.NewReader(buf[:n]), file)
	}

	// Cap the stream a second time so a lying Content-Length can't push
	// us past the ceiling once we're inside the Store.
	body = io.LimitReader(body, MaxAttachmentSize)

	key, size, _, err := h.d.Store.Put(r.Context(), contentType, body)
	if err != nil {
		h.logger().Warn("attachment put failed",
			slog.String("request_id", middleware.RequestIDFrom(r.Context())),
			slog.String("org_id", pr.OrgID),
			slog.String("filename", header.Filename),
			slog.Any("err", err),
		)
		writeProblem(w, r, http.StatusInternalServerError, "internal", "attachment store failed")
		return
	}

	// Best-effort Meta Media Upload: hand the bytes to the provider so
	// subsequent sends can reference media_id (Meta prefers a first-class
	// handle over re-fetching a URL). Failures here don't fail the local
	// store — media_url will still work as a fallback.
	var providerKey, mediaID string
	if h.d.MediaUploader != nil {
		p, _, mid, upErr := h.d.MediaUploader.Upload(r.Context(), pr.OrgID, key, contentType, header.Filename)
		if upErr != nil {
			h.logger().Warn("meta media upload failed; local store OK",
				slog.String("request_id", middleware.RequestIDFrom(r.Context())),
				slog.String("org_id", pr.OrgID),
				slog.String("key", key),
				slog.Any("err", upErr),
			)
		} else {
			providerKey = p
			mediaID = mid
		}
	}

	writeJSON(w, http.StatusCreated, AttachmentUploadResponse{
		AttachmentID: key,
		MediaURL:     h.mediaURL(key),
		MediaID:      mediaID,
		Provider:     providerKey,
		Size:         size,
		ContentType:  contentType,
		Filename:     header.Filename,
	})
}

// mediaURL builds the externally-reachable URL for a stored blob.
// Format: <public_base_url>/api/v1/media/<key>. When PublicBaseURL is
// unset (e.g. local dev without a canonical origin) the path is returned
// bare — still valid inside the SPA that shares the origin.
func (h *attachmentsHandler) mediaURL(key string) string {
	path := "/api/v1/media/" + key
	base := strings.TrimRight(h.d.PublicBaseURL, "/")
	if base == "" {
		return path
	}
	return base + path
}

func (h *attachmentsHandler) logger() *slog.Logger {
	if h.d.Logger != nil {
		return h.d.Logger
	}
	return slog.Default()
}

// headerContentType returns the client-declared Content-Type of a
// multipart part, or the empty string when absent. Callers fall back to
// http.DetectContentType when this returns "".
func headerContentType(h *multipart.FileHeader) string {
	if h == nil {
		return ""
	}
	return h.Header.Get("Content-Type")
}
