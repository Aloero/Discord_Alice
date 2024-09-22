package main

import (
	"fmt"
	"math"
	"time"
	"encoding/binary"
	"log"

	"bytes"
	"os/exec"

	"github.com/maxhawkins/go-webrtcvad"


	"github.com/bwmarrin/discordgo"
	"github.com/hraban/opus"

	"context"

	"google.golang.org/grpc"
	"Alice/proto"
)

// Получение аудио из дискорда и отправка в тоннель
func (c *Connecting) goGetAudioProcessSendgRPC(decoder *opus.Decoder, voice *discordgo.VoiceConnection, vad *webrtcvad.VAD) {
	voice.Speaking(true)
	defer voice.Speaking(false)
	defer close(c.chanExitGet)

	const (
		QUITE_SIZE_Milliseconds = 800
		QUITE_SIZE = 800 * 48
		QUITEThresholdDB = 35.0
		STEP = 20 * 48
		TIME_REQUEST = 800 * 48
	)

	buffsize := 144000 * 2
	bufferWheel := make([]int16, buffsize)
	for i := 0; i < len(bufferWheel); i++ {
		bufferWheel[i] = 0
	}
	offset := 0
	
	// Установление соединения с сервером
	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("Did not connect: %v", err)
	}
	defer conn.Close()
	client := proto.NewStreamServiceClient(conn)

	var pkt *discordgo.Packet
	var st time.Time
	var flagGot bool
	var changeOffset bool
	for {
		select {
		case pkt = <-voice.OpusRecv:
			if pkt == nil {
				continue
			}
			flagGot = true
			st = time.Now()

			// Декодирование данных в PCM
			bufferPCM := make([]int16, 960*CHANNELS)
			n, err := decoder.Decode(pkt.Opus, bufferPCM)
			if err != nil {
				fmt.Printf("Ошибка декодирования %v", err)
				continue
			}
			bufferPCM = bufferPCM[:n*CHANNELS]

			// Запись в колесный буфер
			oldOffset := offset
			monoAudio := preprocess2MonoAudio(bufferPCM)
			err = reWritingBuffWheel(&bufferWheel, monoAudio, &offset)
			if err != nil {
				fmt.Printf("Ошибка в колесном буфере: %v\n", err)
				return
			}
			if offset < oldOffset {
				changeOffset = true
			}

			bufferPause, err := negativeSlice(bufferWheel, offset - QUITE_SIZE, offset)
			if err != nil {
				fmt.Printf("Ошибка: %v", err)
			}
			quiteBool := checkQuitePause(bufferPause, QUITEThresholdDB)

			if !quiteBool {
				continue
			}

			var all_buffer []int16
			if !changeOffset {
				all_buffer = bufferWheel[:offset]
			} else {
				all_buffer = append(bufferWheel[offset:], bufferWheel[:offset]...)
			}
			changeOffset = false
			offset = 0

			if len(all_buffer) < TIME_REQUEST {
				continue
			}
			c.checkAudioData(all_buffer, vad, client)

		case <-c.chanExit:
			return

		default:
			if (time.Since(st) < time.Millisecond * QUITE_SIZE_Milliseconds || !flagGot) {
				time.Sleep(time.Millisecond)
				continue
			}

			var all_buffer []int16
			if !changeOffset {
				all_buffer = bufferWheel[:offset]
			} else {
				all_buffer = append(bufferWheel[offset:], bufferWheel[:offset]...)
			}
			flagGot = false
			changeOffset = false
			offset = 0

			if len(all_buffer) < TIME_REQUEST {
				continue
			}

			c.checkAudioData(all_buffer, vad, client)
		}
	}
}

func (c *Connecting) serverConnection(client proto.StreamServiceClient, msg *proto.Message) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*200)
	defer cancel()

	r, err := client.Chat(ctx, msg)
	if err != nil {
		fmt.Printf("Could not greet: %v\n", err)
		return
	}

	select {
	case c.chanInputText <- r.Body:
	default:
		return
	}
}

func (c *Connecting) checkAudioData(all_buffer []int16, vad *webrtcvad.VAD, client proto.StreamServiceClient) {
	// Байты и Голос
	byteAudio := processInt2ByteAudio(all_buffer)
	isVoices := checkVoices(vad, byteAudio, PERCENT_VOICES_THRESHOLD)

	if !isVoices {
		return
	}

	// Отправка byteAudio через gRPC
	msg := &proto.Message{Body: byteAudio}
	c.serverConnection(client, msg)
}

