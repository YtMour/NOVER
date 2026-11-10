package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProductInputNormalizeAndValidate(t *testing.T) {
	input := productInput{Name: "  中文商品  ", Description: "  简介  ", Detail: "  第一段\n\n第二段  "}
	input.normalize()
	if input.Name != "中文商品" || input.Description != "简介" || input.Detail != "第一段\n\n第二段" {
		t.Fatalf("normalized input = %#v", input)
	}
	if !input.valid() {
		t.Fatal("valid Unicode product input was rejected")
	}

	input.Name = strings.Repeat("商", 121)
	if input.valid() {
		t.Fatal("product name longer than 120 characters was accepted")
	}
}

func TestCreateActiveProductRequiresDeliveryAsset(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/cp-7f3c/products", bytes.NewBufferString(`{"name":"Publish too early","description":"test","detail":"","status":"ACTIVE"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	(&App{}).createProduct(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusConflict, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"code":"delivery_asset_required"`) {
		t.Fatalf("response did not contain delivery_asset_required: %s", response.Body.String())
	}
}
