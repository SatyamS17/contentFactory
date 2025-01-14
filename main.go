package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/vartanbeno/go-reddit/v2/reddit"
)

func main() {
	// Load environment variables from the .env file
	err := godotenv.Load("info.env")
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}

	userAgent := fmt.Sprintf("my-reddit-bot/0.1 (by u/%s)", os.Getenv("REDDIT_USERNAME"))

	client, err := reddit.NewClient(reddit.Credentials{
		ID:       os.Getenv("CLIENT_ID"),
		Secret:   os.Getenv("CLIENT_SECRET"),
		Username: os.Getenv("REDDIT_USERNAME"),
		Password: os.Getenv("REDDIT_PASSWORD"),
	}, reddit.WithUserAgent(userAgent))
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	log.Println("Reddit client created successfully!")

	// Set the options for fetching posts using ListPostOptions
	opts := &reddit.ListPostOptions{
		ListOptions: reddit.ListOptions{
			Limit: 10, // Specify the number of posts to fetch
		},
		Time: "all",
	}

	// Fetch top posts with options
	posts, _, err := client.Subreddit.TopPosts(context.Background(), "funny", opts)
	if err != nil {
		log.Fatalf("Failed to fetch posts: %v", err)
	}

	// Process each post
	for i, post := range posts {
		fmt.Printf("%d. %s\n", i+1, post.Title)

		// Convert text to speech using Azure API
		audioData, err := textToSpeech(post.Title)
		if err != nil {
			log.Printf("Failed to synthesize speech for post %d: %v\n", i+1, err)
			continue
		}

		// Save audio to file
		fileName := fmt.Sprintf("post_%d.mp3", i+1)
		err = os.WriteFile(fileName, audioData, 0644)
		if err != nil {
			log.Printf("Failed to save audio file for post %d: %v\n", i+1, err)
			continue
		}

		log.Printf("Saved audio for post %d to %s\n", i+1, fileName)
	}
}

func textToSpeech(text string) ([]byte, error) {
	region := os.Getenv("AZURE_SPEECH_REGION")
	subscriptionKey := os.Getenv("AZURE_SPEECH_KEY")

	url := fmt.Sprintf("https://%s.tts.speech.microsoft.com/cognitiveservices/v1", region)

	// Create SSML input
	ssml := fmt.Sprintf(`<speak version='1.0' xml:lang='en-US'>
		<voice xml:lang='en-US' xml:gender='Female' name='en-US-JennyNeural'>
			%s
		</voice>
	</speak>`, text)

	// Create request
	req, err := http.NewRequest("POST", url, strings.NewReader(ssml))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/ssml+xml")
	req.Header.Set("X-Microsoft-OutputFormat", "audio-16khz-128kbitrate-mono-mp3")
	req.Header.Set("Ocp-Apim-Subscription-Key", subscriptionKey)

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Read response body
	var buffer bytes.Buffer
	_, err = io.Copy(&buffer, resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	return buffer.Bytes(), nil
}
