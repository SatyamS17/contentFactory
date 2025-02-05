package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
)

// TODO: Comments and clean up code
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

	post, err := processRedditPosts(client, azureConfig)
	if err != nil {
		log.Fatalf("Failed to process Reddit posts: %v", err)
	}

	if err := renderFinalVideo(post.ID); err != nil {
		log.Fatalf("Failed to render video: %v", err)
	}

	// Save processed id into done list after completing the render
	if err := saveProcessedID(post.ID); err != nil {
		log.Fatalf("Failed to save id to hisory: %v", err)
	}

	//* TODO: Make the uploading script run on the background once a day (Research best times to upload) (pending --> published)
	if err := uploadVideo(post); err != nil {
		log.Fatalf("Failed to upload video: %v", err)
	}
}

func renderFinalVideo(id string) error {
	// Command to run the Python script
	cmd := exec.Command("python3", "-u", "editor.py", id)

	// Get the stdout and stderr pipes
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Printf("Error getting stdout: %v\n", err)
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Printf("Error getting stderr: %v\n", err)
		return err
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		fmt.Printf("Error starting command: %v\n", err)
		return err
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
		return err
	}

	return nil
}
