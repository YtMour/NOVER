package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

type product struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

func (a *App) listProducts(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `SELECT id, name, description, status FROM products ORDER BY created_at DESC`)
	if err != nil {
		a.logError(r, "list products failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Products could not be loaded.")
		return
	}
	defer rows.Close()
	products := make([]product, 0)
	for rows.Next() {
		var item product
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.Status); err != nil {
			a.logError(r, "scan product failed", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Products could not be loaded.")
			return
		}
		products = append(products, item)
	}
	if err := rows.Err(); err != nil {
		a.logError(r, "iterate products failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Products could not be loaded.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": products})
}

func (a *App) createProduct(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Enter a valid product request.")
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if input.Name == "" || len(input.Name) > 120 || len(input.Description) > 2000 {
		writeError(w, http.StatusBadRequest, "invalid_product", "Product name is required and must be at most 120 characters.")
		return
	}
	id, err := randomID()
	if err != nil {
		a.logError(r, "create product id failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The product could not be created.")
		return
	}
	item := product{ID: id, Name: input.Name, Description: input.Description, Status: "ACTIVE"}
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		a.logError(r, "begin create product failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The product could not be created.")
		return
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), `INSERT INTO products (id, name, description) VALUES ($1, $2, $3)`, item.ID, item.Name, item.Description)
	if err == nil {
		var eventID string
		eventID, err = randomID()
		if err == nil {
			_, err = tx.Exec(r.Context(), `INSERT INTO admin_events (id, action, resource_type, resource_id) VALUES ($1, 'PRODUCT_CREATED', 'product', $2)`, eventID, item.ID)
		}
	}
	if err == nil {
		err = tx.Commit(r.Context())
	}
	if err != nil {
		a.logError(r, "create product failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "The product could not be created.")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func productID(r *http.Request) string {
	return chi.URLParam(r, "productID")
}
