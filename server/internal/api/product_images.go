package api

import (
	"bytes"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

const maxProductImageBytes = 8 << 20

func inspectProductImage(data []byte) (string, string, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png") || config.Width < 1 || config.Height < 1 || config.Width > 8192 || config.Height > 8192 || int64(config.Width)*int64(config.Height) > 40_000_000 {
		return "", "", errors.New("unsupported image or dimensions")
	}
	if format == "png" {
		return "image/png", ".png", nil
	}
	return "image/jpeg", ".jpg", nil
}

func (a *App) uploadProductImage(w http.ResponseWriter, r *http.Request) {
	if !a.allowRequest(r, "product-image-upload", 30, time.Minute) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Please wait before uploading more images.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxProductImageBytes+(1<<20))
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_upload", "Choose a JPEG or PNG image smaller than 8 MB.")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if len(r.MultipartForm.File["file"]) != 1 {
		writeError(w, http.StatusBadRequest, "image_required", "Upload exactly one image.")
		return
	}
	productID := chi.URLParam(r, "productID")
	var productName string
	if err := a.db.QueryRow(r.Context(), `SELECT name FROM products WHERE id=$1 AND tenant_id=$2 AND status IN ('DRAFT', 'ACTIVE')`, productID, requestTenantID(r)).Scan(&productName); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "product_not_found", "Product was not found.")
		return
	} else if err != nil {
		a.logError(r, "check product for image upload failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The image could not be uploaded.")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "image_required", "Choose an image to upload.")
		return
	}
	defer file.Close()
	if header.Size <= 0 || header.Size > maxProductImageBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "image_too_large", "Images must be smaller than 8 MB.")
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, maxProductImageBytes+1))
	if err != nil || len(data) > maxProductImageBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "image_too_large", "Images must be smaller than 8 MB.")
		return
	}
	mimeType, extension, err := inspectProductImage(data)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_image", "Upload a valid JPEG or PNG image up to 8192 pixels per side.")
		return
	}
	altText := strings.TrimSpace(r.FormValue("alt_text"))
	if altText == "" {
		altText = productName
	}
	if len(altText) > 240 {
		writeError(w, http.StatusBadRequest, "invalid_alt_text", "Image description must be at most 240 characters.")
		return
	}
	id, err := randomID()
	if err != nil {
		a.logError(r, "create product image id failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The image could not be uploaded.")
		return
	}
	key, err := tenantObjectKey(requestTenantID(r), "product-images", id+extension)
	if err != nil {
		a.logError(r, "create product image storage key failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The image could not be uploaded.")
		return
	}
	destination := filepath.Join(a.cfg.StorageDir, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		a.logError(r, "create product image directory failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The image could not be uploaded.")
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".image-*")
	if err != nil {
		a.logError(r, "create product image temporary file failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The image could not be uploaded.")
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		a.logError(r, "write product image failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The image could not be uploaded.")
		return
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		a.logError(r, "sync product image failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The image could not be uploaded.")
		return
	}
	if err := tmp.Close(); err != nil {
		a.logError(r, "close product image failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The image could not be uploaded.")
		return
	}
	if err := os.Rename(tmpPath, destination); err != nil {
		a.logError(r, "finalize product image failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The image could not be uploaded.")
		return
	}
	item := productImage{ID: id, URL: "/api/v1/products/" + productID + "/images/" + id, AltText: altText, MIMEType: mimeType, SizeBytes: int64(len(data))}
	tx, err := a.db.Begin(r.Context())
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO product_images (id, tenant_id, product_id, object_key, alt_text, mime_type, size_bytes) VALUES ($1, $2, $3, $4, $5, $6, $7)`, item.ID, requestTenantID(r), productID, key, item.AltText, item.MIMEType, item.SizeBytes)
	}
	if err == nil {
		var eventID string
		eventID, err = randomID()
		if err == nil {
			_, err = tx.Exec(r.Context(), `INSERT INTO admin_events (id, tenant_id, actor, action, resource_type, resource_id, metadata_json) VALUES ($1,$2,$3,'PRODUCT_IMAGE_UPLOADED','product_image',$4,jsonb_build_object('product_id',$5::text,'size_bytes',$6::bigint))`, eventID, requestTenantID(r), operatorActor(r), item.ID, productID, item.SizeBytes)
		}
	}
	if err == nil {
		err = tx.Commit(r.Context())
	} else if tx != nil {
		_ = tx.Rollback(r.Context())
	}
	if err != nil {
		_ = os.Remove(destination)
		a.logError(r, "save product image metadata failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The image could not be uploaded.")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (a *App) listProductImages(w http.ResponseWriter, r *http.Request) {
	productID := chi.URLParam(r, "productID")
	var exists bool
	if err := a.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM products WHERE id=$1 AND tenant_id=$2)`, productID, requestTenantID(r)).Scan(&exists); err != nil {
		a.logError(r, "check product for images failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Images could not be loaded.")
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "product_not_found", "Product was not found.")
		return
	}
	rows, err := a.db.Query(r.Context(), `SELECT id, alt_text, mime_type, size_bytes FROM product_images WHERE product_id=$1 AND tenant_id=$2 ORDER BY created_at`, productID, requestTenantID(r))
	if err != nil {
		a.logError(r, "list product images failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Images could not be loaded.")
		return
	}
	defer rows.Close()
	items := make([]productImage, 0)
	for rows.Next() {
		var item productImage
		if err := rows.Scan(&item.ID, &item.AltText, &item.MIMEType, &item.SizeBytes); err != nil {
			a.logError(r, "scan product image failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Images could not be loaded.")
			return
		}
		item.URL = "/api/v1/products/" + productID + "/images/" + item.ID
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		a.logError(r, "iterate product images failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Images could not be loaded.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *App) deleteProductImage(w http.ResponseWriter, r *http.Request) {
	productID, imageID := chi.URLParam(r, "productID"), chi.URLParam(r, "imageID")
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.logError(r, "begin product image delete failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The image could not be removed.")
		return
	}
	defer tx.Rollback(r.Context())
	var key string
	err = tx.QueryRow(r.Context(), `DELETE FROM product_images WHERE id=$1 AND product_id=$2 AND tenant_id=$3 RETURNING object_key`, imageID, productID, requestTenantID(r)).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "image_not_found", "Image was not found for this product.")
		return
	}
	if err != nil {
		a.logError(r, "delete product image row failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The image could not be removed.")
		return
	}
	eventID, err := randomID()
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO admin_events (id, tenant_id, actor, action, resource_type, resource_id, metadata_json) VALUES ($1,$2,$3,'PRODUCT_IMAGE_DELETED','product_image',$4,jsonb_build_object('product_id',$5::text))`, eventID, requestTenantID(r), operatorActor(r), imageID, productID)
	}
	if err != nil {
		a.logError(r, "audit product image delete failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The image could not be removed.")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		a.logError(r, "commit product image delete failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The image could not be removed.")
		return
	}
	storagePath, validPath := safeStoragePath(a.cfg.StorageDir, key)
	if !validPath {
		a.logger.Error("invalid product image storage path", "image_id", imageID)
	} else if err := os.Remove(storagePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		a.logger.Error("remove product image file failed", "image_id", imageID, "error", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) serveProductImage(w http.ResponseWriter, r *http.Request) {
	var key, mimeType string
	err := a.db.QueryRow(r.Context(), `SELECT pi.object_key, pi.mime_type FROM product_images pi JOIN products p ON p.id=pi.product_id AND p.tenant_id=pi.tenant_id WHERE pi.id=$1 AND pi.product_id=$2 AND pi.tenant_id=$3 AND p.status='ACTIVE'`, chi.URLParam(r, "imageID"), chi.URLParam(r, "productID"), defaultTenantID).Scan(&key, &mimeType)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "image_not_found", "Image was not found.")
		return
	}
	if err != nil {
		a.logError(r, "load public product image failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The image could not be loaded.")
		return
	}
	if _, validPath := safeStoragePath(a.cfg.StorageDir, key); !validPath {
		writeError(w, http.StatusInternalServerError, "image_path_invalid", "The image could not be loaded.")
		return
	}
	a.serveStoredProductImage(w, r, key, mimeType, true)
}

func (a *App) serveOperatorProductImage(w http.ResponseWriter, r *http.Request) {
	var key, mimeType string
	err := a.db.QueryRow(r.Context(), `SELECT object_key, mime_type FROM product_images WHERE id=$1 AND product_id=$2 AND tenant_id=$3`, chi.URLParam(r, "imageID"), chi.URLParam(r, "productID"), requestTenantID(r)).Scan(&key, &mimeType)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "image_not_found", "Image was not found.")
		return
	}
	if err != nil {
		a.logError(r, "load operator product image failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The image could not be loaded.")
		return
	}
	a.serveStoredProductImage(w, r, key, mimeType, false)
}

func (a *App) serveStoredProductImage(w http.ResponseWriter, r *http.Request, key, mimeType string, public bool) {
	storagePath, validPath := safeStoragePath(a.cfg.StorageDir, key)
	if !validPath {
		writeError(w, http.StatusInternalServerError, "image_path_invalid", "The image could not be loaded.")
		return
	}
	file, err := os.Open(storagePath)
	if err != nil {
		writeError(w, http.StatusNotFound, "image_unavailable", "The image is temporarily unavailable.")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		writeError(w, http.StatusNotFound, "image_unavailable", "The image is temporarily unavailable.")
		return
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if public {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	} else {
		w.Header().Set("Cache-Control", "private, no-store")
	}
	http.ServeContent(w, r, filepath.Base(key), info.ModTime(), file)
}
