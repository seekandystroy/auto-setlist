package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/seekandystroy/auto-setlist/internal/adapters"
	"github.com/seekandystroy/auto-setlist/internal/core/service"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: auto-setlist <artist name>")
		os.Exit(1)
	}
	artistName := strings.Join(os.Args[1:], " ")

	apiKey := os.Getenv("SETLISTFM_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "error: SETLISTFM_API_KEY environment variable is not set")
		os.Exit(1)
	}

	setlistFmAdapter := adapters.NewSetlistFmAdapter(apiKey)
	svc := service.NewService(setlistFmAdapter)

	artists, err := svc.SearchArtistsJSON(artistName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Outputting just one artists' struct for testing purposes before implementation
	artistString := fmt.Sprintf("%+v", artists[0])

	fmt.Println(artistString)
}
