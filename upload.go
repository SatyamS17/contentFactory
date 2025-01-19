package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

const (
	redirectPort = "8080"
	redirectURL  = "http://localhost:8080/callback"
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

func uploadVideo() error {
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

	file, err := os.Open("video/result.mp4")
	if err != nil {
		return fmt.Errorf("error opening file: %v", err)
	}
	defer file.Close()

	upload := &youtube.Video{
		Snippet: &youtube.VideoSnippet{
			Title:       "Testing",
			Description: "If this works we are golden",
			CategoryId:  "22",
			Tags:        []string{"#Shorts"},
		},
		Status: &youtube.VideoStatus{
			PrivacyStatus: "public",
		},
	}

	call := service.Videos.Insert([]string{"snippet", "status"}, upload)
	response, err := call.Media(file).Do()
	if err != nil {
		return fmt.Errorf("error making YouTube API call: %v", err)
	}

	fmt.Printf("Video uploaded successfully! Video ID: %s\n", response.Id)
	return nil
}
