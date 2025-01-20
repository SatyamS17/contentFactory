package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"math/rand"

	"github.com/joho/godotenv"
	"github.com/vartanbeno/go-reddit/v2/reddit"
)

type RedditConfig struct {
	ClientID     string
	ClientSecret string
	Username     string
	Password     string
}

const (
	processedPostFile = "video/pending/processedPosts.txt"
	enviroment        = "private/info.env"
	embedURL          = "https://publish.reddit.com/embed?url="
)

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
	if err := godotenv.Load(enviroment); err != nil {
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

// TODO: Later let the user pick how posts they want + what subreddit they want
func getRandomRedditPosts(client *reddit.Client) ([]*reddit.Post, error) {
	opts := &reddit.ListPostOptions{
		ListOptions: reddit.ListOptions{
			Limit: 25,
		},
		Time: "day",
	}

	posts, _, err := client.Subreddit.TopPosts(context.Background(), "AmItheAsshole", opts)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch posts: %v", err)
	}

	seenPosts := make(map[string]bool)
	today := time.Now().Format("2006-01-02")
	content, err := os.ReadFile(processedPostFile)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read history file: %v", err)
	}

	if len(content) > 0 {
		lines := strings.Split(string(content), "\n")
		if len(lines) > 0 && lines[0] != "" {
			fileDate := lines[0] // First line should be the date

			// If date doesn't match today, clear the file
			if fileDate != today {
				// Clear file by creating new empty file with just today's date
				if err := os.WriteFile(processedPostFile, []byte(today+"\n"), 0644); err != nil {
					return nil, fmt.Errorf("failed to reset history file: %v", err)
				}
			} else {
				// Date matches, load seen posts
				for _, id := range lines[1:] { // Skip first line (date)
					if id != "" {
						seenPosts[id] = true
					}
				}
			}
		}
	} else {
		// File is empty or doesn't exist, create new file with today's date
		if err := os.WriteFile(processedPostFile, []byte(today+"\n"), 0644); err != nil {
			return nil, fmt.Errorf("failed to create history file: %v", err)
		}
	}

	var unseenPosts []*reddit.Post
	for _, post := range posts {
		if !seenPosts[post.ID] {
			unseenPosts = append(unseenPosts, post)
		}
	}

	if len(unseenPosts) == 0 {
		return nil, fmt.Errorf("no unseen posts available")
	}

	return unseenPosts, nil
}

func processRedditPosts(client *reddit.Client, azureConfig AzureConfig) (*reddit.Post, error) {
	posts, err := getRandomRedditPosts(client)
	if err != nil {
		return nil, err
	}

	// TODO: Can allow for processing mulitple posts at once (for now do one at a time)
	post := posts[rand.Intn(len(posts))]
	// TODO: Save the pulled posts that wont be used for later to save API calls

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
			log.Printf("Error processing post %s: %v\n", post.ID, err)
			continue
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	// Get reddit embed (wrap in goroutine later)
	go getPostImage(post.URL, &wg)

	// Transcribe audio using Whisper (wrap in go routine later)
	go getSubtitles(&wg)

	wg.Wait()

	return post, nil
}

func getPostImage(url string, wg *sync.WaitGroup) error {
	fmt.Println("Grabbing reddit post snapshot....")
	defer wg.Done()

	cmd := exec.Command("python3", "screenshot.py", embedURL+url)
	err := cmd.Run()

	if err != nil {
		return fmt.Errorf("failed to run screenshot script: %v", err)
	} else {
		fmt.Println("Got reddit post snapshot!")
	}

	return nil
}

func saveProcessedID(id string) error {
	// Append the new post ID to history file
	f, err := os.OpenFile(processedPostFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open history file: %v", err)
	}
	defer f.Close()

	if _, err := f.WriteString(id + "\n"); err != nil {
		return fmt.Errorf("failed to write to history file: %v", err)
	}
	return nil
}
