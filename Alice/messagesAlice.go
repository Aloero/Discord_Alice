package main

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"io"
	"os/exec"

	"github.com/maxhawkins/go-webrtcvad"

	"github.com/bwmarrin/discordgo"
	"github.com/hraban/opus"
)

type Connecting struct {
	pause                          bool
	playingAudio                   bool
	turnOff                        bool
	skip                           bool
	skipSeconds                    uint64
	exit                           bool
	chanExit                       chan struct{}
	chanExitGet                    chan struct{}
	chanExitPost                   chan struct{}
	chanExitPostGetData            chan struct{}
	chanExitPostPreparingText      chan struct{}
	chanExitPostQueryDownloadAudio chan struct{}
	chanExitPostPlayRequestAudio   chan struct{}
	chanAudioYt                    chan *Yt
	chanQueryDownloadAudio         chan string
	chanInputText                  chan string
	// voice                  *discordgo.VoiceConnection
	lastIDUpdateChannel string
	Mu                  sync.Mutex
}

type Yt struct {
	stdout io.Reader
	cmd    *exec.Cmd
}

func NewConnection() *Connecting { // voice *discordgo.VoiceConnection
	return &Connecting{
		pause:                          false,
		playingAudio:                   false,
		turnOff:                        false,
		skip:                           false,
		skipSeconds:                    0,
		exit:                           false,
		chanExit:                       make(chan struct{}),
		chanExitGet:                    make(chan struct{}),
		chanExitPost:                   make(chan struct{}),
		chanExitPostGetData:            make(chan struct{}),
		chanExitPostPreparingText:      make(chan struct{}),
		chanExitPostQueryDownloadAudio: make(chan struct{}),
		chanExitPostPlayRequestAudio:   make(chan struct{}),
		chanAudioYt:                    make(chan *Yt, 2),
		chanQueryDownloadAudio:         make(chan string, 100),
		chanInputText:                  make(chan string, 10),
		// voice:                  voice,
		lastIDUpdateChannel: "",
		Mu:                  sync.Mutex{},
	}
}

func (ds *GuildTransfer) commandHandler(s *discordgo.Session, i *discordgo.InteractionCreate, encoder *opus.Encoder, decoder *opus.Decoder, vad *webrtcvad.VAD) {
	if i.Type == discordgo.InteractionApplicationCommand {
		switch i.ApplicationCommandData().Name {

		case "start":
			commandStart(ds, s, i, encoder, decoder, vad)

		case "play":
			commandPlay(ds, s, i, encoder, decoder, vad)

		case "leave":
			commandLeave(ds, s, i)

		case "clear-queue": // turnOff
			ds.Mu.Lock()
			guildConnection := ds.guildMap[i.GuildID]
			ds.Mu.Unlock()
			if guildConnection == nil {
				err := answerCommand("%s", "Bot is not in voice", s, i)
				if err != nil {
					fmt.Println("error responding to interaction,", err)
				}
				return
			}
			err := answerCommand("%s", "Clear-queue", s, i)
			if err != nil {
				fmt.Println("error responding to interaction,", err)
			}
			guildConnection.turnOff = true

		case "skip":
			ds.Mu.Lock()
			guildConnection := ds.guildMap[i.GuildID]
			ds.Mu.Unlock()
			if guildConnection == nil {
				err := answerCommand("%s", "Bot is not in voice", s, i)
				if err != nil {
					fmt.Println("error responding to interaction,", err)
				}
				return
			}
			err := answerCommand("%s", "Skip", s, i)
			if err != nil {
				fmt.Println("error responding to interaction,", err)
			}
			guildConnection.skip = true

		case "pause":
			ds.Mu.Lock()
			guildConnection := ds.guildMap[i.GuildID]
			ds.Mu.Unlock()
			if guildConnection == nil {
				err := answerCommand("%s", "Bot is not in voice", s, i)
				if err != nil {
					fmt.Println("error responding to interaction,", err)
				}
				return
			}
			err := answerCommand("%s", "Pause", s, i)
			if err != nil {
				fmt.Println("error responding to interaction,", err)
			}
			guildConnection.pause = true

		case "continue":
			ds.Mu.Lock()
			guildConnection := ds.guildMap[i.GuildID]
			ds.Mu.Unlock()
			if guildConnection == nil {
				err := answerCommand("%s", "Bot is not in voice", s, i)
				if err != nil {
					fmt.Println("error responding to interaction,", err)
				}
				return
			}
			err := answerCommand("%s", "Continue", s, i)
			if err != nil {
				fmt.Println("error responding to interaction,", err)
			}
			guildConnection.pause = false

		case "rewind": // skipSeconds
			ds.Mu.Lock()
			guildConnection := ds.guildMap[i.GuildID]
			ds.Mu.Unlock()
			if guildConnection == nil {
				err := answerCommand("%s", "Bot is not in voice", s, i)
				if err != nil {
					fmt.Println("error responding to interaction,", err)
				}
				return
			}
			seconds := i.ApplicationCommandData().Options[0].StringValue()
			secondUint, err := strconv.ParseUint(seconds, 10, 64)
			if err != nil || secondUint < 1 {
				err := answerCommand("Not a natural number: %s", seconds, s, i)
				if err != nil {
					fmt.Println("error responding to interaction,", err)
				}
				return
			}
			err = answerCommand("Rewinded: %s", seconds, s, i)
			if err != nil {
				fmt.Println("error responding to interaction,", err)
			}
			guildConnection.skipSeconds = secondUint
		}
	}
}

