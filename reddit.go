package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/joho/godotenv"
	"github.com/vartanbeno/go-reddit/v2/reddit"
)

type RedditConfig struct {
	ClientID     string
	ClientSecret string
	Username     string
	Password     string
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
	if err := godotenv.Load("private/info.env"); err != nil {
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

func processRedditPosts(client *reddit.Client, azureConfig AzureConfig) error {
	// Limit the number of posts fetched to one for now
	opts := &reddit.ListPostOptions{
		ListOptions: reddit.ListOptions{
			Limit: 1,
		},
		Time: "day",
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
