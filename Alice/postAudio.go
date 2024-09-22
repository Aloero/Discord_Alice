package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"strings"

	"os/exec"
	"time"
	"youtubeAPI"

	"github.com/bwmarrin/discordgo"
	"github.com/hraban/opus"
)

// Проигрывание аудио в std тоннель
func (c *Connecting) play_2_PCM_tonnel(stdoutOpus io.Reader) (io.Reader, *exec.Cmd, error) {
	ffmpeg := exec.Command("ffmpeg", "-i", "-", "-f", "s16le", "-ar", "48000", "-ac", "2", "-")

	ffmpeg.Stdin = stdoutOpus

	out, err := ffmpeg.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}

	err = ffmpeg.Start()
	if err != nil {
		return nil, nil, err
	}

	return out, ffmpeg, nil
}

func stopProcess(cmd *exec.Cmd) error {
	err := cmd.Process.Kill()
	if err != nil {
		return err
	}

	err = cmd.Wait()
	if err != nil {
		return err
	}

	return nil
}

// Проигрывание аудио в дискорд из файла
func (c *Connecting) playAudio(encoder *opus.Encoder, voice *discordgo.VoiceConnection, stdoutOpus io.Reader, yt *exec.Cmd) error {
	stdout, ffmpeg, err := c.play_2_PCM_tonnel(stdoutOpus)
	if err != nil {
		return err
	}
	fmt.Print("Играет")

	c.playingAudio = true
	defer func() {
		c.playingAudio = false
		_ = stopProcess(ffmpeg)
		_ = stopProcess(yt)
	}()

	buffer := bufio.NewReaderSize(stdout, FRAME_SIZE*CHANNELS*2)
	var skipIteration uint64

	for {
		// Перекодирование из байтов в int16
		audioBuffer := make([]int16, FRAME_SIZE*CHANNELS)
		err = binary.Read(buffer, binary.LittleEndian, &audioBuffer)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil
		}
		if err != nil {
			return err
		}
		if c.exit {
			return nil
		}
		if c.pause {
			for {
				if !c.pause {
					break
				}
				time.Sleep(time.Millisecond * 16)
			}
		}
		if c.turnOff {
			// Отчистка каналов
			for {
				select {
				case structYT := <-c.chanAudioYt:
					_ = stopProcess(structYT.cmd)
				case <-c.chanQueryDownloadAudio:
				default:
					c.turnOff = false
					return nil
				}
			}
		}
		if c.skip {
			c.skip = false
			return nil
		}
		if c.skipSeconds != 0 {
			skipIteration = uint64((48000 / 960) * c.skipSeconds)
			c.skipSeconds = 0
		}
		for skipIteration > 0 {
			audioBuffer = make([]int16, FRAME_SIZE*CHANNELS)
			skipIteration -= 1
			continue
		}

		// Перекодирование из int16 в opus
		bufferOpus := make([]byte, FRAME_SIZE*CHANNELS)
		n2, err := encoder.Encode(audioBuffer, bufferOpus)
		if err != nil {
			fmt.Printf("Ошибка перекодирования %v", err)
			return err
		}

		voice.OpusSend <- bufferOpus[:n2]
	}
}

// Получение текста от whisper, поиск в ютуб и воспроизведение аудио в дискорд
func (c *Connecting) goGetTextProcessSendDiscord(encoder *opus.Encoder, voice *discordgo.VoiceConnection) {
	defer close(c.chanInputText)
	defer close(c.chanExitPost)

	// Получение текста
	// go c.goGetData(c.inputTextChan)
	// Обработка текста
	go c.goPreparingText(c.chanInputText, c.chanQueryDownloadAudio)
	// Скачивание музыки
	go c.goQueryDownloadAudio()
	// Воспроизведение аудио из файла
	go c.goPlayRequestAudio(encoder, voice)

	<-c.chanExit
	<-c.chanExitPostGetData
	<-c.chanExitPostPreparingText
	<-c.chanExitPostQueryDownloadAudio
	<-c.chanExitPostPlayRequestAudio
}

