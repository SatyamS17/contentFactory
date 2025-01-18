package main

import (
	"bytes"
	"context"
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

// Segment represents the transcription output from Whisper
type Segment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

type OEmbedResponse struct {
	HTML string `json:"html"`
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

	filePath := fmt.Sprintf("audio/text-to-speech/%s.mp3", content.fileName)
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

	// TODO: make each post processing its own go routine (will need to fix file naming and stuff)
	for i, post := range posts {
		var wg sync.WaitGroup

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

		wg.Add(2)
		// Get reddit embed (wrap in goroutine later)
		go getPostImage(post.URL, &wg)

		// Transcribe audio using Whisper (wrap in go routine later)
		go getSubtitles(&wg)

		wg.Wait()
	}

	return nil
}

func getPostImage(url string, wg *sync.WaitGroup) error {
	fmt.Println("Grabbing reddit post snapshot....")
	defer wg.Done()

	embedURL := "https://publish.reddit.com/embed?url="
	cmd := exec.Command("python3", "screenshot.py", embedURL+url)
	err := cmd.Run()

	if err != nil {
		return fmt.Errorf("failed to run screenshot script: %v", err)
	} else {
		fmt.Println("Got reddit post snapshot!")
	}

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

	// Command to run the Python script
	cmd := exec.Command("python3", "-u", "editor.py")

	// Get the stdout and stderr pipes
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Printf("Error getting stdout: %v\n", err)
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Printf("Error getting stderr: %v\n", err)
		return
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		fmt.Printf("Error starting command: %v\n", err)
		return
	}

	// Function to copy output to stdout in real-time
	copyOutput := func(reader io.ReadCloser) {
		defer reader.Close()
		if _, err := io.Copy(io.Writer(os.Stdout), reader); err != nil {
			fmt.Printf("Error copying output: %v\n", err)
		}
	}

	// Read stdout and stderr in separate goroutines
	go copyOutput(stdout)
	go copyOutput(stderr)

	// Wait for the command to finish
	if err := cmd.Wait(); err != nil {
		fmt.Printf("Command finished with error: %v\n", err)
	}
}
