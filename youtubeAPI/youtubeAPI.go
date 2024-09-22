package youtubeAPI

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"io"

	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

type APItube struct {
	Token string
	Proxy string
}

func NewAPItube(token, proxy string) *APItube {
	return &APItube{
		Token: token,
		Proxy: proxy,
	}
}

func (at *APItube) SearchAudio(word_request string) ([]string, []string, error) {
	apiKey := at.Token
	ctx := context.Background()

	service, err := youtube.NewService(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		log.Fatalf("Error creating YouTube service: %v", err)
		return []string{}, []string{}, err
	}

	searchCall := service.Search.List([]string{"snippet"}).
		Q(word_request).
		Type("video").
		MaxResults(5)

	searchResponse, err := searchCall.Do()
	if err != nil {
		return []string{}, []string{}, fmt.Errorf("error making search API call: %v", err)
	}

	if len(searchResponse.Items) == 0 {
		return []string{}, []string{}, fmt.Errorf("нет результатов запроса")
	}
	var videoNames []string
	var videoIDs []string
	for i := 0; i < len(searchResponse.Items); i++ {
		if i >= 5 {
			break
		}
		videoIDs = append(videoIDs, searchResponse.Items[i].Id.VideoId)
		videoNames = append(videoNames, searchResponse.Items[i].Snippet.Title)
	}

	return []string{}, []string{}, nil
}

func (at *APItube) YoutubeAudio2OpusTonnel(videoIDorHttps string) (io.ReadCloser, *exec.Cmd, error) {

	var videoURL string
	if strings.HasPrefix(videoIDorHttps, "https://") {
		videoURL = videoIDorHttps
	} else {
		videoURL = fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoIDorHttps)
	}

	// Создание команды для скачивания с помощью yt-dlp
	cmd := exec.Command(
		"yt-dlp.exe",
		"-f", "bestaudio",
		"--recode-video", "opus",
		"--proxy", at.Proxy,
		"--no-playlist",
		"-o", "-",
		videoURL,
	)

	_, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}

	err = cmd.Start()
	if err != nil {
		return nil, nil, err
	}

	return nil, nil, nil
}