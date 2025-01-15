package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/vartanbeno/go-reddit/v2/reddit"
)

type AzureConfig struct {
	Region          string
	SubscriptionKey string
}

type RedditConfig struct {
	ClientID     string
	ClientSecret string
	Username     string
	Password     string
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

// estimateDuration calculates approximate duration for a piece of text
// assuming average speaking rate of 150 words per minute
func estimateDuration(text string) time.Duration {
	words := len(strings.Fields(text))
	// 400ms per word (150 words per minute)
	return time.Duration(words) * 400 * time.Millisecond
}

// formatDuration converts duration to simplified timestamp format (SS,mmm)
func formatDuration(d time.Duration) string {
	s := d / time.Second
	d -= s * time.Second
	ms := d / time.Millisecond

	return fmt.Sprintf("%02d,%03d", s, ms)
}

// createSubtitles generates subtitle entries from text
func createSubtitles(text string) []SubtitleEntry {
	var entries []SubtitleEntry
	var currentEntry strings.Builder
	var currentIndex int = 1
	var currentTime time.Duration = 0

	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Split(bufio.ScanWords)

	wordCount := 0

	for scanner.Scan() {
		word := scanner.Text()
		if len(currentEntry.String()) > 0 {
			currentEntry.WriteString(" ")
		}
		currentEntry.WriteString(word)
		wordCount++

		if wordCount >= 4 ||
			strings.ContainsAny(word, ".!?") ||
			currentEntry.Len() > 15 {

			entry := SubtitleEntry{
				Index:     currentIndex,
				StartTime: currentTime,
				Text:      strings.TrimSpace(currentEntry.String()),
			}

			duration := estimateDuration(entry.Text)
			entry.EndTime = currentTime + duration

			entries = append(entries, entry)
			currentIndex++
			currentTime += duration
			currentEntry.Reset()
			wordCount = 0
		}
	}

	if currentEntry.Len() > 0 {
		finalText := strings.TrimSpace(currentEntry.String())
		duration := estimateDuration(finalText)
		entries = append(entries, SubtitleEntry{
			Index:     currentIndex,
			StartTime: currentTime,
			EndTime:   currentTime + duration,
			Text:      finalText,
		})
	}

	return entries
}

// saveSubtitlesToFile saves subtitles in SRT format with simplified timestamps
func saveSubtitlesToFile(entries []SubtitleEntry) error {
	file, err := os.Create(fmt.Sprintf("text-to-speech/subtitles.txt"))
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

func initRedditClient(config RedditConfig) (*reddit.Client, error) {
	userAgent := fmt.Sprintf("my-reddit-bot/0.1 (by u/%s)", config.Username)
	return reddit.NewClient(reddit.Credentials{
		ID:       config.ClientID,
		Secret:   config.ClientSecret,
		Username: config.Username,
		Password: config.Password,
	}, reddit.WithUserAgent(userAgent))
}

func loadConfigs() (RedditConfig, AzureConfig, error) {
	if err := godotenv.Load("info.env"); err != nil {
		return RedditConfig{}, AzureConfig{}, fmt.Errorf("error loading .env file: %v", err)
	}

	redditConfig := RedditConfig{
		ClientID:     os.Getenv("CLIENT_ID"),
		ClientSecret: os.Getenv("CLIENT_SECRET"),
		Username:     os.Getenv("REDDIT_USERNAME"),
		Password:     os.Getenv("REDDIT_PASSWORD"),
	}

	azureConfig := AzureConfig{
		Region:          os.Getenv("AZURE_SPEECH_REGION"),
		SubscriptionKey: os.Getenv("AZURE_SPEECH_KEY"),
	}

	return redditConfig, azureConfig, nil
}

func textToSpeech(text string, config AzureConfig) ([]byte, error) {
	url := fmt.Sprintf("https://%s.tts.speech.microsoft.com/cognitiveservices/v1", config.Region)

	ssml := fmt.Sprintf(`<speak version='1.0' xml:lang='en-US'>
        <voice xml:lang='en-US' xml:gender='Female' name='en-US-JennyNeural'>
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

	filePath := fmt.Sprintf("text-to-speech/%s.mp3", content.fileName)
	if err := os.WriteFile(filePath, audioData, 0644); err != nil {
		return fmt.Errorf("failed to save audio file: %v", err)
	}

	log.Printf("Saved audio to %s\n", filePath)
	return nil
}

func processRedditPosts(client *reddit.Client, azureConfig AzureConfig) error {
	// Limit the number of posts fetched to one for now
	opts := &reddit.ListPostOptions{
		ListOptions: reddit.ListOptions{
			Limit: 1,
		},
		Time: "all",
	}

	posts, _, err := client.Subreddit.TopPosts(context.Background(), "AmItheAsshole", opts)
	if err != nil {
		return fmt.Errorf("failed to fetch posts: %v", err)
	}

	for i, post := range posts {
		fmt.Printf("%d. %s\n", i+1, post.Title)

		// Replace the AITA to the full form for when you are converting to text-to-speech
		if strings.HasPrefix(post.Title, "AITA") {
			post.Title = strings.Replace(post.Title, "AITA", "Am I the asshole", 1)
		}

		contents := []AudioContent{
			{text: post.Body, fileName: "post_body"},
			{text: post.Title, fileName: "post_title"},
		}

		for _, content := range contents {
			if err := saveTextToSpeech(content, azureConfig); err != nil {
				log.Printf("Error processing post %d: %v\n", i+1, err)
				continue
			}
		}

		// Generate and save subtitles
		subtitles := createSubtitles(post.Body)
		if err := saveSubtitlesToFile(subtitles); err != nil {
			log.Printf("Error generating subtitles for post %d: %v\n", i+1, err)
			continue
		}

	}

	return nil
}

func main() {
	redditConfig, azureConfig, err := loadConfigs()
	if err != nil {
		log.Fatalf("Failed to load configurations: %v", err)
	}

	client, err := initRedditClient(redditConfig)
	if err != nil {
		log.Fatalf("Failed to create Reddit client: %v", err)
	}

	log.Println("Reddit client created successfully!")

	if err := processRedditPosts(client, azureConfig); err != nil {
		log.Fatalf("Failed to process Reddit posts: %v", err)
	}
}
