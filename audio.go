package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type AzureConfig struct {
	Region          string
	SubscriptionKey string
}

type AudioContent struct {
	text     string
	fileName string
}

// SubtitleEntry represents a single subtitle with timing
type SubtitleEntry struct {
	Index     int
	StartTime time.Duration
	EndTime   time.Duration
	Text      string
}

// Segment represents the transcription output from Whisper
type Segment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

// TranscribeAudio uses a Python Whisper script to transcribe audio
func TranscribeAudio(audioFile string) ([]Segment, error) {
	cmd := exec.Command("python3", "sub.py", audioFile)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()

	if err != nil {
		return nil, fmt.Errorf("failed to run whisper script: %v", err)
	}

	var segments []Segment
	if err := json.Unmarshal(out.Bytes(), &segments); err != nil {
		return nil, fmt.Errorf("failed to parse whisper output: %v", err)
	}

	return segments, nil
}

// formatDuration converts duration to simplified timestamp format (SS,mmm)
func formatDuration(d time.Duration) string {
	s := d / time.Second
	d -= s * time.Second
	ms := d / time.Millisecond

	return fmt.Sprintf("%02d,%03d", s, ms)
}

// saveSubtitlesToFile saves subtitles with simplified timestamps
func saveSubtitlesToFile(entries []SubtitleEntry) error {
	file, err := os.Create("audio/text-to-speech/subtitles.txt")
	if err != nil {
		return fmt.Errorf("failed to create subtitle file: %v", err)
	}
	defer file.Close()

	for _, entry := range entries {
		_, err := fmt.Fprintf(file, "%d\n%s --> %s\n%s\n\n",
			entry.Index,
			formatDuration(entry.StartTime),
			formatDuration(entry.EndTime),
			entry.Text)
		if err != nil {
			return fmt.Errorf("failed to write subtitle entry: %v", err)
		}
	}

	return nil
}

// ConvertSegmentsToSubtitles converts Whisper segments to subtitle entries
func ConvertSegmentsToSubtitles(segments []Segment) []SubtitleEntry {
	var entries []SubtitleEntry

	for i, segment := range segments {
		// Convert start and end times to time.Duration
		start := time.Duration(segment.Start * float64(time.Second))
		end := time.Duration(segment.End * float64(time.Second))

		// Create a new SubtitleEntry for each segment
		entry := SubtitleEntry{
			Index:     i + 1,
			StartTime: start,
			EndTime:   end,
			Text:      segment.Text,
		}

		// Append the entry to the list
		entries = append(entries, entry)
	}

	return entries
}

func textToSpeech(text string, config AzureConfig) ([]byte, error) {
	url := fmt.Sprintf("https://%s.tts.speech.microsoft.com/cognitiveservices/v1", config.Region)

	ssml := fmt.Sprintf(`<speak version='1.0' xml:lang='en-US'>
        <voice xml:lang='en-US' xml:gender='Male' name='en-US-AdamMultilingualNeural'>
            %s
        </voice>
    </speak>`, text)

	req, err := http.NewRequest("POST", url, strings.NewReader(ssml))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/ssml+xml")
	req.Header.Set("X-Microsoft-OutputFormat", "audio-16khz-128kbitrate-mono-mp3")
	req.Header.Set("Ocp-Apim-Subscription-Key", config.SubscriptionKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var buffer bytes.Buffer
	if _, err := io.Copy(&buffer, resp.Body); err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	return buffer.Bytes(), nil
}

func saveTextToSpeech(content AudioContent, azureConfig AzureConfig) error {
	audioData, err := textToSpeech(content.text, azureConfig)
	if err != nil {
		return fmt.Errorf("failed to synthesize speech: %v", err)
	}

	filePath := fmt.Sprintf("audio/text-to-speech/%s.mp3", content.fileName)
	if err := os.WriteFile(filePath, audioData, 0644); err != nil {
		return fmt.Errorf("failed to save audio file: %v", err)
	}

	log.Printf("Saved audio to %s\n", filePath)
	return nil
}

func getSubtitles(wg *sync.WaitGroup) {
	// Transcribe audio using Whisper
	fmt.Println("Creating subtitles....")
	defer wg.Done()

	segments, err := TranscribeAudio("/home/satyam/social/audio/text-to-speech/post_body.mp3")
	if err != nil {
		log.Printf("Error transcribing audio: %v\n", err)
		return
	}

	// Convert segments to subtitles
	subtitles := ConvertSegmentsToSubtitles(segments)

	// Save subtitles to file
	if err := saveSubtitlesToFile(subtitles); err != nil {
		log.Printf("Error saving subtitles: %v\n", err)
	} else {
		log.Printf("Subtitles downloaded!")
	}
}
