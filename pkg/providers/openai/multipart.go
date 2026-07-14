package openai

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"path/filepath"
)

type audioFormData struct {
	audio       []byte
	filename    string
	model       string
	language    string
	prompt      string
	temperature *float32
}

func buildAudioForm(data audioFormData) (io.Reader, string, error) {
	if len(data.audio) == 0 {
		return nil, "", fmt.Errorf("no audio data provided")
	}

	filename := data.filename
	if filename == "" {
		filename = "audio.wav"
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
	header.Set("Content-Type", audioContentType(filename))
	part, err := writer.CreatePart(header)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create file part: %w", err)
	}
	if _, err := part.Write(data.audio); err != nil {
		return nil, "", fmt.Errorf("failed to write file data: %w", err)
	}

	fields := []struct {
		name  string
		value string
	}{
		{name: "model", value: data.model},
		{name: "language", value: data.language},
		{name: "prompt", value: data.prompt},
	}
	for _, field := range fields {
		if field.value == "" {
			continue
		}
		if err := writer.WriteField(field.name, field.value); err != nil {
			return nil, "", fmt.Errorf("failed to add %s field: %w", field.name, err)
		}
	}
	if data.temperature != nil {
		if err := writer.WriteField("temperature", fmt.Sprintf("%.2f", *data.temperature)); err != nil {
			return nil, "", fmt.Errorf("failed to add temperature field: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("failed to close multipart writer: %w", err)
	}
	return bytes.NewReader(body.Bytes()), writer.FormDataContentType(), nil
}

func audioContentType(filename string) string {
	switch filepath.Ext(filename) {
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".m4a":
		return "audio/mp4"
	case ".ogg":
		return "audio/ogg"
	case ".flac":
		return "audio/flac"
	case ".webm":
		return "audio/webm"
	default:
		return "application/octet-stream"
	}
}
