package openai

import (
	"io"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildAudioFormPreservesWireFormat(t *testing.T) {
	t.Parallel()

	temperature := float32(0.25)
	reader, contentType, err := buildAudioForm(audioFormData{
		audio:       []byte("audio bytes"),
		filename:    "speech.wav",
		model:       "whisper-1",
		language:    "en",
		prompt:      "transcribe",
		temperature: &temperature,
	})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(contentType, "multipart/form-data; boundary="))

	form := readAudioForm(t, reader, contentType)
	assert.Equal(t, []string{"whisper-1"}, form.Value["model"])
	assert.Equal(t, []string{"en"}, form.Value["language"])
	assert.Equal(t, []string{"transcribe"}, form.Value["prompt"])
	assert.Equal(t, []string{"0.25"}, form.Value["temperature"])
	require.Len(t, form.File["file"], 1)
	file := form.File["file"][0]
	assert.Equal(t, "speech.wav", file.Filename)
	assert.Equal(t, "audio/wav", file.Header.Get("Content-Type"))
	assert.Equal(t, `form-data; name="file"; filename="speech.wav"`, file.Header.Get("Content-Disposition"))
}

func TestBuildAudioFormDefaultsAndOmitsOptionalFields(t *testing.T) {
	t.Parallel()

	reader, contentType, err := buildAudioForm(audioFormData{audio: []byte("audio")})
	require.NoError(t, err)
	form := readAudioForm(t, reader, contentType)
	require.Len(t, form.File["file"], 1)
	file := form.File["file"][0]
	assert.Equal(t, "audio.wav", file.Filename)
	assert.Equal(t, "audio/wav", file.Header.Get("Content-Type"))
	assert.NotContains(t, form.Value, "model")
	assert.NotContains(t, form.Value, "language")
	assert.NotContains(t, form.Value, "prompt")
	assert.NotContains(t, form.Value, "temperature")

	_, _, err = buildAudioForm(audioFormData{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no audio data")
}

func TestAudioContentType(t *testing.T) {
	t.Parallel()

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
		assert.Equal(t, want, audioContentType(filename), filename)
	}
}

func readAudioForm(t *testing.T, reader io.Reader, contentType string) *multipart.Form {
	t.Helper()
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	boundary := strings.TrimPrefix(contentType, "multipart/form-data; boundary=")
	form, err := multipart.NewReader(strings.NewReader(string(data)), boundary).ReadForm(2048)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, form.RemoveAll()) })
	return form
}