func startConnecting(connection *Connecting, ds *GuildTransfer, s *discordgo.Session, i *discordgo.InteractionCreate, vs_ *discordgo.VoiceState, encoder *opus.Encoder, decoder *opus.Decoder, vad *webrtcvad.VAD) {
	ds.Mu.Lock()
	ds.guildMap[i.GuildID] = connection
	ds.Mu.Unlock()

	connection.lastIDUpdateChannel = i.ChannelID

	var voice *discordgo.VoiceConnection
	voice, err := s.ChannelVoiceJoin(i.GuildID, vs_.ChannelID, false, false)
	if err != nil {
		fmt.Println("Error joining voice channel,", err)
		ds.Mu.Lock()
		ds.guildMap[i.GuildID] = nil
		ds.Mu.Unlock()
		connection.exitConnecting()
		return
	}

	defer func() {
		ds.Mu.Lock()
		ds.guildMap[i.GuildID] = nil
		ds.Mu.Unlock()
		close(connection.chanAudioYt)
		close(connection.chanQueryDownloadAudio)
		voice.Speaking(false)
		voice.Disconnect()
		fmt.Print("Поток выключился\n")
	}()
	
	go connection.goGetTextProcessSendDiscord(encoder, voice)
	go connection.goGetAudioProcessSendgRPC(decoder, voice, vad)

	fmt.Print("Поток запустился\n")

	<-connection.chanExit
	<-connection.chanExitGet
	<-connection.chanExitPost
}

// Проверка состояния
func (ds *GuildTransfer) voiceStateUpdate(s *discordgo.Session, vs *discordgo.VoiceStateUpdate) {
	botID := s.State.User.ID
	guildID := vs.GuildID
	
	ds.Mu.Lock()
	guildConnection := ds.guildMap[guildID]
	ds.Mu.Unlock()

	// Проверяем, что это обновление касается бота
	if vs.UserID == botID {
		if vs.ChannelID == "" {


			if guildConnection == nil {
				return
			}
			
			ds.Mu.Lock()
			ds.guildMap[guildID] = nil
			ds.Mu.Unlock()

			guildConnection.exitConnecting()

			textChannelID := guildConnection.lastIDUpdateChannel
			message := "Bot was kicked or disconnected from the voice channel"
			_, err := s.ChannelMessageSend(textChannelID, message)
			if err != nil {
				fmt.Println("Error sending message to channel:", err)
			}
		}

		return
	}

	// Получаем гильдию, в которой произошло событие
	guild, err := s.State.Guild(vs.GuildID)
	if err != nil {
		fmt.Println("Error fetching guild:", err)
		return
	}

	// Проверяем, находится ли бот в этом канале
	var botChannelID string
	for _, voiceState := range guild.VoiceStates {
		if voiceState.UserID == botID {
			botChannelID = voiceState.ChannelID
			break
		}
	}

	// Если бот не в голосовом канале, выходим
	if botChannelID == "" {
		return
	}

	// Если обновление связано с каналом, в котором находится бот
	if vs.ChannelID == botChannelID || vs.BeforeUpdate != nil && vs.BeforeUpdate.ChannelID == botChannelID {
		usersInChannel := 0

		// Считаем количество пользователей в том же канале
		for _, voiceState := range guild.VoiceStates {
			if voiceState.ChannelID == botChannelID && voiceState.UserID != botID {
				usersInChannel++
			}
		}

		if usersInChannel == 0 {

			ds.Mu.Lock()
			ds.guildMap[guildID] = nil
			ds.Mu.Unlock()
			textChannelID := guildConnection.lastIDUpdateChannel
			guildConnection.exitConnecting()

			message := "Bot is alone in the voice channel."
			_, err := s.ChannelMessageSend(textChannelID, message)
			if err != nil {
				fmt.Println("Error sending message to channel:", err)
			}
		}
	}
}

