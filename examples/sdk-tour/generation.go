package main

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/garyblankenship/wormhole/v3/types"
)

func embeddingExample(ctx context.Context, j *journey) error {
	client := j.client()
	builder := client.Embeddings().Model(j.cfg.Models["embedding"]).Input(j.cfg.Inputs...)
	if j.fixture.mode == "embeddings-base64" {
		builder.EncodingFormat(types.EmbeddingEncodingBase64)
	}
	var response *types.EmbeddingsResponse
	var err error
	if j.fixture.mode == "embeddings-batched" {
		response, err = builder.GenerateBatched(ctx, j.cfg.Settings.BatchConcurrency)
	} else {
		response, err = builder.Generate(ctx)
	}
	if err != nil {
		return err
	}
	if response.Count() != len(j.cfg.Inputs) {
		return errors.New("embedding count/order mismatch")
	}
	for i, e := range response.Embeddings {
		if e.Index != i || (j.fixture.mode != "embeddings-base64" && !slices.Equal(e.Embedding, j.cfg.Vectors[i])) {
			return fmt.Errorf("embedding %d differs from input vector", i)
		}
		if j.fixture.mode == "embeddings-base64" {
			data, err := base64.StdEncoding.DecodeString(e.Base64)
			if err != nil {
				return err
			}
			if e.Embedding != nil || len(data) != len(j.cfg.Vectors[i])*4 {
				return errors.New("base64 vector length mismatch")
			}
			for k, want := range j.cfg.Vectors[i] {
				if math.Float32frombits(binary.LittleEndian.Uint32(data[k*4:])) != float32(want) {
					return errors.New("base64 vector fidelity mismatch")
				}
			}
		}
	}
	return nil
}
func rerankExample(ctx context.Context, j *journey) error {
	client := j.client()
	resp, err := client.Rerank().Model(j.cfg.Models["text"]).Query(j.cfg.Prompt).Documents(j.cfg.Documents...).TopN(len(j.cfg.Documents)).Generate(ctx)
	if err != nil {
		return err
	}
	if len(resp.Results) != 2 {
		return errors.New("rerank result count mismatch")
	}
	for position, index := range []int{1, 0} {
		result := resp.Results[position]
		if result.Index != index || result.Document != j.cfg.Documents[index] || result.RelevanceScore != j.cfg.Scores[index] {
			return fmt.Errorf("rerank source/score/order mismatch: %+v", result)
		}
	}
	return nil
}
func imageExample(ctx context.Context, j *journey) error {
	client := j.client()
	resp, err := client.Image().Model(j.cfg.Models["image"]).Prompt(j.cfg.Prompt).Size(j.cfg.Settings.ImageSize).ResponseFormat("b64_json").Generate(ctx)
	if err != nil {
		return err
	}
	return require(len(resp.Images) == 1 && resp.Images[0].B64JSON == j.cfg.Image, "image bytes mismatch")
}
func sttExample(ctx context.Context, j *journey) error {
	client := j.client()
	audio, err := decodeFixture(j.cfg.Audio)
	if err != nil {
		return err
	}
	resp, err := client.Audio().SpeechToText().Model(j.cfg.Models["audio"]).Audio(audio, j.cfg.Settings.AudioFormat).Transcribe(ctx)
	if err != nil {
		return err
	}
	return require(resp.Text == j.cfg.Answer, "transcription content mismatch")
}
func ttsExample(ctx context.Context, j *journey) error {
	client := j.client()
	resp, err := client.Audio().TextToSpeech().Model(j.cfg.Models["audio"]).Input(j.cfg.Prompt).Voice(j.cfg.Settings.Voice).ResponseFormat(j.cfg.Settings.AudioFormat).Generate(ctx)
	if err != nil {
		return err
	}
	want, err := decodeFixture(j.cfg.Audio)
	if err != nil {
		return err
	}
	return require(slices.Equal(resp.Audio, want) && resp.Format == j.cfg.Settings.AudioFormat, "speech bytes/format mismatch")
}
