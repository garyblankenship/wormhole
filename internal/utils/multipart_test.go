package utils

import (
	"io"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMultipartBuilder(t *testing.T) {
	builder := NewMultipartBuilder()
	require.NoError(t, builder.AddTextField("model", "whisper-1"))
	require.NoError(t, builder.AddAudioFile("file", "audio.mp3", []byte("audio bytes")))

	data, contentType, err := builder.Build()
	require.NoError(t, err)
	assert.Contains(t, contentType, "multipart/form-data")

	reader := multipart.NewReader(strings.NewReader(string(data)), strings.TrimPrefix(contentType, "multipart/form-data; boundary="))
	form, err := reader.ReadForm(1024)
	require.NoError(t, err)
	assert.Equal(t, []string{"whisper-1"}, form.Value["model"])
	require.Len(t, form.File["file"], 1)
	assert.Equal(t, "audio.mp3", form.File["file"][0].Filename)
	assert.Equal(t, "audio/mpeg", form.File["file"][0].Header.Get("Content-Type"))
}

func TestMultipartBuilderReader(t *testing.T) {
	builder := NewMultipartBuilder()
	require.NoError(t, builder.AddTextField("field", "value"))

	reader, contentType, err := builder.Reader()
	require.NoError(t, err)
	assert.Contains(t, contentType, "multipart/form-data")

	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Contains(t, string(data), "value")
}

func TestGetContentType(t *testing.T) {
	tests := map[string]string{
		"audio.mp3":  "audio/mpeg",
		"audio.wav":  "audio/wav",
		"audio.m4a":  "audio/mp4",
		"audio.ogg":  "audio/ogg",
		"audio.flac": "audio/flac",
		"audio.webm": "audio/webm",
		"audio.bin":  "application/octet-stream",
	}

	for filename, want := range tests {
		t.Run(filename, func(t *testing.T) {
			assert.Equal(t, want, getContentType(filename))
		})
	}
}

func TestBuildAudioForm(t *testing.T) {
	temp := float32(0.25)
	reader, contentType, err := BuildAudioForm(AudioFormData{
		Audio:       []byte("audio bytes"),
		Filename:    "speech.wav",
		Model:       "whisper-1",
		Language:    "en",
		Prompt:      "transcribe",
		Temperature: &temp,
	})
	require.NoError(t, err)

	data, err := io.ReadAll(reader)
	require.NoError(t, err)

	boundary := strings.TrimPrefix(contentType, "multipart/form-data; boundary=")
	form, err := multipart.NewReader(strings.NewReader(string(data)), boundary).ReadForm(2048)
	require.NoError(t, err)
	assert.Equal(t, []string{"whisper-1"}, form.Value["model"])
	assert.Equal(t, []string{"en"}, form.Value["language"])
	assert.Equal(t, []string{"transcribe"}, form.Value["prompt"])
	assert.Equal(t, []string{"0.25"}, form.Value["temperature"])
	require.Len(t, form.File["file"], 1)
	assert.Equal(t, "speech.wav", form.File["file"][0].Filename)
}

func TestBuildAudioFormDefaultsAndValidation(t *testing.T) {
	_, _, err := BuildAudioForm(AudioFormData{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no audio data")

	reader, contentType, err := BuildAudioForm(AudioFormData{Audio: []byte("audio")})
	require.NoError(t, err)
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	boundary := strings.TrimPrefix(contentType, "multipart/form-data; boundary=")
	form, err := multipart.NewReader(strings.NewReader(string(data)), boundary).ReadForm(1024)
	require.NoError(t, err)
	require.Len(t, form.File["file"], 1)
	assert.Equal(t, "audio.wav", form.File["file"][0].Filename)
}
