package main

import (
	"fmt"

	"github.com/maxhawkins/go-webrtcvad"

	"github.com/bwmarrin/discordgo"
	"github.com/hraban/opus"

	"log"
	"os"
	"os/signal"
	"syscall"
	"sync"
)

// protoc --go_out=. --go-grpc_out=. stream.proto
// python -m grpc_tools.protoc -I. --python_out=. --grpc_python_out=. stream.proto

const (
	applicationID = ""
	publicKey     = ""
	clientSecret  = ""
	guildI        = ""
	guildID       = ""
	ChannelI      = ""
	ChannelID     = ""
	Token         = ""

	TokenYouTube = ""
	ProxyServer  = ""

	SAMPLE_RATE = 48000
	CHANNELS    = 2
	APPLICATION = "Music"
	FRAME_SIZE  = 960

	Voice_Detection_Frame_Size = 960
	PERCENT_VOICES_THRESHOLD = 0.3
)

type GuildTransfer struct {
	guildMap map[string]*Connecting
	Mu sync.Mutex
}

func NewGuildTransfer() *GuildTransfer {
	return &GuildTransfer{
		guildMap: make(map[string]*Connecting),
		Mu: sync.Mutex{},
	}
}

func main() {
	dsTransfer := NewGuildTransfer()

	vad, err := webrtcvad.New()
	if err != nil {
		log.Fatal(err)
	}
	if err := vad.SetMode(3); err != nil {
		log.Fatal(err)
	}
	if ok := vad.ValidRateAndFrameLength(SAMPLE_RATE, Voice_Detection_Frame_Size); !ok {
		log.Fatal("invalid rate or frame length")
	}

	encoder, err := opus.NewEncoder(SAMPLE_RATE, CHANNELS, opus.AppAudio)
	if err != nil {
		fmt.Printf("Failed to create encoder: %v", err)
		return
	}

	decoder, err := opus.NewDecoder(SAMPLE_RATE, CHANNELS)
	if err != nil {
		fmt.Printf("Failed to create decoder: %v", err)
		return
	}

	discord, err := discordgo.New("Bot " + Token)
	if err != nil {
		fmt.Println("Error creating Discord session,", err)
		return
	}

	discord.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		dsTransfer.commandHandler(s, i, encoder, decoder, vad)
	})
	discord.AddHandler(dsTransfer.voiceStateUpdate)

	// Открытие веб-сокета
	err = discord.Open()
	if err != nil {
		fmt.Println("Error opening connection,", err)
		return
	}
	defer discord.Close()

	// Регистрация команд Slash
	err = registerCommands(discord)
	if err != nil {
		fmt.Println("error registering commands,", err)
		return
	}

	fmt.Println("Press CTRL+C to exit.")
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-stop
}
