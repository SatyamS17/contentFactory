package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/vartanbeno/go-reddit/v2/reddit"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

const (
	redirectPort      = "8080"
	redirectURL       = "http://localhost:8080/callback"
	pending           = "video/pending"
	uploadingInterval = 6
)

func getClient(config *oauth2.Config) *http.Client {
	// Load token from file or generate new one
	tokFile := "private/token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	// Create a channel to receive the authorization code
	codeChan := make(chan string)

	var server *http.Server // Declare server variable first

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/callback" {
			code := r.URL.Query().Get("code")
			codeChan <- code

			// Send success message to browser
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, "<h1>Authorization Successful</h1><p>You can close this window and return to the application.</p>")

			// Shutdown server after small delay
			go func() {
				time.Sleep(2 * time.Second)
				server.Shutdown(context.Background())
			}()
		}
	})

	// Now initialize the server with the handler
	server = &http.Server{
		Addr:    ":" + redirectPort,
		Handler: handler,
	}

	// Start the server in a goroutine
	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Generate the authorization URL
	config.RedirectURL = redirectURL
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)

	fmt.Printf("Go to the following link in your browser:\n%v\n", authURL)

	// Wait for the code from the callback
	code := <-codeChan

	// Exchange the authorization code for a token
	token, err := config.Exchange(context.Background(), code)
	if err != nil {
		log.Fatalf("Unable to retrieve token: %v", err)
	}

	return token
}

func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

func saveToken(path string, token *oauth2.Token) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func getVideostoUpload(postID string) ([]string, error) {
	// Read the directory
	entries, err := os.ReadDir(pending)
	if err != nil {
		return nil, fmt.Errorf("failed to read pending list: %v", err)
	}

	// Check if directory is empty
	if len(entries) == 0 {
		return nil, fmt.Errorf("pending list is empty")
	}

	// Prepare regular expression to match filenames like <post.id>_part_<num>.mp4
	re := regexp.MustCompile(fmt.Sprintf(`^%s_part_\d+\.mp4$`, regexp.QuoteMeta(postID)))

	// Initialize a slice to store matching file paths
	var matchedFiles []string

	// Find all relevant MP4 files
	for _, entry := range entries {
		// Skip directories and check for .mp4 extension
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".mp4") {
			// If the filename matches the pattern <post.id>_part_<num>.mp4
			if re.MatchString(entry.Name()) {
				matchedFiles = append(matchedFiles, filepath.Join(pending, entry.Name()))
			}
		}
	}

	// If no files were found that match the pattern
	if len(matchedFiles) == 0 {
		return nil, fmt.Errorf("no pending video files found for post ID %s in directory", postID)
	}

	return matchedFiles, nil
}

func extractPartNumber(filename string) (int, error) {
	// Define the regular expression
	re := regexp.MustCompile(`part_(\d+)`)

	// Find the first match for the regular expression
	matches := re.FindStringSubmatch(filename)

	// If no match is found, return an error
	if len(matches) < 2 {
		return 0, fmt.Errorf("no part number found in filename")
	}

	// Convert the matched part number string to an integer
	var partNumber int
	_, err := fmt.Sscanf(matches[1], "%d", &partNumber)
	if err != nil {
		return 0, fmt.Errorf("unable to convert part number to integer: %v", err)
	}

	return partNumber, nil
}

func getScheduledTime(partNum int) string {
	// Calculate the scheduled start time for the next video (in UTC)
	now := time.Now().UTC()
	scheduledTime := now.Add(time.Duration(uploadingInterval*(partNum-1)) * time.Hour)
	return scheduledTime.Format(time.RFC3339) // Format as RFC3339
}

func uploadVideo(post *reddit.Post) error {
	if err := godotenv.Load("private/info.env"); err != nil {
		return err
	}

	ctx := context.Background()

	// Read the credentials file
	b, err := os.ReadFile("private/client_secrets.json")
	if err != nil {
		return fmt.Errorf("unable to read client secret file: %v", err)
	}

	// Configure OAuth2
	config, err := google.ConfigFromJSON(b, youtube.YoutubeUploadScope)
	if err != nil {
		return fmt.Errorf("unable to parse client secret file to config: %v", err)
	}

	client := getClient(config)

	service, err := youtube.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return fmt.Errorf("error creating YouTube client: %v", err)
	}

	filepaths, err := getVideostoUpload(post.ID)
	if err != nil {
		return fmt.Errorf("error getting video: %v", err)
	}

	for _, filepath := range filepaths {
		partNum, err := extractPartNumber(filepath)
		if err != nil {
			fmt.Printf("error getting video: %v", err)
		}

		file, err := os.Open(filepath)
		if err != nil {
			return fmt.Errorf("error opening file: %v", err)
		}
		defer file.Close()

		// Create the video upload object
		videoTitle := fmt.Sprintf("Part %d | %s", partNum, post.Title)
		description := fmt.Sprintf("Credit: %s\n\n%s\n\nURL: %s", post.Author, post.Body, post.URL)
		fmt.Printf("Uploading video with title: %s\n", videoTitle) // Debug print to check title

		upload := &youtube.Video{
			Snippet: &youtube.VideoSnippet{
				Title:       videoTitle,
				Description: description,
				CategoryId:  "22",
				Tags: []string{
					"#Shorts", "#AITA", "#r/AmItheAsshole", "#Reddit", "#Stories",
					"#Funny", "#BestOfReddit", "#LOL", "#Entertainment", "#Relatable",
					"#TrueStories", "#LifeStories", "#Drama", "#DailyDose",
				},
			},
			Status: &youtube.VideoStatus{
				PrivacyStatus: "public",
			},
		}

		if partNum > 1 {
			upload.Status.PrivacyStatus = "private"
			upload.Status.PublishAt = getScheduledTime(partNum)
		}

		call := service.Videos.Insert([]string{"snippet", "status"}, upload)
		response, err := call.Media(file).Do()
		if err != nil {
			return fmt.Errorf("error making YouTube API call: %v", err)
		}

		fmt.Printf("Video uploaded successfully! Video ID: %s\n", response.Id)

		// Move the file to published
		destPath := "video/published/" + strings.Split(filepath, "/")[2]
		err = os.Rename(filepath, destPath)
		if err != nil {
			fmt.Println("Error moving file:", err)
		} else {
			fmt.Println("File moved successfully to ", destPath)
		}
	}

	return nil
}
