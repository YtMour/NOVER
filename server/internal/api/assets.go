package api

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type asset struct {
	ID             string `json:"id"`
	ProductID      string `json:"product_id"`
	ReleaseID      string `json:"release_id,omitempty"`
	ReleaseVersion string `json:"release_version,omitempty"`
	FileName       string `json:"file_name"`
	MIMEType       string `json:"mime_type"`
	SizeBytes      int64  `json:"size_bytes"`
	SHA256         string `json:"sha256"`
}

func (a *App) uploadAsset(w http.ResponseWriter, r *http.Request) {
	if !a.allowRequest(r, "upload", 30, time.Minute) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Please wait before uploading more files.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, a.cfg.MaxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_upload", "Choose a file smaller than the configured upload limit.")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	files := r.MultipartForm.File["file"]
	if len(files) != 1 {
		writeError(w, http.StatusBadRequest, "file_required", "Upload exactly one file.")
		return
	}
	productID := chi.URLParam(r, "productID")
	releaseID := strings.TrimSpace(r.FormValue("release_id"))
	var exists bool
	if err := a.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM products WHERE id=$1 AND tenant_id=$2 AND status IN ('DRAFT', 'ACTIVE'))`, productID, requestTenantID(r)).Scan(&exists); err != nil {
		a.logError(r, "check product failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The file could not be uploaded.")
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "product_not_found", "Product was not found.")
		return
	}
	var releaseVersion string
	if releaseID != "" {
		err := a.db.QueryRow(r.Context(), `SELECT version FROM product_releases WHERE id=$1 AND product_id=$2 AND tenant_id=$3 AND status='DRAFT'`, releaseID, productID, requestTenantID(r)).Scan(&releaseVersion)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "release_not_uploadable", "Choose a draft release for this product before uploading the file.")
			return
		}
		if err != nil {
			a.logError(r, "check release for asset upload failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "The file could not be uploaded.")
			return
		}
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file_required", "Choose a file to upload.")
		return
	}
	defer file.Close()
	if header.Size > a.cfg.MaxUploadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "The file exceeds the configured upload limit.")
		return
	}
	name := filepath.Base(strings.ReplaceAll(header.Filename, "\\", "/"))
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 240 {
		writeError(w, http.StatusBadRequest, "invalid_file_name", "The file name must be between 1 and 240 characters.")
		return
	}
	id, err := randomID()
	if err != nil {
		a.logError(r, "create asset id failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The file could not be uploaded.")
		return
	}
	key, err := tenantObjectKey(requestTenantID(r), "assets", id+".blob")
	if err != nil {
		a.logError(r, "create asset storage key failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The file could not be uploaded.")
		return
	}
	destination := filepath.Join(a.cfg.StorageDir, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		a.logError(r, "create asset directory failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The file could not be uploaded.")
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".upload-*")
	if err != nil {
		a.logError(r, "create temporary file failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The file could not be uploaded.")
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	hash := sha256.New()
	limited := io.LimitReader(file, a.cfg.MaxUploadBytes+1)
	size, copyErr := io.Copy(io.MultiWriter(tmp, hash), limited)
	if copyErr != nil {
		tmp.Close()
		a.logError(r, "write uploaded file failed", copyErr)
		writeError(w, http.StatusInternalServerError, "internal_error", "The file could not be uploaded.")
		return
	}
	if size > a.cfg.MaxUploadBytes {
		tmp.Close()
		writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "The file exceeds the configured upload limit.")
		return
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		a.logError(r, "sync uploaded file failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The file could not be uploaded.")
		return
	}
	if err := tmp.Close(); err != nil {
		a.logError(r, "close uploaded file failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The file could not be uploaded.")
		return
	}
	mimeType := mime.TypeByExtension(filepath.Ext(name))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	if err := os.Rename(tmpPath, destination); err != nil {
		a.logError(r, "finalize uploaded file failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The file could not be uploaded.")
		return
	}
	item := asset{ID: id, ProductID: productID, ReleaseID: releaseID, ReleaseVersion: releaseVersion, FileName: name, MIMEType: mimeType, SizeBytes: size, SHA256: hex.EncodeToString(hash.Sum(nil))}
	tx, err := a.db.Begin(r.Context())
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO assets (id, tenant_id, product_id, release_id, object_key, file_name, mime_type, size_bytes, sha256) VALUES ($1, $2, $3, NULLIF($4,''), $5, $6, $7, $8, $9)`, item.ID, requestTenantID(r), item.ProductID, item.ReleaseID, key, item.FileName, item.MIMEType, item.SizeBytes, item.SHA256)
	}
	if err == nil {
		var eventID string
		eventID, err = randomID()
		if err == nil {
			_, err = tx.Exec(r.Context(), `INSERT INTO admin_events (id, tenant_id, actor, action, resource_type, resource_id, metadata_json) VALUES ($1,$2,$3,'ASSET_UPLOADED','asset',$4,jsonb_build_object('product_id',$5::text,'release_id',NULLIF($6::text,''),'file_name',$7::text,'size_bytes',$8::bigint))`, eventID, requestTenantID(r), operatorActor(r), item.ID, item.ProductID, item.ReleaseID, item.FileName, item.SizeBytes)
		}
	}
	if err == nil {
		err = tx.Commit(r.Context())
	} else if tx != nil {
		_ = tx.Rollback(r.Context())
	}
	if err != nil {
		_ = os.Remove(destination)
		a.logError(r, "save asset metadata failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The file could not be uploaded.")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (a *App) createDownloadTicket(w http.ResponseWriter, r *http.Request) {
	if !a.allowRequest(r, "ticket", 60, time.Minute) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Please wait before requesting another download.")
		return
	}
	token := bearerToken(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "delivery_auth_required", "Open your delivery link to continue.")
		return
	}
	assetID := chi.URLParam(r, "assetID")
	ticket, err := newOpaqueToken()
	if err != nil {
		a.logError(r, "create download ticket failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "A download link could not be created.")
		return
	}
	ticketID, err := randomID()
	if err != nil {
		a.logError(r, "create ticket id failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "A download link could not be created.")
		return
	}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.logError(r, "begin ticket transaction failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "A download link could not be created.")
		return
	}
	defer tx.Rollback(r.Context())
	var entitlementID, tenantID string
	var maxDownloads, downloadsIssued int
	var validUntil time.Time
	err = tx.QueryRow(r.Context(), `SELECT id, tenant_id, max_downloads, downloads_issued, valid_until FROM entitlements WHERE access_token_hash=$1 AND status='ACTIVE' FOR UPDATE`, hashToken(token)).Scan(&entitlementID, &tenantID, &maxDownloads, &downloadsIssued, &validUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnauthorized, "delivery_auth_invalid", "The delivery session is invalid or expired.")
		return
	}
	if err != nil {
		a.logError(r, "load entitlement for ticket failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "A download link could not be created.")
		return
	}
	if !validUntil.After(time.Now()) {
		writeError(w, http.StatusGone, "entitlement_expired", "This delivery period has ended.")
		return
	}
	if downloadsIssued >= maxDownloads {
		writeError(w, http.StatusTooManyRequests, "download_limit_reached", "The download limit for this item has been reached.")
		return
	}
	var exists bool
	err = tx.QueryRow(r.Context(), `SELECT EXISTS(
		SELECT 1 FROM assets a
		JOIN entitlements e ON e.id=$2 AND e.tenant_id=a.tenant_id AND e.product_id=a.product_id
		LEFT JOIN product_releases base ON base.id=e.release_id AND base.tenant_id=e.tenant_id
		LEFT JOIN product_releases ar ON ar.id=a.release_id AND ar.tenant_id=a.tenant_id
		WHERE a.id=$1 AND (a.release_id IS NULL OR (ar.status='PUBLISHED' AND (e.release_id IS NULL OR ar.published_at>=base.published_at)))
	)`, assetID, entitlementID).Scan(&exists)
	if err != nil {
		a.logError(r, "check asset entitlement failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "A download link could not be created.")
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "asset_not_found", "This file is not available in your delivery.")
		return
	}
	expires := time.Now().Add(time.Duration(a.cfg.DownloadTTLSeconds) * time.Second)
	_, err = tx.Exec(r.Context(), `INSERT INTO download_tickets (id, tenant_id, entitlement_id, asset_id, token_hash, expires_at) VALUES ($1, $2, $3, $4, $5, $6)`, ticketID, tenantID, entitlementID, assetID, hashToken(ticket), expires)
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE entitlements SET downloads_issued=downloads_issued+1 WHERE id=$1 AND tenant_id=$2`, entitlementID, tenantID)
	}
	if err != nil {
		a.logError(r, "save download ticket failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "A download link could not be created.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.logError(r, "commit ticket transaction failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "A download link could not be created.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"url": "/api/v1/downloads/" + ticket, "expires_at": expires})
}

func (a *App) download(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	var item asset
	var entitlementID, objectKey, tenantID string
	var expires time.Time
	err := a.db.QueryRow(r.Context(), `SELECT a.id, a.product_id, a.file_name, a.mime_type, a.size_bytes, a.sha256, a.object_key, e.id, t.tenant_id, t.expires_at FROM download_tickets t JOIN assets a ON a.id=t.asset_id AND a.tenant_id=t.tenant_id JOIN entitlements e ON e.id=t.entitlement_id AND e.tenant_id=t.tenant_id WHERE t.token_hash=$1 AND e.status='ACTIVE' AND e.valid_until>NOW()`, hashToken(token)).Scan(&item.ID, &item.ProductID, &item.FileName, &item.MIMEType, &item.SizeBytes, &item.SHA256, &objectKey, &entitlementID, &tenantID, &expires)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !expires.After(time.Now())) {
		writeError(w, http.StatusGone, "download_link_expired", "This download link has expired. Request a new link from your delivery page.")
		return
	}
	if err != nil {
		a.logError(r, "load download ticket failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The file could not be downloaded.")
		return
	}
	storagePath, validPath := safeStoragePath(a.cfg.StorageDir, objectKey)
	if !validPath {
		writeError(w, http.StatusInternalServerError, "asset_path_invalid", "The file could not be downloaded.")
		return
	}
	file, err := os.Open(storagePath)
	if err != nil {
		a.logError(r, "open asset failed", err)
		writeError(w, http.StatusNotFound, "asset_unavailable", "The file is temporarily unavailable.")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		if err != nil {
			a.logError(r, "inspect asset failed", err)
		}
		writeError(w, http.StatusNotFound, "asset_unavailable", "The file is temporarily unavailable.")
		return
	}
	eventID, err := randomID()
	if err != nil {
		a.logError(r, "create download event id failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The file could not be downloaded.")
		return
	}
	if _, err := a.db.Exec(r.Context(), `INSERT INTO download_events (id, tenant_id, entitlement_id, asset_id, result) VALUES ($1, $2, $3, $4, 'STARTED')`, eventID, tenantID, entitlementID, item.ID); err != nil {
		a.logError(r, "record download event failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The file could not be downloaded.")
		return
	}
	w.Header().Set("Content-Type", item.MIMEType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": item.FileName}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, item.FileName, info.ModTime(), file)
}
