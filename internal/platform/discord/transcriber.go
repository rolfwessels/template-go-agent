package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

type Transcriber interface {
	Transcribe(ctx context.Context, audioURL string) (string, error)
}

type whisperTranscriber struct {
	apiKey string
	client *http.Client
}

func NewWhisperTranscriber(apiKey string) Transcriber {
	return &whisperTranscriber{
		apiKey: apiKey,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (w *whisperTranscriber) Transcribe(ctx context.Context, audioURL string) (string, error) {
	audio, err := w.download(ctx, audioURL)
	if err != nil {
		return "", err
	}
	return w.transcribe(ctx, audio)
}

func (w *whisperTranscriber) download(ctx context.Context, audioURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, audioURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating download request: %w", err)
	}
	resp, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading audio: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading audio: %w", err)
	}
	return data, nil
}

func (w *whisperTranscriber) transcribe(ctx context.Context, audio []byte) (string, error) {
	body, contentType, err := buildMultipartForm(audio)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/audio/transcriptions", body)
	if err != nil {
		return "", fmt.Errorf("creating transcription request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+w.apiKey)
	req.Header.Set("Content-Type", contentType)

	resp, err := w.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("transcription request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("transcription API returned %d", resp.StatusCode)
	}

	var result struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding transcription response: %w", err)
	}
	return result.Text, nil
}

func buildMultipartForm(audio []byte) (*bytes.Buffer, string, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", "voice.ogg")
	if err != nil {
		return nil, "", fmt.Errorf("creating form file: %w", err)
	}
	if _, err := part.Write(audio); err != nil {
		return nil, "", fmt.Errorf("writing audio to form: %w", err)
	}
	if err := w.WriteField("model", "whisper-1"); err != nil {
		return nil, "", fmt.Errorf("writing model field: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, "", fmt.Errorf("closing multipart writer: %w", err)
	}
	return &body, w.FormDataContentType(), nil
}