func saveAudio(data []int16, path string) error {
	buf := new(bytes.Buffer)

	for _, v := range data {
		if err := binary.Write(buf, binary.LittleEndian, v); err != nil {
			return fmt.Errorf("ошибка при записи в буфер: %w", err)
		}
	}

	cmd := exec.Command("ffmpeg", "-f", "s16le", "-ar", "48000", "-ac", "1", "-i", "-", path)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("ошибка при открытии stdin: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ошибка при запуске ffmpeg: %w", err)
	}

	if _, err := stdin.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("ошибка при записи данных в stdin: %w", err)
	}

	if err := stdin.Close(); err != nil {
		return fmt.Errorf("ошибка при закрытии stdin: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ошибка при ожидании завершения ffmpeg: %w", err)
	}

	fmt.Println("Файл", path, "создан успешно")
	return nil
}

func negativeSlice[T any](arr []T, start int, end int) ([]T, error){
	len_arr := len(arr)
	var zero []T

	if start > end {
		return zero, fmt.Errorf("начальный индекс больше конечного")
	}
	if !(-len_arr <= start && start < len_arr) {
		return zero, fmt.Errorf("начальный индекс выходит из диапозона")
	}
	if !(-len_arr <= end && end <= len_arr) {
		return zero, fmt.Errorf("конечный индекс выходит из диапозона")
	}
	if end - start > len_arr {
		return zero, fmt.Errorf("выходной массив больше начального")
	}

	if start >= 0 && end >= 0 {
		return arr[start:end], nil
	}

	if start < 0 && end < 0 {
		start += len_arr
		end += len_arr
		return arr[start:end], nil
	}

	if start < 0 && end >= 0 {
		start += len_arr
		return append(arr[start:], arr[:end]...), nil
	}

	return zero, fmt.Errorf("ни одно из условий не подошло")
}

func checkVoices(vad *webrtcvad.VAD, monoAudio []byte, PERCENT_VOICES_THRESHOLD float64) bool {
	len_monoAudio := len(monoAudio)
	var active_counter int32
	var all_iterations int32

	for i := 0; i < len_monoAudio; i += Voice_Detection_Frame_Size {
		if i+Voice_Detection_Frame_Size >= len_monoAudio {
			break
		}
		frame := monoAudio[i:i+Voice_Detection_Frame_Size]

		if len(frame) != Voice_Detection_Frame_Size {
			break
		}

		active, err := vad.Process(SAMPLE_RATE, frame)
		if err != nil {
			log.Fatal(err)
		}

		if active {
			active_counter++
		}
		all_iterations++
	}
	// fmt.Print(float64(active_counter) / float64(all_iterations))
	return float64(active_counter) / float64(all_iterations) > PERCENT_VOICES_THRESHOLD
}

func processInt2ByteAudio(bufferPCM []int16) []byte {
	len_bufferPCM := len(bufferPCM)
	bufferBytes := make([]byte, len_bufferPCM*2)

	for i := 0; i < len_bufferPCM; i++ {
		binary.LittleEndian.PutUint16(bufferBytes[i*2:], uint16(bufferPCM[i]))
	}

	return bufferBytes
}

func preprocess2MonoAudio(all_buffer []int16) []int16 {
	len_all_buffer := len(all_buffer)
	monoAudio := make([]int16, len_all_buffer / 2)

	for i := 0; i < len_all_buffer/2; i++ {
		monoAudio[i] = (all_buffer[i*2] + all_buffer[i*2+1]) / 2
	}

	return monoAudio
}

func checkQuitePause(bufferPause []int16, quietThresholdDB float64) (bool) {
	const maxPCMvalue = 32768.0
	const CONST_DECIBELS = 100
	
	if len(bufferPause) == 0 {
		return false
	}
	
	var sumPCM float64

	for _, value := range bufferPause {
		sumPCM += math.Abs(float64(value))
	}
	srVal := sumPCM / float64(len(bufferPause))
	
	normalizedValue := srVal / maxPCMvalue
	
	if normalizedValue == 0 {
		normalizedValue = 1e-12
	}

	srDB := 20 * math.Log10(normalizedValue)
	
	return srDB + CONST_DECIBELS < quietThresholdDB
}

func reWritingBuffWheel(mainbuff *[]int16, buff []int16, offset *int) (error) {
	lenMain := len(*mainbuff)
	lenBuff := len(buff)
	
	if lenBuff > lenMain {
		return fmt.Errorf("reWriting mass less, than second argument")
	}
	if !(0 <= *offset && *offset < lenMain) {
		return fmt.Errorf("offset is bad value")
	}
	
	countBuff := 0
	for i := *offset; i < len(*mainbuff); i++ {
		if countBuff >= lenBuff{
			break
		}
		(*mainbuff)[i] = buff[countBuff]
		countBuff++
	}
	
	if *offset + lenBuff > lenMain {
		for i := 0; i < len(*mainbuff); i++ {
			if countBuff == lenBuff {
				break
			}
			(*mainbuff)[i] = buff[countBuff]
			countBuff++
		}
	}
	
	*offset = *offset + lenBuff
	if *offset >= lenMain {
		*offset %= lenMain
	}
	
	return nil
}