func (c *Connecting) goQueryDownloadAudio() {
	defer close(c.chanExitPostQueryDownloadAudio)

	youtubeAPI := youtubeAPI.NewAPItube(TokenYouTube, ProxyServer)

	for {
		select {
		case query := <-c.chanQueryDownloadAudio:
			_, _, err := startExecPlay(youtubeAPI, query)
			if err != nil {
				fmt.Printf("Error with downloading %v", err)
				continue
			}
			fmt.Print("Прошло")

			yt := &Yt{
				stdout: nil,
				cmd:    nil,
			}

			c.chanAudioYt <- yt

		case <-c.chanExit:
			return
		}
	}
}

// Воспроизведение аудио в дискорд
func (c *Connecting) goPlayRequestAudio(encoder *opus.Encoder, voice *discordgo.VoiceConnection) {
	defer close(c.chanExitPostPlayRequestAudio)
	
	for {
		select {
		case structYT := <-c.chanAudioYt:
			err := c.playAudio(encoder, voice, structYT.stdout, structYT.cmd)
			if err != nil {
				fmt.Println("Error during playback:", err)
				continue
			}
		case <-c.chanExit:
			return
		}
	}
}

// Обработка текста
func (c *Connecting) goPreparingText(tonnelIN <-chan string, tonnelOUT chan<- string) {
	defer close(c.chanExitPostPreparingText)

	for {
		select {
		case text := <-tonnelIN:

			bufferText := strings.ToLower(text)
			fmt.Printf("Текст: %s\n", bufferText)                //

			isAlise := strings.Contains(bufferText, "лис")
			if !isAlise {
				continue
			}

			pause := strings.Contains(bufferText, "пауза")
			if pause {
				c.pause = true
				continue
			}
			continuePlay := strings.Contains(bufferText, "продолжи")
			if continuePlay {
				c.pause = false
				continue
			}
			exit := strings.Contains(bufferText, "выйди")
			if exit {
				c.exitConnecting()
				continue
			}
			turnOff := strings.Contains(bufferText, "выключ")
			if turnOff {
				c.turnOff = true
				continue
			}
			skip := strings.Contains(bufferText, "пропуст")
			if skip {
				c.skip = true
				continue
			}
			indexSkipSeconds := strings.Index(bufferText, "перемотай на")
			if indexSkipSeconds != -1 {
				sliceText := bufferText[indexSkipSeconds+len("перемотай на")+1:]
				splitText := strings.Split(sliceText, " ")

				number, err := strconv.ParseUint(splitText[0], 10, 64)
				if err != nil {
					fmt.Printf("Число не распознано %v", err)
					continue
				}

				c.skipSeconds = number
				continue
			}
			index := strings.Index(bufferText, "ключ")
			if index != -1 {
				// if len(c.chanAudioPath) == 0 && !c.playingAudio {
				// 	c.chanAudioPath <- "pathToВключаю"
				// }

				prepareText := text[index+len("включи") - 1:]

				fmt.Printf("Запрос: %s\n", prepareText)                  //
				tonnelOUT <- prepareText
			}
		case <-c.chanExit:
			return
		}
	}
}

func startExecPlay(youtubeAPI *youtubeAPI.APItube, queryORhttps string) (io.Reader, *exec.Cmd, error) {
	var idORhttps string
	if strings.HasPrefix(queryORhttps, "https://") {
		idORhttps = queryORhttps
	} else {
		videoIDs, _, err := youtubeAPI.SearchAudio(queryORhttps)
		if err != nil {
			fmt.Printf("Error with youtube request: %v", err)
			return nil, nil, err
		}
		idORhttps = videoIDs[0]
	}

	stdout, cmd, err := youtubeAPI.YoutubeAudio2OpusTonnel(idORhttps)
	if err != nil {
		fmt.Printf("Error with youtube download: %v", err)
		return nil, nil, err
	}

	return stdout, cmd, err
}

func isChanClosed(ch <-chan struct{}) bool {
	select {
	case _, ok := <-ch:
		return !ok
	default:
		return false
	}
}

func (c *Connecting) exitConnecting() {
	c.exit = true
	if !isChanClosed(c.chanExit) {
		close(c.chanExit)
	}
}