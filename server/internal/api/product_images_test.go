package api

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestInspectProductImage(t *testing.T) {
	canvas := image.NewRGBA(image.Rect(0, 0, 2, 2))
	canvas.Set(0, 0, color.RGBA{R: 30, G: 80, B: 120, A: 255})

	var pngData bytes.Buffer
	if err := png.Encode(&pngData, canvas); err != nil {
		t.Fatal(err)
	}
	var jpegData bytes.Buffer
	if err := jpeg.Encode(&jpegData, canvas, nil); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		data      []byte
		wantMIME  string
		wantExt   string
		wantError bool
	}{
		{name: "png", data: pngData.Bytes(), wantMIME: "image/png", wantExt: ".png"},
		{name: "jpeg", data: jpegData.Bytes(), wantMIME: "image/jpeg", wantExt: ".jpg"},
		{name: "other content", data: []byte("not an image"), wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mimeType, extension, err := inspectProductImage(test.data)
			if (err != nil) != test.wantError {
				t.Fatalf("inspectProductImage error = %v, want error %v", err, test.wantError)
			}
			if err == nil && (mimeType != test.wantMIME || extension != test.wantExt) {
				t.Fatalf("got (%q, %q), want (%q, %q)", mimeType, extension, test.wantMIME, test.wantExt)
			}
		})
	}
}
