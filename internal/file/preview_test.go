package file

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"testing"

	"github.com/gen2brain/avif"
)

func TestJPEGPreviewAppliesEXIFOrientation(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 2, 1))
	source.Set(0, 0, color.RGBA{R: 255, A: 255})
	source.Set(1, 0, color.RGBA{B: 255, A: 255})
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, source, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	data := appendJPEGOrientation(encoded.Bytes(), 6)
	preview, _, err := makeStaticPreview(uploadHeader(t, "phone.jpg", data), ".jpg")
	if err != nil {
		t.Fatalf("make preview: %v", err)
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(preview))
	if err != nil {
		t.Fatal(err)
	}
	if config.Width != 1 || config.Height != 2 {
		t.Fatalf("preview dimensions = %dx%d, want 1x2", config.Width, config.Height)
	}
}

func TestHEICAndAVIFGenerateStaticJPEGPreview(t *testing.T) {
	heicData, err := base64.StdEncoding.DecodeString(testHEICBase64)
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"phone.heic": heicData, "phone.heif": heicData} {
		preview, mimeType, err := makeStaticPreview(uploadHeader(t, name, data), extension(name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		assertJPEGPreview(t, preview, mimeType)
	}

	var encoded bytes.Buffer
	if err := avif.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatalf("encode avif fixture: %v", err)
	}
	preview, mimeType, err := makeStaticPreview(uploadHeader(t, "phone.avif", encoded.Bytes()), ".avif")
	if err != nil {
		t.Fatalf("avif: %v", err)
	}
	assertJPEGPreview(t, preview, mimeType)
}

func TestPreviewSafetyLimitsFailBeforeDecode(t *testing.T) {
	if err := validatePixelSize(8000, 4001); err == nil {
		t.Fatal("expected 32 megapixel limit")
	}
	opened := false
	source := imageSource{size: maxWASMSourceBytes + 1, open: func() (io.ReadCloser, error) {
		opened = true
		return nil, errors.New("should not open")
	}}
	if _, _, err := makeStaticPreviewSource(source, ".heic"); err == nil || opened {
		t.Fatalf("HEIC source limit err=%v opened=%v", err, opened)
	}
	source.size = maxSVGSourceBytes + 1
	if _, _, err := makeStaticPreviewSource(source, ".svg"); err == nil || opened {
		t.Fatalf("SVG source limit err=%v opened=%v", err, opened)
	}
}

func assertJPEGPreview(t *testing.T, preview []byte, mimeType string) {
	t.Helper()
	if mimeType != "image/jpeg" || len(preview) == 0 {
		t.Fatalf("preview mime=%q bytes=%d", mimeType, len(preview))
	}
	if _, err := jpeg.Decode(bytes.NewReader(preview)); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
}

func appendJPEGOrientation(jpegData []byte, orientation byte) []byte {
	tiff := []byte{
		'I', 'I', 0x2a, 0x00, 0x08, 0x00, 0x00, 0x00,
		0x01, 0x00,
		0x12, 0x01, 0x03, 0x00, 0x01, 0x00, 0x00, 0x00, orientation, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
	payload := append([]byte{'E', 'x', 'i', 'f', 0x00, 0x00}, tiff...)
	segmentLength := len(payload) + 2
	segment := []byte{0xff, 0xe1, byte(segmentLength >> 8), byte(segmentLength)}
	segment = append(segment, payload...)
	result := append([]byte{}, jpegData[:2]...)
	result = append(result, segment...)
	return append(result, jpegData[2:]...)
}

func extension(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '.' {
			return name[i:]
		}
	}
	return ""
}

const testHEICBase64 = "AAAAHGZ0eXBoZWljAAAAAG1pZjFoZWljbWlhZgAAAY5tZXRhAAAAAAAAACFoZGxyAAAAAAAAAABwaWN0AAAAAAAAAAAAAAAAAAAAAA5waXRtAAAAAAABAAAANGlsb2MAAAAAREAAAgABAAAAAAGyAAEAAAAAAAAB1AACAAAAAAOGAAEAAAAAAAAA5AAAADhpaW5mAAAAAAACAAAAFWluZmUCAAAAAAEAAGh2YzEAAAAAFWluZmUCAAABAAIAAEV4aWYAAAAAzWlwcnAAAACuaXBjbwAAAHlodmNDAQNwAAAAAAAAAAAAWvAA/P34+AAADwNgAAEAGEABDAH//wNwAAADAJAAAAMAAAMAWroCQGEAAQAsQgEBA3AAAAMAkAAAAwAAAwBaoAUCAeFlupJKa5uAhoMCAAADADIAAAMAAhBiAAEAB0QBwXKwYkAAAAAUaXNwZQAAAAAAAAKAAAAB4AAAABBwaXhpAAAAAAMICAgAAAAJaXJvdAMAAAAXaXBtYQAAAAAAAAABAAEEgQIDhAAAABppcmVmAAAAAAAAAA5jZHNjAAIAAQABAAACwG1kYXQAAAHQKAGvExB7DZ2ufw1k5TKPE//K+YnsdKChs9tP9NRKDeHdR62JE7rBlMqcWFV8SfFPnuCGhviCUvtAd8ZvhLPJ9bLynm9+beFUnrRf354QX0OrRr8ArMJ1fxeAAbLu0VkoI8vAPtKjDIpjCsIC6aulFw6/BFJLAp+J+oueLdF5W7Ld14VzcYejal/pF6oLQxQBEwPoAAADAAADADXgqLFxxDLFToGyXIAlkg25bSijcf+rh9IO04Te79jOlkkRCxTJ5ODQ3mziXw15CIvPfQYKbUfEHoqhWcHAmtFuGLCkZNwotC2m+xbwOIdycWk4afqgRg2X8rzLx6AAAAMAAAMAAAMAAAMAHNAb4h3ClqEPq2GMdJEDvzFT/ciuf3hCSR8fwvx5pOlx1JF0FI2Fv9IaSBrxKLMIqIAC6oAe7AQFAS0AmeBG0CPpJN/5+QRrrheOQ68HxtVVq9L1de4kDTrJykpwnBrZ5TNfjIMXFZEEYfSPRbeKib5Mmm7Os8ytfd4tQeua+s9Zb2m+aoBUTl9FZcZM5fimyYD1BCsl15Ib5DnaexxRaySFnbcy2AAB3AAAAwAAAwAABmyF4lqV8pR+Cks2lPbJ1RXKiAAAAwAAGVAAAAAGRXhpZgAATU0AKgAAAAgACAEPAAIAAAAIAAAAbgEQAAIAAAAJAAAAdgESAAMAAAABAAYAAAEaAAUAAAABAAAAgAEbAAUAAAABAAAAiAEoAAMAAAABAAEAAAITAAMAAAABAAEAAIdpAAQAAAABAAAAkAAAAABUZXN0Q2FtAE1vZGVsMTIzAAAAAAABAAAAAQAAAAEAAAABAAWCnQAFAAAAAQAAANKIJwADAAAAAQMgAACQAAAHAAAABDAyMzKRAQAHAAAABAECAwCgAQADAAAAAf//AAAAAAAAAAAAHAAAAAU="
