package utils

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"path/filepath"
)

// MultipartBuilder helps build multipart form data for audio uploads
type MultipartBuilder struct {
	buffer *bytes.Buffer
	writer *multipart.Writer
}

// NewMultipartBuilder creates a new multipart form builder
func NewMultipartBuilder() *MultipartBuilder {
	buffer := &bytes.Buffer{}
	writer := multipart.NewWriter(buffer)

	return &MultipartBuilder{
		buffer: buffer,
		writer: writer,
	}
}

// AddTextField adds a text field to the form
func (m *MultipartBuilder) AddTextField(name, value string) error {
	return m.writer.WriteField(name, value)
}

// AddFileField adds a file field with binary data
func (m *MultipartBuilder) AddFileField(fieldName, filename string, data []byte) error {
	// Create form file with proper headers
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, filename))

	// Determine content type based on file extension
	contentType := getContentType(filename)
	h.Set("Content-Type", contentType)

	part, err := m.writer.CreatePart(h)
	if err != nil {
		return fmt.Errorf("failed to create file part: %w", err)
	}

	_, err = part.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write file data: %w", err)
	}

	return nil
}

// AddAudioFile adds an audio file field with appropriate content type
func (m *MultipartBuilder) AddAudioFile(fieldName, filename string, audioData []byte) error {
	return m.AddFileField(fieldName, filename, audioData)
}

// Build finalizes the multipart form and returns the data and content type
func (m *MultipartBuilder) Build() ([]byte, string, error) {
	err := m.writer.Close()
	if err != nil {
		return nil, "", fmt.Errorf("failed to close multipart writer: %w", err)
	}

	contentType := m.writer.FormDataContentType()
	data := m.buffer.Bytes()

	return data, contentType, nil
}

// Reader returns an io.Reader for the multipart data
func (m *MultipartBuilder) Reader() (io.Reader, string, error) {
	data, contentType, err := m.Build()
	if err != nil {
		return nil, "", err
	}

	return bytes.NewReader(data), contentType, nil
}

// getContentType returns appropriate MIME type for audio files
func getContentType(filename string) string {
	ext := filepath.Ext(filename)

	switch ext {
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

// AudioFormData represents structured data for audio upload forms
type AudioFormData struct {
	Audio       []byte
	Filename    string
	Model       string
	Language    string
	Prompt      string
	Temperature *float32
	// Additional fields can be added here
}

// BuildAudioForm creates a multipart form for audio upload
func BuildAudioForm(data AudioFormData) (io.Reader, string, error) {
	builder := NewMultipartBuilder()

	// Add audio file
	if len(data.Audio) == 0 {
		return nil, "", fmt.Errorf("no audio data provided")
	}

	filename := data.Filename
	if filename == "" {
		filename = "audio.wav" // Default filename
	}

	err := builder.AddAudioFile("file", filename, data.Audio)
	if err != nil {
		return nil, "", fmt.Errorf("failed to add audio file: %w", err)
	}

	// Add required model field
	if data.Model != "" {
		err = builder.AddTextField("model", data.Model)
		if err != nil {
			return nil, "", fmt.Errorf("failed to add model field: %w", err)
		}
	}

	// Add optional fields
	if data.Language != "" {
		err = builder.AddTextField("language", data.Language)
		if err != nil {
			return nil, "", fmt.Errorf("failed to add language field: %w", err)
		}
	}

	if data.Prompt != "" {
		err = builder.AddTextField("prompt", data.Prompt)
		if err != nil {
			return nil, "", fmt.Errorf("failed to add prompt field: %w", err)
		}
	}

	if data.Temperature != nil {
		err = builder.AddTextField("temperature", fmt.Sprintf("%.2f", *data.Temperature))
		if err != nil {
			return nil, "", fmt.Errorf("failed to add temperature field: %w", err)
		}
	}

	return builder.Reader()
}