// Регистрация Slash-команд
func registerCommands(s *discordgo.Session) error {
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "start",
			Description: "Join to channel",
		},
		{
			Name:        "play",
			Description: "Request for music",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "query-or-link",
					Description: "Request for music",
					Type:        discordgo.ApplicationCommandOptionString,
					Required:    true,
				},
			},
		},
		{
			Name:        "leave",
			Description: "Leave from voice",
		},
		{
			Name:        "clear-queue",
			Description: "Clears bot's queue",
		},
		{
			Name:        "skip",
			Description: "Skips bot's queue",
		},
		{
			Name:        "pause",
			Description: "Pause bot's queue",
		},
		{
			Name:        "continue",
			Description: "Continue bot's queue",
		},
		{
			Name:        "rewind",
			Description: "Rewind music",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "seconds",
					Description: "Input a number",
					Type:        discordgo.ApplicationCommandOptionString,
					Required:    true,
				},
			},
		},
	}

	_, err := s.ApplicationCommandBulkOverwrite(s.State.User.ID, "", commands)
	return err
}

func commandStart(ds *GuildTransfer, s *discordgo.Session, i *discordgo.InteractionCreate, encoder *opus.Encoder, decoder *opus.Decoder, vad *webrtcvad.VAD) {
	var guildConnection *Connecting
	ds.Mu.Lock()
	guildConnection = ds.guildMap[i.GuildID]
	ds.Mu.Unlock()

	guild, err := s.State.Guild(i.GuildID)
	if err != nil {
		fmt.Println("Ошибка получения гильдии:", err)
		return
	}

	// Поиск пользователя в каналах
	var vs_ *discordgo.VoiceState
	for _, vs := range guild.VoiceStates {
		if vs.UserID == i.Member.User.ID {
			vs_ = vs
			break
		}
	}

	// Ответ пользователю
	if vs_ == nil {
		response := "You need to be in the voice channel"
		err := answerCommand("%s", response, s, i)
		if err != nil {
			fmt.Println("error responding to interaction,", err)
		}
		return
	} else {
		response := "Starting"
		err := answerCommand("%s", response, s, i)
		if err != nil {
			fmt.Println("error responding to interaction,", err)
		}
	}

	if guildConnection != nil {
		guildConnection.exitConnecting()
		time.Sleep(time.Second * 1)
	}

	connecting := NewConnection()
	guildConnection = connecting

	startConnecting(guildConnection, ds, s, i, vs_, encoder, decoder, vad)
}

func commandPlay(ds *GuildTransfer, s *discordgo.Session, i *discordgo.InteractionCreate, encoder *opus.Encoder, decoder *opus.Decoder, vad *webrtcvad.VAD) {
	ds.Mu.Lock()
	guildConnection := ds.guildMap[i.GuildID]
	ds.Mu.Unlock()

	var text string
	if guildConnection != nil {
		text = i.ApplicationCommandData().Options[0].StringValue()
		err := answerCommand("Play: %s", text, s, i)
		if err != nil {
			fmt.Println("error responding to interaction,", err)
		}
		guildConnection.chanQueryDownloadAudio <- text
		return
	}

	guild, err := s.State.Guild(i.GuildID)
	if err != nil {
		fmt.Println("Ошибка получения гильдии:", err)
		return
	}

	// Поиск пользователя в каналах
	var vs_ *discordgo.VoiceState
	for _, vs := range guild.VoiceStates {
		if vs.UserID == i.Member.User.ID {
			vs_ = vs
			break
		}
	}

	// Ответ пользователю
	if vs_ == nil {
		err := answerCommand("%s", "You need to be in the voice channel", s, i)
		if err != nil {
			fmt.Println("error responding to interaction,", err)
		}
		return
	} else {
		text = i.ApplicationCommandData().Options[0].StringValue()
		err := answerCommand("Играет: %s", text, s, i)
		if err != nil {
			fmt.Println("error responding to interaction,", err)
		}
	}

	connecting := NewConnection()
	guildConnection = connecting
	guildConnection.chanQueryDownloadAudio <- text

}

func commandLeave(ds *GuildTransfer, s *discordgo.Session, i *discordgo.InteractionCreate) {
	ds.Mu.Lock()
	guildConnection := ds.guildMap[i.GuildID]
	ds.Mu.Unlock()

	err := answerCommand("%s", "Left channel", s, i)
	if err != nil {
		fmt.Println("error responding to interaction,", err)
	}

	if guildConnection == nil {
		return
	}
	guildConnection.exitConnecting()
}

func answerCommand(extraText, text string, s *discordgo.Session, i *discordgo.InteractionCreate) error {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf(extraText, text),
		},
	})
	if err != nil {
		return err
	}
	return nil
}